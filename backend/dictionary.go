package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// upsertDictionaryOccurrence 记录一次单词出现：新词插入一行，已存在则出现次数 +1、刷新最后更新时间。
// 用 ON DUPLICATE KEY UPDATE 做原子 upsert，天然处理并发下多个用户同时第一次输入同一新词的竞态。
func (a *App) upsertDictionaryOccurrence(ctx context.Context, wordKey, displayWord string, now time.Time) {
	if err := a.dict.UpsertOccurrence(ctx, wordKey, displayWord, now); err != nil {
		log.Printf("记录词库出现次数失败 word=%s: %v", wordKey, err)
	}
}

// lookupDictionarySenses 查全局词库缓存，只有真正缓存了非空释义才算命中
func (a *App) lookupDictionarySenses(ctx context.Context, wordKey string) ([]Sense, bool) {
	sensesRaw, err := a.dict.LookupSenses(ctx, wordKey)
	if err != nil || len(sensesRaw) == 0 {
		return nil, false
	}
	var senses []Sense
	if err := json.Unmarshal(sensesRaw, &senses); err != nil {
		log.Printf("解析词库释义失败 word=%s: %v", wordKey, err)
		return nil, false
	}
	if len(senses) == 0 {
		return nil, false
	}
	return senses, true
}

// saveDictionarySenses 把查到的释义写入全局词库缓存，只在词库里还没有缓存内容时写入，避免并发下互相覆盖
func (a *App) saveDictionarySenses(ctx context.Context, wordKey string, senses []Sense) {
	sensesJSON, err := json.Marshal(senses)
	if err != nil {
		log.Printf("序列化词库释义失败 word=%s: %v", wordKey, err)
		return
	}
	if err := a.dict.SaveSenses(ctx, wordKey, sensesJSON); err != nil {
		log.Printf("写入词库释义失败 word=%s: %v", wordKey, err)
	}
}

type dictionaryEntry struct {
	WordKey         string    `json:"word_key"`
	DisplayWord     string    `json:"display_word"`
	Senses          []Sense   `json:"senses"`
	OccurrenceCount int       `json:"occurrence_count"`
	LastUpdatedAt   time.Time `json:"last_updated_at"`
}

// formatSenses 把释义拼成一段可读文本，如 "n. 名词；v. 动词"，用于导出等纯文本场景
func formatSenses(senses []Sense) string {
	parts := make([]string, 0, len(senses))
	for _, s := range senses {
		if s.Pos != "" {
			parts = append(parts, s.Pos+" "+s.Translation)
		} else {
			parts = append(parts, s.Translation)
		}
	}
	return strings.Join(parts, "；")
}

// handleListDictionary 管理员查看全局词库：单词、释义、出现次数、最后更新时间。
// 分页 + 关键字过滤 + 释义状态过滤都在数据库侧完成，前端只渲染当前页。
// status 取值：no_definition（暂无释义）/ has_definition（已有释义）/ 空（不过滤）。
func (a *App) handleListDictionary(w http.ResponseWriter, r *http.Request) {
	page, limit, offset := parsePagination(r)
	keyword := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("keyword")))
	status := r.URL.Query().Get("status")

	total, err := a.dict.Count(r.Context(), keyword, status)
	if err != nil {
		log.Printf("统计词库总数失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	entries, err := a.dict.ListPage(r.Context(), keyword, status, limit, offset)
	if err != nil {
		log.Printf("查询词库失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	result := newPageResult(entries, total, page, limit)
	// 顶部总数展示全库单词数，不受当前关键字/状态筛选影响；空筛选下 total 即全库数，无需再查一次。
	if keyword == "" && status == "" {
		result.TotalAll = total
	} else if totalAll, err := a.dict.Count(r.Context(), "", ""); err == nil {
		result.TotalAll = totalAll
	}
	writeJSON(w, http.StatusOK, result)
}

type retryDictionaryRequest struct {
	WordKey string `json:"word_key"`
}

// handleRetryDictionary 管理员手动对词库里一条「暂无释义」的词条重新触发一次查词。
// 同步执行：成功则把新释义写回词库缓存并返回，失败则返回明确错误、不改动原词条。
// 只影响全局词库缓存表 word_dictionary，不触碰任何用户个人的 words 记录。
func (a *App) handleRetryDictionary(w http.ResponseWriter, r *http.Request) {
	var req retryDictionaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	wordKey := strings.ToLower(strings.TrimSpace(req.WordKey))
	if wordKey == "" {
		writeError(w, http.StatusBadRequest, "单词不能为空")
		return
	}

	// 先确认词条存在，避免对不存在的 key 白白调用一次模型
	if _, err := a.dict.LookupSenses(r.Context(), wordKey); err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "词条不存在")
		return
	}

	cfg := a.getDeepSeekConfig()
	result := translateWord(r.Context(), wordKey, cfg)
	merged := mergeSensesByPos(result.Senses)
	if len(merged) == 0 {
		writeError(w, http.StatusServiceUnavailable, "重新查询失败，请稍后重试")
		return
	}
	a.saveDictionarySenses(r.Context(), wordKey, merged)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"word_key": wordKey,
		"senses":   merged,
	})
}

// handleExportDictionary 管理员导出全局词库为 CSV，带 UTF-8 BOM 保证 Excel 打开中文不乱码
func (a *App) handleExportDictionary(w http.ResponseWriter, r *http.Request) {
	entries, err := a.dict.List(r.Context())
	if err != nil {
		log.Printf("导出词库失败: %v", err)
		writeError(w, http.StatusInternalServerError, "导出失败")
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=dictionary.csv")
	w.Write([]byte("\xEF\xBB\xBF"))

	writer := csv.NewWriter(w)
	writer.Write([]string{"单词", "释义", "最后更新时间"})
	for _, e := range entries {
		writer.Write([]string{e.DisplayWord, formatSenses(e.Senses), e.LastUpdatedAt.Format("2006-01-02 15:04:05")})
	}
	writer.Flush()
}

// handleDeleteDictionaryEntry 管理员删除全局词库里的一条缓存记录；只影响词库缓存本身，
// 不影响任何用户已经保存在自己单词表里的记录
func (a *App) handleDeleteDictionaryEntry(w http.ResponseWriter, r *http.Request) {
	wordKey := r.PathValue("word_key")
	if err := a.dict.Delete(r.Context(), wordKey); err != nil {
		log.Printf("删除词库记录失败 word=%s: %v", wordKey, err)
		writeError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type deleteDictionaryBatchRequest struct {
	WordKeys []string `json:"word_keys"`
}

// handleDeleteDictionaryBatch 管理员批量删除词库缓存；上限 maxPageLimit 条，
// 超出取前 N 条直接执行，避免一次提交删除过多记录拖慢数据库。
func (a *App) handleDeleteDictionaryBatch(w http.ResponseWriter, r *http.Request) {
	var req deleteDictionaryBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	keys := make([]string, 0, len(req.WordKeys))
	for _, k := range req.WordKeys {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) > maxPageLimit {
		keys = keys[:maxPageLimit]
	}
	affected, err := a.dict.DeleteMany(r.Context(), keys)
	if err != nil {
		log.Printf("批量删除词库记录失败: %v", err)
		writeError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": affected})
}

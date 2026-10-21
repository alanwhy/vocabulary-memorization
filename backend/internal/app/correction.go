// 单词纠错与历史内容补全：页面 -> 个人词条预约 -> 同词串行查询 -> 个人 JSON/全局缓存。
// 纠错强制重查所有内容，普通补全优先命中缓存；失败保留旧释义和独立的复习状态。
package app

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// lockWordLookup 对同词请求串行化，等待可取消；锁只保护模型查询及缓存写回。
func (a *App) lockWordLookup(ctx context.Context, key string) (func(), error) {
	value, _ := a.lookupLocks.LoadOrStore(key, make(chan struct{}, 1))
	lock := value.(chan struct{})
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ensureWordEnrichment 只补正常旧释义；先预约，复用齐全缓存，否则启动后台查词。
func (a *App) ensureWordEnrichment(ctx context.Context, wd *Word) {
	if wd.Translating || !sensesNeedEnrichment(wd.Senses) {
		return
	}
	started, err := a.words.TryMarkTranslating(ctx, wd.ID, time.Now())
	if err != nil {
		log.Printf("预约补全失败 word=%s: %v", wd.WordKey, err)
		return
	}
	wd.Translating = true
	if !started {
		return
	}
	if cached, hit := a.lookupDictionarySenses(ctx, wd.WordKey); hit && sensesEnriched(cached) {
		raw, _ := json.Marshal(cached)
		if err := a.words.UpdateSenses(ctx, wd.ID, raw); err == nil {
			wd.Senses, wd.Translating = cached, false
			return
		}
	}
	a.spawnTranslation(wd.ID, wd.WordKey)
}

// restoreWordSenses 使用独立短 context，保证客户端断开后也能解除预约、保留旧内容。
func (a *App) restoreWordSenses(id int, raw []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.words.UpdateSenses(ctx, id, raw); err != nil {
		log.Printf("恢复单词内容失败 id=%d: %v", id, err)
	}
}

func (a *App) handleCorrectWord(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > 500 {
		writeError(w, http.StatusBadRequest, "请填写 1–500 字的错误原因")
		return
	}
	list, err := a.words.FindByIDs(r.Context(), currentUser(r).ID, []int{id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if len(list) == 0 {
		writeError(w, http.StatusNotFound, "单词不存在")
		return
	}
	wd := list[0]
	if wd.Translating {
		writeError(w, http.StatusConflict, "单词正在查询，请完成后再纠错")
		return
	}
	started, err := a.words.TryMarkTranslating(r.Context(), id, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "重新查询失败")
		return
	}
	if !started {
		writeError(w, http.StatusConflict, "单词正在查询，请完成后再纠错")
		return
	}
	// 预约成功后重新读取：此前快照可能在其他纠错完成前取得，失败不能恢复成更旧内容。
	latest, err := a.words.FindByIDs(r.Context(), currentUser(r).ID, []int{id})
	if err != nil || len(latest) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if clearErr := a.words.ClearTranslating(ctx, id); clearErr != nil {
			log.Printf("解除纠错预约失败 id=%d: %v", id, clearErr)
		}
		writeError(w, http.StatusInternalServerError, "读取单词内容失败，请重试")
		return
	}
	wd = latest[0]
	oldRaw, _ := json.Marshal(wd.Senses)
	saved := false
	defer func() {
		if !saved {
			a.restoreWordSenses(id, oldRaw)
		}
	}()
	unlock, err := a.lockWordLookup(r.Context(), wd.WordKey)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "纠错查询已取消，请重试")
		return
	}
	defer unlock()
	// 纠错永不复用缓存；反馈只用于核对，必须得到完整的新释义才允许覆盖。
	result := translateWord(r.Context(), wd.WordKey, a.getDeepSeekConfig(), reason)
	if result.IsSpellingError {
		writeError(w, http.StatusUnprocessableEntity, "单词拼写可能有误，请检查")
		return
	}
	merged := mergeSensesByPos(result.Senses)
	if len(merged) == 0 {
		writeError(w, http.StatusServiceUnavailable, "纠错查询失败，原内容已保留，请重试")
		return
	}
	raw, err := json.Marshal(merged)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存失败")
		return
	}
	if err := a.dict.ReplaceSenses(r.Context(), wd.WordKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "词库缓存更新失败，原内容已保留，请重试")
		return
	}
	if err := a.words.UpdateSenses(r.Context(), id, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "保存失败，原内容已保留")
		return
	}
	saved = true
	wd.Senses, wd.Translating = merged, false
	writeJSON(w, http.StatusOK, wd)
}

// separateWordRelations 删除派生/形近交叉及本词项，防止同一形式被展示成两种关系。
// 语义由模型判定，这里只对模型明确给出的派生集合做确定性去交叉。
func separateWordRelations(word string, senses []Sense) []Sense {
	keyOf := func(ref string) string {
		parts := strings.FieldsFunc(ref, func(r rune) bool { return r == '（' || r == '(' })
		if len(parts) == 0 {
			return ""
		}
		return strings.ToLower(strings.TrimSpace(parts[0]))
	}
	derived := map[string]bool{strings.ToLower(word): true}
	for _, s := range senses {
		for _, ref := range s.Derivatives {
			if strings.TrimSpace(ref) != "" {
				derived[keyOf(ref)] = true
			}
		}
	}
	for i := range senses {
		out := make([]string, 0, len(senses[i].Lookalikes))
		for _, ref := range senses[i].Lookalikes {
			if strings.TrimSpace(ref) != "" && !derived[keyOf(ref)] {
				out = append(out, strings.TrimSpace(ref))
			}
		}
		senses[i].Lookalikes = out
	}
	return senses
}

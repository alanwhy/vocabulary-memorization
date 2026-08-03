package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// wordPattern 简单校验：以英文字母开头，只允许字母、空格、连字符、单引号，长度不超过 64
var wordPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z'\- ]{0,63}$`)

func main() {
	loadBuiltinDict()
	connectDB()
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/words", handleAddWord)
	mux.HandleFunc("GET /api/words", handleListWords)
	mux.HandleFunc("DELETE /api/words/{id}", handleDeleteWord)
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	addr := ":8080"
	log.Printf("服务启动，监听 %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

type addWordRequest struct {
	Word string `json:"word"`
}

func handleAddWord(w http.ResponseWriter, r *http.Request) {
	var req addWordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	raw := strings.TrimSpace(req.Word)
	if raw == "" {
		writeError(w, http.StatusBadRequest, "单词不能为空")
		return
	}
	if !wordPattern.MatchString(raw) {
		writeError(w, http.StatusBadRequest, "请输入有效的英文单词（仅支持英文字母、空格、连字符、单引号）")
		return
	}
	wordKey := strings.ToLower(raw)
	now := time.Now()

	// 已存在则次数 +1 并直接返回
	if handled := tryIncrementExisting(w, wordKey, now); handled {
		return
	}

	// 不存在：查翻译后新建一条记录
	result := translateWord(wordKey)
	res, err := db.Exec(
		`INSERT INTO words (word_key, display_word, translation, pos, review_count, first_added_at, last_reviewed_at) VALUES (?, ?, ?, ?, 1, ?, ?)`,
		wordKey, raw, result.Translation, result.Pos, now, now,
	)
	if err != nil {
		// 并发下可能有另一个请求刚好抢先插入了同一个单词，退化为累加次数
		if mysqlErr, ok := err.(*mysqldriver.MySQLError); ok && mysqlErr.Number == 1062 {
			if handled := tryIncrementExisting(w, wordKey, now); handled {
				return
			}
		}
		log.Printf("插入单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "保存失败")
		return
	}
	id, _ := res.LastInsertId()

	newWord := Word{
		ID:             int(id),
		WordKey:        wordKey,
		DisplayWord:    raw,
		Translation:    result.Translation,
		Pos:            result.Pos,
		ReviewCount:    1,
		FirstAddedAt:   now,
		LastReviewedAt: now,
	}
	writeJSON(w, http.StatusCreated, newWord)
}

// tryIncrementExisting 如果 wordKey 已存在，则次数 +1、更新最近背诵时间，并写响应；
// 返回 true 表示请求已经处理完毕（无论是成功还是出错），调用方不需要再做任何事。
func tryIncrementExisting(w http.ResponseWriter, wordKey string, now time.Time) bool {
	var existing Word
	err := db.QueryRow(
		`SELECT id, word_key, display_word, translation, pos, review_count, first_added_at, last_reviewed_at FROM words WHERE word_key = ?`,
		wordKey,
	).Scan(&existing.ID, &existing.WordKey, &existing.DisplayWord, &existing.Translation, &existing.Pos, &existing.ReviewCount, &existing.FirstAddedAt, &existing.LastReviewedAt)

	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		log.Printf("查询单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return true
	}

	newCount := existing.ReviewCount + 1
	if _, err := db.Exec(`UPDATE words SET review_count = ?, last_reviewed_at = ? WHERE id = ?`, newCount, now, existing.ID); err != nil {
		log.Printf("更新单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "更新失败")
		return true
	}
	existing.ReviewCount = newCount
	existing.LastReviewedAt = now
	writeJSON(w, http.StatusOK, existing)
	return true
}

func handleListWords(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, word_key, display_word, translation, pos, review_count, first_added_at, last_reviewed_at FROM words ORDER BY review_count DESC, last_reviewed_at DESC`)
	if err != nil {
		log.Printf("查询列表失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()

	list := []Word{}
	for rows.Next() {
		var wd Word
		if err := rows.Scan(&wd.ID, &wd.WordKey, &wd.DisplayWord, &wd.Translation, &wd.Pos, &wd.ReviewCount, &wd.FirstAddedAt, &wd.LastReviewedAt); err != nil {
			log.Printf("读取记录失败: %v", err)
			continue
		}
		list = append(list, wd)
	}
	writeJSON(w, http.StatusOK, list)
}

func handleDeleteWord(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}
	if _, err := db.Exec(`DELETE FROM words WHERE id = ?`, id); err != nil {
		log.Printf("删除失败: %v", err)
		writeError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("写入响应失败: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// upsertDictionaryOccurrence 记录一次单词出现：新词插入一行，已存在则出现次数 +1、刷新最后更新时间。
// 用 ON DUPLICATE KEY UPDATE 做原子 upsert，天然处理并发下多个用户同时第一次输入同一新词的竞态。
func upsertDictionaryOccurrence(wordKey, displayWord string, now time.Time) {
	if _, err := db.Exec(
		`INSERT INTO word_dictionary (word_key, display_word, senses, occurrence_count, first_seen_at, last_updated_at)
		 VALUES (?, ?, JSON_ARRAY(), 1, ?, ?)
		 ON DUPLICATE KEY UPDATE occurrence_count = occurrence_count + 1, last_updated_at = ?`,
		wordKey, displayWord, now, now, now,
	); err != nil {
		log.Printf("记录词库出现次数失败 word=%s: %v", wordKey, err)
	}
}

// lookupDictionarySenses 查全局词库缓存，只有真正缓存了非空释义才算命中
func lookupDictionarySenses(wordKey string) ([]Sense, bool) {
	var sensesRaw []byte
	if err := db.QueryRow(`SELECT senses FROM word_dictionary WHERE word_key = ?`, wordKey).Scan(&sensesRaw); err != nil {
		return nil, false
	}
	if len(sensesRaw) == 0 {
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
func saveDictionarySenses(wordKey string, senses []Sense) {
	sensesJSON, err := json.Marshal(senses)
	if err != nil {
		log.Printf("序列化词库释义失败 word=%s: %v", wordKey, err)
		return
	}
	if _, err := db.Exec(
		`UPDATE word_dictionary SET senses = ? WHERE word_key = ? AND (senses IS NULL OR JSON_LENGTH(senses) = 0)`,
		sensesJSON, wordKey,
	); err != nil {
		log.Printf("写入词库释义失败 word=%s: %v", wordKey, err)
	}
}

type dictionaryEntry struct {
	WordKey         string    `json:"word_key"`
	DisplayWord     string    `json:"display_word"`
	OccurrenceCount int       `json:"occurrence_count"`
	LastUpdatedAt   time.Time `json:"last_updated_at"`
}

// handleListDictionary 管理员查看全局词库：单词、出现次数、最后更新时间，按出现次数降序
func handleListDictionary(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT word_key, display_word, occurrence_count, last_updated_at FROM word_dictionary ORDER BY occurrence_count DESC`)
	if err != nil {
		log.Printf("查询词库失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()

	entries := []dictionaryEntry{}
	for rows.Next() {
		var e dictionaryEntry
		if err := rows.Scan(&e.WordKey, &e.DisplayWord, &e.OccurrenceCount, &e.LastUpdatedAt); err != nil {
			log.Printf("读取词库记录失败: %v", err)
			continue
		}
		entries = append(entries, e)
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleDeleteDictionaryEntry 管理员删除全局词库里的一条缓存记录；只影响词库缓存本身，
// 不影响任何用户已经保存在自己单词表里的记录
func handleDeleteDictionaryEntry(w http.ResponseWriter, r *http.Request) {
	wordKey := r.PathValue("word_key")
	if _, err := db.Exec(`DELETE FROM word_dictionary WHERE word_key = ?`, wordKey); err != nil {
		log.Printf("删除词库记录失败 word=%s: %v", wordKey, err)
		writeError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

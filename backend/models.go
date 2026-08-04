package main

import (
	"strings"
	"time"
)

// Sense 一个单词的一个词性 + 对应释义
type Sense struct {
	Pos         string `json:"pos"`
	Translation string `json:"translation"`
}

// mergeSensesByPos 按词性合并释义：同一词性的多条释义合并成一条，用中文分号分隔，保持首次出现顺序
func mergeSensesByPos(senses []Sense) []Sense {
	order := make([]string, 0, len(senses))
	grouped := make(map[string][]string)
	seen := make(map[string]bool)

	for _, s := range senses {
		pos := strings.TrimSpace(s.Pos)
		translation := strings.TrimSpace(s.Translation)
		if translation == "" {
			continue
		}
		if _, ok := grouped[pos]; !ok {
			order = append(order, pos)
		}
		key := pos + "\x00" + translation
		if seen[key] {
			continue
		}
		seen[key] = true
		grouped[pos] = append(grouped[pos], translation)
	}

	merged := make([]Sense, 0, len(order))
	for _, pos := range order {
		merged = append(merged, Sense{Pos: pos, Translation: strings.Join(grouped[pos], "；")})
	}
	return merged
}

// Word 对应数据库 words 表的一条记录
type Word struct {
	ID             int       `json:"id"`
	WordKey        string    `json:"word_key"`
	DisplayWord    string    `json:"display_word"`
	Senses         []Sense   `json:"senses"`
	Translating    bool      `json:"translating"`
	Archived       bool      `json:"archived"`
	ReviewCount    int       `json:"review_count"`
	FirstAddedAt   time.Time `json:"first_added_at"`
	LastReviewedAt time.Time `json:"last_reviewed_at"`
}

// User 对应数据库 users 表的一条记录
type User struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

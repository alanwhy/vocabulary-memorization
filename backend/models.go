package main

import "time"

// Sense 一个单词的一个词性 + 对应释义
type Sense struct {
	Pos         string `json:"pos"`
	Translation string `json:"translation"`
}

// Word 对应数据库 words 表的一条记录
type Word struct {
	ID             int       `json:"id"`
	WordKey        string    `json:"word_key"`
	DisplayWord    string    `json:"display_word"`
	Senses         []Sense   `json:"senses"`
	Translating    bool      `json:"translating"`
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

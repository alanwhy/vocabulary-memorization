package main

import "time"

// Word 对应数据库 words 表的一条记录
type Word struct {
	ID             int       `json:"id"`
	WordKey        string    `json:"word_key"`
	DisplayWord    string    `json:"display_word"`
	Translation    string    `json:"translation"`
	Pos            string    `json:"pos"`
	ReviewCount    int       `json:"review_count"`
	FirstAddedAt   time.Time `json:"first_added_at"`
	LastReviewedAt time.Time `json:"last_reviewed_at"`
}

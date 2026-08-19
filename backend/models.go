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

// User 对应数据库 users 表的一条记录；LastLoginAt 用指针表示“从未登录过”（数据库里为 NULL）
type User struct {
	ID          int        `json:"id"`
	Username    string     `json:"username"`
	IsAdmin     bool       `json:"is_admin"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// UserWithStats 管理员用户列表的一行：User 加上该用户录入的单词总数（含已归档）
type UserWithStats struct {
	User
	WordCount int `json:"word_count"`
}

// pageResult 所有分页接口的统一响应信封；HasMore 由后端算好，前端不用自己拿 total 和 page 推
type pageResult struct {
	Items   interface{} `json:"items"`
	Total   int         `json:"total"`
	// TotalAll 不受筛选影响的整表总数（仅词库管理接口填充），用于顶部展示全库单词数。
	TotalAll int  `json:"total_all,omitempty"`
	Page     int  `json:"page"`
	Limit    int  `json:"limit"`
	HasMore  bool `json:"has_more"`
}

func newPageResult(items interface{}, total, page, limit int) pageResult {
	return pageResult{
		Items:   items,
		Total:   total,
		Page:    page,
		Limit:   limit,
		HasMore: page*limit < total,
	}
}

// dailyCount 某一天的新增单词数，Date 为本地时区的 YYYY-MM-DD
type dailyCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// wordCloudItem 词云里的一条：Word 为展示拼写，Count 为累计背诵次数（权重）
type wordCloudItem struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

// letterStat 按开头字母分组的一条统计
type letterStat struct {
	Letter string `json:"letter"`
	Count  int    `json:"count"`
}

// WordStats 统计页需要的全部聚合数值。列表分页后前端拿不到全量数据，这些改由后端用 SQL 聚合算出。
// 除 TotalAllWords / ArchivedWords 外，其余字段都只统计未归档的单词，保持统计页原有口径。
type WordStats struct {
	TotalWords       int             `json:"total_words"`
	ArchivedWords    int             `json:"archived_words"`
	TotalAllWords    int             `json:"total_all_words"`
	TotalReviews     int             `json:"total_reviews"`
	TodayReviews     int             `json:"today_reviews"`
	TranslatingCount int             `json:"translating_count"`
	ReviewBuckets    []int           `json:"review_buckets"`
	DailyAdditions   []dailyCount    `json:"daily_additions"`
	WordCloud        []wordCloudItem `json:"word_cloud"`
	LetterStats      []letterStat    `json:"letter_stats"`
}

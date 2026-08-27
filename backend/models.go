package main

import (
	"math"
	"strings"
	"time"
)

// Sense 一个单词的一个词性 + 对应释义。
// 强化字段平铺在这里：phonetic/root/affix/synonyms/antonyms/lookalikes 是词级信息，多词性下会重复存储，
// 前端只取第一条词性的词级信息展示；example 是词义级，每条词性对应一句英文例句，
// example_translation 是该例句的中文翻译。synonyms/antonyms/lookalikes 每个元素形如
// "chance（机会）"，英文词后紧跟中文释义，前端无需二次拆解即可直接展示。
// lookalikes 是最后加入的字段，故意不用 omitempty：即使没有形近词也序列化成 []，
// 这样「字段是否为 nil」就能区分「最新 prompt 查过」和「老数据没这个 key」（见 sensesEnriched）。
type Sense struct {
	Pos                string   `json:"pos"`
	Translation        string   `json:"translation"`
	Phonetic           string   `json:"phonetic,omitempty"`
	Example            string   `json:"example,omitempty"`
	ExampleTranslation string   `json:"example_translation,omitempty"`
	Root               string   `json:"root,omitempty"`
	Affix              string   `json:"affix,omitempty"`
	Synonyms           []string `json:"synonyms,omitempty"`
	Antonyms           []string `json:"antonyms,omitempty"`
	Lookalikes         []string `json:"lookalikes"`
}

// sensesEnriched 判断一组释义是否已用最新 prompt 强化过（含例句/词根词缀/近反义/形近词）。
// 以 lookalikes（形近词，最后加入的字段）是否为 nil 作为判据：最新 prompt 一定会返回
// lookalikes 字段，即使没有形近词也是空数组 []，反序列化后是非 nil 空切片；老数据没这个 key，
// 反序列化后是 nil。这样既能给缺形近词的老词补全，又不会在已齐全时重复调用大模型。
func sensesEnriched(senses []Sense) bool {
	if len(senses) == 0 {
		return false
	}
	return senses[0].Lookalikes != nil
}

// mergeSenseEnrichment 把 src 的非空强化字段合并进 dst，字段级「首个非空值优先」：
// 组内后出现的空字段不会覆盖前面已经记下的有效值。
func mergeSenseEnrichment(dst *Sense, src Sense) {
	if dst.Phonetic == "" && src.Phonetic != "" {
		dst.Phonetic = src.Phonetic
	}
	if dst.Example == "" && src.Example != "" {
		dst.Example = src.Example
	}
	if dst.ExampleTranslation == "" && src.ExampleTranslation != "" {
		dst.ExampleTranslation = src.ExampleTranslation
	}
	if dst.Root == "" && src.Root != "" {
		dst.Root = src.Root
	}
	if dst.Affix == "" && src.Affix != "" {
		dst.Affix = src.Affix
	}
	if len(dst.Synonyms) == 0 && len(src.Synonyms) > 0 {
		dst.Synonyms = src.Synonyms
	}
	if len(dst.Antonyms) == 0 && len(src.Antonyms) > 0 {
		dst.Antonyms = src.Antonyms
	}
	if len(dst.Lookalikes) == 0 && len(src.Lookalikes) > 0 {
		dst.Lookalikes = src.Lookalikes
	}
}

// mergeSensesByPos 按词性合并释义：同一词性的多条释义合并成一条，用中文分号分隔，保持首次出现顺序。
// 强化字段随合并一并保留（词级字段与 example 都取组内第一个非空值），避免合并后字段丢失。
func mergeSensesByPos(senses []Sense) []Sense {
	order := make([]string, 0, len(senses))
	grouped := make(map[string][]string)
	enrichment := make(map[string]Sense)
	seen := make(map[string]bool)

	for _, s := range senses {
		pos := strings.TrimSpace(s.Pos)
		translation := strings.TrimSpace(s.Translation)
		if translation == "" {
			continue
		}
		if _, ok := grouped[pos]; !ok {
			order = append(order, pos)
			enrichment[pos] = s
		}
		key := pos + "\x00" + translation
		if seen[key] {
			continue
		}
		seen[key] = true
		grouped[pos] = append(grouped[pos], translation)
		e := enrichment[pos]
		mergeSenseEnrichment(&e, s)
		enrichment[pos] = e
	}

	merged := make([]Sense, 0, len(order))
	for _, pos := range order {
		e := enrichment[pos]
		e.Pos = pos
		e.Translation = strings.Join(grouped[pos], "；")
		merged = append(merged, e)
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
	// 间隔重复（SRS）排期状态：DueAt 为下次到期时间（NULL = 从未用闪卡复习过，视为新词立即到期），
	// IntervalDays 为当前间隔天数，EaseFactor 为难度系数（区间 [1.30, 2.50]）。
	DueAt        *time.Time `json:"due_at"`
	IntervalDays int        `json:"interval_days"`
	EaseFactor   float64    `json:"ease_factor"`
}

// applySRSScheduling 根据一次闪卡自评结果算出下一个复习间隔（天）和新的难度系数。
// rating 只能是 good / hard / again，与前端三个评分按钮一一对应，采用简化版 SM-2：
//   - good：间隔按难度系数倍增（首次为 1 天），难度不变
//   - hard：间隔按 1.2 倍小幅增长（至少 1 天），难度下降 0.15
//   - again：间隔重置回 1 天（明天再见），难度下降 0.20
//
// 难度系数始终收敛在 [1.30, 2.50] 并保留两位小数，避免越界或精度抖动。
func applySRSScheduling(intervalDays int, easeFactor float64, rating string) (int, float64) {
	const (
		minEase    = 1.30
		maxEase    = 2.50
		hardFactor = 1.2
	)

	switch rating {
	case "hard":
		intervalDays = int(math.Round(float64(intervalDays) * hardFactor))
		if intervalDays < 1 {
			intervalDays = 1
		}
		easeFactor -= 0.15
	case "again":
		intervalDays = 1
		easeFactor -= 0.20
	default: // good（含未知值兜底，避免非法评分破坏排期）
		if intervalDays == 0 {
			intervalDays = 1
		} else {
			intervalDays = int(math.Round(float64(intervalDays) * easeFactor))
		}
	}

	if easeFactor < minEase {
		easeFactor = minEase
	}
	if easeFactor > maxEase {
		easeFactor = maxEase
	}
	return intervalDays, math.Round(easeFactor*100) / 100
}

// User 对应数据库 users 表的一条记录；LastLoginAt 用指针表示“从未登录过”（数据库里为 NULL）。
// Disabled 表示账号是否被管理员禁用：禁用后登录被拒、已有会话立即失效。
type User struct {
	ID          int        `json:"id"`
	Username    string     `json:"username"`
	IsAdmin     bool       `json:"is_admin"`
	Disabled    bool       `json:"disabled"`
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
	Items interface{} `json:"items"`
	Total int         `json:"total"`
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

// wordCloudItem 词云里的一条：Word 为展示拼写，Count 为累计背诵次数（权重），Meaning 为中文释义（tooltip 用）
type wordCloudItem struct {
	Word    string `json:"word"`
	Count   int    `json:"count"`
	Meaning string `json:"meaning"`
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

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"vocab-backend/internal/model"
)

// WordRepository 封装 words 表的数据访问。
type WordRepository struct {
	db *sql.DB
}

// NewWordRepository 创建单词数据访问实现。
func NewWordRepository(db *sql.DB) *WordRepository {
	return &WordRepository{db: db}
}

// wordColumns 与 scanWordRows 的扫描顺序严格配对，避免查询新增列后出现字段错位。
const wordColumns = `id, word_key, display_word, senses, important_glosses, translating, archived, review_count, first_added_at, last_reviewed_at, due_at, interval_days, ease_factor`

func scanWordRows(rows *sql.Rows) ([]model.Word, error) {
	defer rows.Close()

	list := []model.Word{}
	for rows.Next() {
		var word model.Word
		var sensesRaw []byte
		var importantRaw []byte
		var dueAt sql.NullTime
		if err := rows.Scan(&word.ID, &word.WordKey, &word.DisplayWord, &sensesRaw, &importantRaw, &word.Translating, &word.Archived, &word.ReviewCount, &word.FirstAddedAt, &word.LastReviewedAt, &dueAt, &word.IntervalDays, &word.EaseFactor); err != nil {
			return nil, err
		}
		word.DueAt = nullTimePtr(dueAt)
		if len(sensesRaw) > 0 {
			if err := json.Unmarshal(sensesRaw, &word.Senses); err != nil {
				return nil, err
			}
		}
		// NULL/空值归一为非 nil 空切片，前端无需额外区分 null。
		word.ImportantGlosses = []string{}
		if len(importantRaw) > 0 {
			if err := json.Unmarshal(importantRaw, &word.ImportantGlosses); err != nil {
				return nil, err
			}
		}
		list = append(list, word)
	}
	return list, rows.Err()
}

// wordOrderBy 将外部排序值映射为固定 SQL 片段，既阻止 SQL 注入，也用 id 保证分页稳定。
func wordOrderBy(sort string) string {
	switch sort {
	case "time":
		return `last_reviewed_at DESC, id DESC`
	case "time_asc":
		return `last_reviewed_at ASC, id ASC`
	case "alpha":
		return `word_key ASC, id ASC`
	case "alpha_desc":
		return `word_key DESC, id DESC`
	case "count_asc":
		return `review_count ASC, last_reviewed_at ASC, id ASC`
	default:
		return `review_count DESC, last_reviewed_at DESC, id DESC`
	}
}

func (r *WordRepository) Insert(ctx context.Context, userID int, wordKey, displayWord string, sensesJSON []byte, translating bool, now time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO words (user_id, word_key, display_word, senses, translating, review_count, first_added_at, last_reviewed_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		userID, wordKey, displayWord, sensesJSON, translating, now, now,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// FindByUserAndKey 返回原始 senses 供调用方判断是否需要强化，同时解码用户的重要释义标记。
func (r *WordRepository) FindByUserAndKey(ctx context.Context, userID int, wordKey string) (model.Word, []byte, error) {
	var word model.Word
	var sensesRaw []byte
	var importantRaw []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, word_key, display_word, senses, important_glosses, translating, archived, review_count, first_added_at, last_reviewed_at FROM words WHERE user_id = ? AND word_key = ?`,
		userID, wordKey,
	).Scan(&word.ID, &word.WordKey, &word.DisplayWord, &sensesRaw, &importantRaw, &word.Translating, &word.Archived, &word.ReviewCount, &word.FirstAddedAt, &word.LastReviewedAt)
	word.ImportantGlosses = []string{}
	if err == nil && len(importantRaw) > 0 {
		if unmarshalErr := json.Unmarshal(importantRaw, &word.ImportantGlosses); unmarshalErr != nil {
			return word, sensesRaw, unmarshalErr
		}
	}
	return word, sensesRaw, err
}

func (r *WordRepository) IncrementReview(ctx context.Context, id, newCount int, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE words SET review_count = ?, last_reviewed_at = ? WHERE id = ?`, newCount, now, id)
	return err
}

// ApplyFlashcardReview 原子写入一次闪卡自评的计数、时间、SRS 排期与归档结果。
// WHERE 同时限定用户，防止跨用户更新同一个数字 ID。
func (r *WordRepository) ApplyFlashcardReview(ctx context.Context, id, userID, newCount, intervalDays int, easeFactor float64, dueAt, now time.Time, archived bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE words SET review_count = ?, last_reviewed_at = ?, interval_days = ?, ease_factor = ?, due_at = ?, archived = ? WHERE id = ? AND user_id = ?`,
		newCount, now, intervalDays, easeFactor, dueAt, archived, id, userID,
	)
	return err
}

// DueFlashcards 返回未归档且已到期的闪卡，优先重复出现用户反复遗忘的词。
func (r *WordRepository) DueFlashcards(ctx context.Context, userID, limit int, now time.Time) ([]model.Word, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+wordColumns+` FROM words
		 WHERE user_id = ? AND archived = 0 AND (due_at IS NULL OR due_at <= ?)
		 ORDER BY review_count DESC, due_at ASC, id ASC
		 LIMIT ?`,
		userID, now, limit,
	)
	if err != nil {
		return nil, err
	}
	return scanWordRows(rows)
}

// ListPage 在数据库侧完成分页、关键字与释义状态过滤，排序值只接受白名单映射。
func (r *WordRepository) ListPage(ctx context.Context, userID int, archived bool, keyword, status, sort string, limit, offset int) ([]model.Word, error) {
	conditions, args := senseFilterWhere(keyword, status)
	if conditions != "" {
		conditions = " AND " + conditions
	}
	args = append([]interface{}{userID, archived}, args...)
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+wordColumns+` FROM words WHERE user_id = ? AND archived = ?`+conditions+
			` ORDER BY `+wordOrderBy(sort)+` LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	return scanWordRows(rows)
}

func (r *WordRepository) CountByUser(ctx context.Context, userID int, archived bool, keyword, status string) (int, error) {
	conditions, args := senseFilterWhere(keyword, status)
	if conditions != "" {
		conditions = " AND " + conditions
	}
	args = append([]interface{}{userID, archived}, args...)
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM words WHERE user_id = ? AND archived = ?`+conditions, args...).Scan(&count)
	return count, err
}

// ResetReviewCounts 只重置背诵次数，不改变现有 SRS 排期字段。
func (r *WordRepository) ResetReviewCounts(ctx context.Context, userID int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE words SET review_count = 1 WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *WordRepository) Delete(ctx context.Context, id, userID int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM words WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteByUserID 删除指定用户的全部单词，供管理侧删除用户前清理关联记录。
func (r *WordRepository) DeleteByUserID(ctx context.Context, userID int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM words WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteByWordKey 删除所有用户中匹配 word_key 的记录，保持词库管理删除的原有联动语义。
func (r *WordRepository) DeleteByWordKey(ctx context.Context, wordKey string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM words WHERE word_key = ?`, wordKey)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteByWordKeys 批量删除全体用户匹配的单词；值继续使用参数绑定，仅占位符按数量生成。
func (r *WordRepository) DeleteByWordKeys(ctx context.Context, wordKeys []string) (int64, error) {
	if len(wordKeys) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(wordKeys)), ",")
	args := make([]interface{}, len(wordKeys))
	for index, wordKey := range wordKeys {
		args[index] = wordKey
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM words WHERE word_key IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *WordRepository) SetArchived(ctx context.Context, id, userID int, archived bool) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE words SET archived = ? WHERE id = ? AND user_id = ?`, archived, id, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *WordRepository) UpdateSenses(ctx context.Context, id int, sensesJSON []byte) error {
	_, err := r.db.ExecContext(ctx, `UPDATE words SET senses = ?, translating = 0 WHERE id = ?`, sensesJSON, id)
	return err
}

// UpdateImportantGlosses 只修改当前用户单词的手工释义标记，不触碰后台可能重写的 senses。
func (r *WordRepository) UpdateImportantGlosses(ctx context.Context, id, userID int, glossesJSON []byte) error {
	_, err := r.db.ExecContext(ctx, `UPDATE words SET important_glosses = ? WHERE id = ? AND user_id = ?`, glossesJSON, id, userID)
	return err
}

// MarkTranslationStarted 刷新本轮查词开始时间，避免合法重试被后台扫描器误判为卡死。
func (r *WordRepository) MarkTranslationStarted(ctx context.Context, id int, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE words SET translation_started_at = ? WHERE id = ?`, now, id)
	return err
}

// MarkTranslating 同时设置查词状态和开始时间；状态也充当防止重复发起查词的标记。
func (r *WordRepository) MarkTranslating(ctx context.Context, id int, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE words SET translating = 1, translation_started_at = ? WHERE id = ?`, now, id)
	return err
}

// FindTranslating 返回进程重启前尚未完成的全部查词任务。
func (r *WordRepository) FindTranslating(ctx context.Context) ([]model.Word, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, word_key FROM words WHERE translating = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []model.Word{}
	for rows.Next() {
		var word model.Word
		if err := rows.Scan(&word.ID, &word.WordKey); err != nil {
			return nil, err
		}
		list = append(list, word)
	}
	return list, rows.Err()
}

// FindTranslatingStale 返回超过阈值或缺少开始时间的查词任务，供周期扫描恢复。
func (r *WordRepository) FindTranslatingStale(ctx context.Context, threshold time.Time) ([]model.Word, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, word_key FROM words WHERE translating = 1 AND (translation_started_at IS NULL OR translation_started_at < ?)`,
		threshold,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []model.Word{}
	for rows.Next() {
		var word model.Word
		if err := rows.Scan(&word.ID, &word.WordKey); err != nil {
			return nil, err
		}
		list = append(list, word)
	}
	return list, rows.Err()
}

// FindTranslatingByUser 返回当前用户尚在查词的完整记录，供前端局部轮询更新。
func (r *WordRepository) FindTranslatingByUser(ctx context.Context, userID int) ([]model.Word, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+wordColumns+` FROM words WHERE user_id = ? AND translating = 1`, userID)
	if err != nil {
		return nil, err
	}
	return scanWordRows(rows)
}

// FindByIDs 返回当前用户指定 ID 的最新记录，包括已经完成翻译的记录。
func (r *WordRepository) FindByIDs(ctx context.Context, userID int, ids []int) ([]model.Word, error) {
	if len(ids) == 0 {
		return []model.Word{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+wordColumns+` FROM words WHERE user_id = ? AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	return scanWordRows(rows)
}

// Stats 使用聚合查询生成统计页数据，避免将全量单词拉回应用层计算。
func (r *WordRepository) Stats(ctx context.Context, userID int, since, since7, todaySince, todayUntil time.Time) (model.WordStats, error) {
	var stats model.WordStats

	err := r.db.QueryRowContext(ctx,
		`SELECT
		   COALESCE(SUM(archived = 0), 0),
		   COALESCE(SUM(archived = 1), 0),
		   COUNT(*),
		   COALESCE(SUM(CASE WHEN archived = 0 THEN review_count ELSE 0 END), 0),
		   COALESCE(SUM(archived = 0 AND translating = 1), 0),
		   COALESCE(SUM(CASE WHEN last_reviewed_at >= ? AND last_reviewed_at < ? THEN 1 ELSE 0 END), 0)
		 FROM words WHERE user_id = ?`,
		todaySince, todayUntil, userID,
	).Scan(&stats.TotalWords, &stats.ArchivedWords, &stats.TotalAllWords, &stats.TotalReviews, &stats.TranslatingCount, &stats.TodayReviews)
	if err != nil {
		return model.WordStats{}, err
	}

	buckets := make([]int, 5)
	err = r.db.QueryRowContext(ctx,
		`SELECT
		   COALESCE(SUM(review_count = 1), 0),
		   COALESCE(SUM(review_count BETWEEN 2 AND 3), 0),
		   COALESCE(SUM(review_count BETWEEN 4 AND 6), 0),
		   COALESCE(SUM(review_count BETWEEN 7 AND 10), 0),
		   COALESCE(SUM(review_count >= 11), 0)
		 FROM words WHERE user_id = ? AND archived = 0`,
		userID,
	).Scan(&buckets[0], &buckets[1], &buckets[2], &buckets[3], &buckets[4])
	if err != nil {
		return model.WordStats{}, err
	}
	stats.ReviewBuckets = buckets

	rows, err := r.db.QueryContext(ctx,
		`SELECT DATE_FORMAT(first_added_at, '%Y-%m-%d') d, COUNT(*)
		 FROM words WHERE user_id = ? AND archived = 0 AND first_added_at >= ?
		 GROUP BY d ORDER BY d`,
		userID, since,
	)
	if err != nil {
		return model.WordStats{}, err
	}
	defer rows.Close()

	stats.DailyAdditions = []model.DailyCount{}
	for rows.Next() {
		var count model.DailyCount
		if err := rows.Scan(&count.Date, &count.Count); err != nil {
			return model.WordStats{}, err
		}
		stats.DailyAdditions = append(stats.DailyAdditions, count)
	}
	if err := rows.Err(); err != nil {
		return model.WordStats{}, err
	}

	cloudRows, err := r.db.QueryContext(ctx,
		`SELECT display_word, review_count, senses FROM words
		 WHERE user_id = ? AND archived = 0 AND last_reviewed_at >= ?
		 ORDER BY review_count DESC, display_word ASC LIMIT 50`,
		userID, since7,
	)
	if err != nil {
		return model.WordStats{}, err
	}
	defer cloudRows.Close()

	stats.WordCloud = []model.WordCloudItem{}
	for cloudRows.Next() {
		var item model.WordCloudItem
		var sensesRaw []byte
		if err := cloudRows.Scan(&item.Word, &item.Count, &sensesRaw); err != nil {
			return model.WordStats{}, err
		}
		item.Meaning = cloudMeaning(sensesRaw)
		stats.WordCloud = append(stats.WordCloud, item)
	}
	if err := cloudRows.Err(); err != nil {
		return model.WordStats{}, err
	}

	letterRows, err := r.db.QueryContext(ctx,
		`SELECT UPPER(LEFT(display_word, 1)) l, COUNT(*) c FROM words
		 WHERE user_id = ? AND archived = 0
		 GROUP BY l ORDER BY l`,
		userID,
	)
	if err != nil {
		return model.WordStats{}, err
	}
	defer letterRows.Close()

	stats.LetterStats = []model.LetterStat{}
	for letterRows.Next() {
		var stat model.LetterStat
		if err := letterRows.Scan(&stat.Letter, &stat.Count); err != nil {
			return model.WordStats{}, err
		}
		stats.LetterStats = append(stats.LetterStats, stat)
	}
	if err := letterRows.Err(); err != nil {
		return model.WordStats{}, err
	}
	return stats, nil
}

// cloudMeaning 合并相同词性的释义后拼成词云 tooltip 文本；无效或空 JSON 返回空串。
func cloudMeaning(sensesRaw []byte) string {
	if len(sensesRaw) == 0 {
		return ""
	}
	var senses []model.Sense
	if err := json.Unmarshal(sensesRaw, &senses); err != nil {
		return ""
	}
	merged := model.MergeSensesByPos(senses)
	translations := make([]string, 0, len(merged))
	for _, sense := range merged {
		translations = append(translations, sense.Translation)
	}
	return strings.Join(translations, "；")
}

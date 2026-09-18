package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"vocab-backend/internal/model"
)

// DictionaryRepository 封装全局 word_dictionary 表的数据访问。
type DictionaryRepository struct {
	db *sql.DB
}

// NewDictionaryRepository 创建全局词库数据访问实现。
func NewDictionaryRepository(db *sql.DB) *DictionaryRepository {
	return &DictionaryRepository{db: db}
}

// VocabularyIndex 返回全量 word_key 到 occurrence_count 的轻量索引。
func (r *DictionaryRepository) VocabularyIndex(ctx context.Context) ([]model.VocabularyItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT word_key, occurrence_count FROM word_dictionary`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []model.VocabularyItem{}
	for rows.Next() {
		var item model.VocabularyItem
		if err := rows.Scan(&item.WordKey, &item.OccurrenceCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *DictionaryRepository) UpsertOccurrence(ctx context.Context, wordKey, displayWord string, now time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO word_dictionary (word_key, display_word, senses, occurrence_count, first_seen_at, last_updated_at)
		 VALUES (?, ?, JSON_ARRAY(), 1, ?, ?)
		 ON DUPLICATE KEY UPDATE occurrence_count = occurrence_count + 1, last_updated_at = ?`,
		wordKey, displayWord, now, now, now,
	)
	return err
}

// LookupSenses 返回缓存的原始 JSON；未找到时保留 sql.ErrNoRows 语义。
func (r *DictionaryRepository) LookupSenses(ctx context.Context, wordKey string) ([]byte, error) {
	var sensesRaw []byte
	err := r.db.QueryRowContext(ctx, `SELECT senses FROM word_dictionary WHERE word_key = ?`, wordKey).Scan(&sensesRaw)
	return sensesRaw, err
}

// SaveSenses 只在缓存为空或仍是缺少强化字段的旧格式时写入，避免并发查词互相覆盖。
func (r *DictionaryRepository) SaveSenses(ctx context.Context, wordKey string, sensesJSON []byte) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE word_dictionary SET senses = ?
		 WHERE word_key = ?
		   AND (senses IS NULL
		        OR JSON_LENGTH(senses) = 0
		        OR JSON_EXTRACT(senses, '$[0].phonetic') IS NULL
		        OR JSON_EXTRACT(senses, '$[0].phonetic') = '')`,
		sensesJSON, wordKey,
	)
	return err
}

// dictColumns 与 scanDictRows 的扫描顺序严格配对。
const dictColumns = `word_key, display_word, senses, occurrence_count, last_updated_at`

func scanDictRows(rows *sql.Rows) ([]model.DictionaryEntry, error) {
	defer rows.Close()

	entries := []model.DictionaryEntry{}
	for rows.Next() {
		var entry model.DictionaryEntry
		var sensesRaw []byte
		if err := rows.Scan(&entry.WordKey, &entry.DisplayWord, &sensesRaw, &entry.OccurrenceCount, &entry.LastUpdatedAt); err != nil {
			return nil, err
		}
		if len(sensesRaw) > 0 {
			if err := json.Unmarshal(sensesRaw, &entry.Senses); err != nil {
				return nil, err
			}
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// List 返回全量词库，供 CSV 导出。
func (r *DictionaryRepository) List(ctx context.Context) ([]model.DictionaryEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+dictColumns+` FROM word_dictionary ORDER BY last_updated_at DESC, word_key ASC`)
	if err != nil {
		return nil, err
	}
	return scanDictRows(rows)
}

// ListPage 在数据库侧完成关键字、释义状态过滤和分页。
func (r *DictionaryRepository) ListPage(ctx context.Context, keyword, status string, limit, offset int) ([]model.DictionaryEntry, error) {
	conditions, args := senseFilterWhere(keyword, status)
	where := ""
	if conditions != "" {
		where = " WHERE " + conditions
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+dictColumns+` FROM word_dictionary`+where+
			` ORDER BY last_updated_at DESC, word_key ASC LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	return scanDictRows(rows)
}

func (r *DictionaryRepository) Count(ctx context.Context, keyword, status string) (int, error) {
	conditions, args := senseFilterWhere(keyword, status)
	where := ""
	if conditions != "" {
		where = " WHERE " + conditions
	}
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM word_dictionary`+where, args...).Scan(&count)
	return count, err
}

func (r *DictionaryRepository) Delete(ctx context.Context, wordKey string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM word_dictionary WHERE word_key = ?`, wordKey)
	return err
}

// DeleteMany 批量删除缓存记录；值继续使用参数绑定，仅占位符按数量生成。
func (r *DictionaryRepository) DeleteMany(ctx context.Context, wordKeys []string) (int64, error) {
	if len(wordKeys) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(wordKeys)), ",")
	args := make([]interface{}, len(wordKeys))
	for index, wordKey := range wordKeys {
		args[index] = wordKey
	}
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM word_dictionary WHERE word_key IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// senseFilterWhere 生成 words 与 word_dictionary 共用的筛选条件和绑定参数。
func senseFilterWhere(keyword, status string) (string, []interface{}) {
	conditions := []string{}
	args := []interface{}{}
	if keyword != "" {
		like := likeContains(keyword)
		conditions = append(conditions, `(word_key LIKE ? OR JSON_SEARCH(senses, 'one', ?, NULL, '$.translation') IS NOT NULL)`)
		args = append(args, like, like)
	}
	switch status {
	case "no_definition":
		conditions = append(conditions, `(senses IS NULL OR JSON_LENGTH(senses) = 0)`)
	case "has_definition":
		conditions = append(conditions, `(senses IS NOT NULL AND JSON_LENGTH(senses) > 0)`)
	}
	if len(conditions) == 0 {
		return "", nil
	}
	return strings.Join(conditions, " AND "), args
}

// likeContains 构造包含匹配；空关键字保持原有的全匹配语义。
func likeContains(keyword string) string {
	if keyword == "" {
		return "%"
	}
	return "%" + escapeLikePattern(keyword) + "%"
}

// escapeLikePattern 转义 MySQL LIKE 元字符；反斜杠必须先转义，避免后续转义失效。
func escapeLikePattern(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

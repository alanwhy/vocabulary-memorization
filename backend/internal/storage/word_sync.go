// 个人母库同步：只补齐有实际释义的缺词，事务内保留所有已有学习记录。
package storage

import (
	"context"
	"time"
)

// SyncDictionaryToUser 将母库有非空、非错误释义的缺词复制给 userID，新增次数为 1，时间为 now。
// 返回实际新增数和包含归档词的总数；用户不存在返回 sql.ErrNoRows，其余错误回滚事务并交调用方处理。
func (r *WordRepository) SyncDictionaryToUser(ctx context.Context, userID int, now time.Time) (int64, int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	// 同一用户的同步串行化，并兼容没有用户外键约束的历史数据库。
	var id int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = ? FOR UPDATE`, userID).Scan(&id); err != nil {
		return 0, 0, err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO words (user_id, word_key, display_word, senses, review_count, first_added_at, last_reviewed_at)
		SELECT DISTINCT ?, d.word_key, d.display_word, d.senses, 1, ?, ?
		FROM word_dictionary d
		JOIN JSON_TABLE(
			CASE WHEN JSON_TYPE(d.senses) = 'ARRAY' THEN d.senses ELSE JSON_ARRAY() END,
			'$[*]' COLUMNS (
				sense_index FOR ORDINALITY,
				translation TEXT PATH '$.translation' NULL ON EMPTY NULL ON ERROR,
				pos VARCHAR(32) PATH '$.pos' NULL ON EMPTY NULL ON ERROR
			)
		) AS sense ON TRUE
		WHERE JSON_TYPE(JSON_EXTRACT(d.senses, CONCAT('$[', sense.sense_index - 1, '].translation'))) = 'STRING'
		AND COALESCE(sense.translation, '') REGEXP '[^[:space:]]'
		AND TRIM(COALESCE(sense.pos, '')) <> 'error'
		AND NOT EXISTS (SELECT 1 FROM words w WHERE w.user_id = ? AND w.word_key = d.word_key)
		ORDER BY d.word_key
		ON DUPLICATE KEY UPDATE id = words.id`, userID, now, now, userID)
	if err != nil {
		return 0, 0, err
	}
	added, err := res.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM words WHERE user_id = ?`, userID).Scan(&total); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return added, total, nil
}

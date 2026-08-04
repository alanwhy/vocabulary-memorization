package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// userRepo 封装 users 表的数据访问
type userRepo struct{ db *sql.DB }

func (r *userRepo) Insert(ctx context.Context, username, passwordHash string, isAdmin bool, now time.Time) (User, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, isAdmin, now,
	)
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{ID: int(id), Username: username, IsAdmin: isAdmin, CreatedAt: now}, nil
}

// FindByUsername 返回用户及其密码哈希；未找到时 err 为 sql.ErrNoRows
func (r *userRepo) FindByUsername(ctx context.Context, username string) (User, string, error) {
	var u User
	var hash string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, is_admin, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &hash, &u.IsAdmin, &u.CreatedAt)
	return u, hash, err
}

func (r *userRepo) FindPasswordHash(ctx context.Context, id int) (string, error) {
	var hash string
	err := r.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, id).Scan(&hash)
	return hash, err
}

func (r *userRepo) UpdatePasswordHash(ctx context.Context, id int, hash string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *userRepo) List(ctx context.Context) ([]User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, username, is_admin, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, nil
}

func (r *userRepo) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&count)
	return count, err
}

func (r *userRepo) FirstAdminID(ctx context.Context) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT id FROM users WHERE is_admin = 1 ORDER BY id LIMIT 1`).Scan(&id)
	return id, err
}

// sessionRepo 封装 sessions 表的数据访问
type sessionRepo struct{ db *sql.DB }

func (r *sessionRepo) Create(ctx context.Context, token string, userID int, expiresAt, createdAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		token, userID, expiresAt, createdAt,
	)
	return err
}

// FindWithUser 联表查询会话及其所属用户；未找到或已被清除时 err 为 sql.ErrNoRows
func (r *sessionRepo) FindWithUser(ctx context.Context, token string) (User, time.Time, error) {
	var u User
	var expiresAt time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.is_admin, u.created_at, s.expires_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ?`,
		token,
	).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &expiresAt)
	return u, expiresAt, err
}

func (r *sessionRepo) Touch(ctx context.Context, token string, newExpiry time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE token = ?`, newExpiry, token)
	return err
}

func (r *sessionRepo) DeleteByToken(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (r *sessionRepo) DeleteByUser(ctx context.Context, userID int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (r *sessionRepo) DeleteByUserExcept(ctx context.Context, userID int, exceptToken string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token != ?`, userID, exceptToken)
	return err
}

// wordRepo 封装 words 表的数据访问
type wordRepo struct{ db *sql.DB }

func (r *wordRepo) Insert(ctx context.Context, userID int, wordKey, displayWord string, sensesJSON []byte, translating bool, now time.Time) (int, error) {
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

// FindByUserAndKey 未找到时 err 为 sql.ErrNoRows
func (r *wordRepo) FindByUserAndKey(ctx context.Context, userID int, wordKey string) (Word, []byte, error) {
	var wd Word
	var sensesRaw []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, word_key, display_word, senses, translating, archived, review_count, first_added_at, last_reviewed_at FROM words WHERE user_id = ? AND word_key = ?`,
		userID, wordKey,
	).Scan(&wd.ID, &wd.WordKey, &wd.DisplayWord, &sensesRaw, &wd.Translating, &wd.Archived, &wd.ReviewCount, &wd.FirstAddedAt, &wd.LastReviewedAt)
	return wd, sensesRaw, err
}

func (r *wordRepo) IncrementReview(ctx context.Context, id, newCount int, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE words SET review_count = ?, last_reviewed_at = ? WHERE id = ?`, newCount, now, id)
	return err
}

func (r *wordRepo) List(ctx context.Context, userID int, archived bool) ([]Word, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, word_key, display_word, senses, translating, archived, review_count, first_added_at, last_reviewed_at FROM words WHERE user_id = ? AND archived = ? ORDER BY review_count DESC, last_reviewed_at DESC`,
		userID, archived,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Word{}
	for rows.Next() {
		var wd Word
		var sensesRaw []byte
		if err := rows.Scan(&wd.ID, &wd.WordKey, &wd.DisplayWord, &sensesRaw, &wd.Translating, &wd.Archived, &wd.ReviewCount, &wd.FirstAddedAt, &wd.LastReviewedAt); err != nil {
			return nil, err
		}
		if len(sensesRaw) > 0 {
			if err := json.Unmarshal(sensesRaw, &wd.Senses); err != nil {
				return nil, err
			}
		}
		list = append(list, wd)
	}
	return list, nil
}

func (r *wordRepo) Delete(ctx context.Context, id, userID int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM words WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *wordRepo) SetArchived(ctx context.Context, id, userID int, archived bool) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE words SET archived = ? WHERE id = ? AND user_id = ?`, archived, id, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *wordRepo) UpdateSenses(ctx context.Context, id int, sensesJSON []byte) error {
	_, err := r.db.ExecContext(ctx, `UPDATE words SET senses = ?, translating = 0 WHERE id = ?`, sensesJSON, id)
	return err
}

// FindTranslating 返回所有 translating=1 的记录（进程重启前未完成的查词任务）
func (r *wordRepo) FindTranslating(ctx context.Context) ([]Word, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, word_key FROM words WHERE translating = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Word{}
	for rows.Next() {
		var wd Word
		if err := rows.Scan(&wd.ID, &wd.WordKey); err != nil {
			return nil, err
		}
		list = append(list, wd)
	}
	return list, nil
}

// dictionaryRepo 封装 word_dictionary 表的数据访问
type dictionaryRepo struct{ db *sql.DB }

func (r *dictionaryRepo) UpsertOccurrence(ctx context.Context, wordKey, displayWord string, now time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO word_dictionary (word_key, display_word, senses, occurrence_count, first_seen_at, last_updated_at)
		 VALUES (?, ?, JSON_ARRAY(), 1, ?, ?)
		 ON DUPLICATE KEY UPDATE occurrence_count = occurrence_count + 1, last_updated_at = ?`,
		wordKey, displayWord, now, now, now,
	)
	return err
}

// LookupSenses 未找到或未缓存时 err 为 sql.ErrNoRows / raw 为空
func (r *dictionaryRepo) LookupSenses(ctx context.Context, wordKey string) ([]byte, error) {
	var sensesRaw []byte
	err := r.db.QueryRowContext(ctx, `SELECT senses FROM word_dictionary WHERE word_key = ?`, wordKey).Scan(&sensesRaw)
	return sensesRaw, err
}

func (r *dictionaryRepo) SaveSenses(ctx context.Context, wordKey string, sensesJSON []byte) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE word_dictionary SET senses = ? WHERE word_key = ? AND (senses IS NULL OR JSON_LENGTH(senses) = 0)`,
		sensesJSON, wordKey,
	)
	return err
}

func (r *dictionaryRepo) List(ctx context.Context) ([]dictionaryEntry, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT word_key, display_word, senses, last_updated_at FROM word_dictionary ORDER BY last_updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []dictionaryEntry{}
	for rows.Next() {
		var e dictionaryEntry
		var sensesRaw []byte
		if err := rows.Scan(&e.WordKey, &e.DisplayWord, &sensesRaw, &e.LastUpdatedAt); err != nil {
			return nil, err
		}
		if len(sensesRaw) > 0 {
			if err := json.Unmarshal(sensesRaw, &e.Senses); err != nil {
				return nil, err
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (r *dictionaryRepo) Delete(ctx context.Context, wordKey string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM word_dictionary WHERE word_key = ?`, wordKey)
	return err
}

// settingsRepo 封装 settings 表的数据访问
type settingsRepo struct{ db *sql.DB }

func (r *settingsRepo) SeedIfMissing(ctx context.Context, name, value string) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE name = ?`, name).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO settings (name, value) VALUES (?, ?)`, name, value)
	return err
}

func (r *settingsRepo) LoadValues(ctx context.Context, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	args := make([]interface{}, len(names))
	for i, n := range names {
		args[i] = n
	}
	rows, err := r.db.QueryContext(ctx, `SELECT name, value FROM settings WHERE name IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := map[string]string{}
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		values[name] = value
	}
	return values, nil
}

// UpsertMany 在单个事务里原子更新多条配置
func (r *settingsRepo) UpsertMany(ctx context.Context, updates map[string]string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for name, value := range updates {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (name, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = ?`, name, value, value); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

package storage

import (
	"context"
	"database/sql"
	"time"

	"vocab-backend/internal/model"
)

// SessionRepository 封装 sessions 表的数据访问。
type SessionRepository struct {
	db *sql.DB
}

// NewSessionRepository 创建会话数据访问实现。
func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, token string, userID int, expiresAt, createdAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		token, userID, expiresAt, createdAt,
	)
	return err
}

// FindWithUser 联表查询会话及其所属用户；未找到或已被清除时 err 为 sql.ErrNoRows。
func (r *SessionRepository) FindWithUser(ctx context.Context, token string) (model.User, time.Time, error) {
	var user model.User
	var expiresAt time.Time
	var lastLogin sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.is_admin, u.disabled, u.created_at, u.last_login_at, s.expires_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ?`,
		token,
	).Scan(&user.ID, &user.Username, &user.IsAdmin, &user.Disabled, &user.CreatedAt, &lastLogin, &expiresAt)
	user.LastLoginAt = nullTimePtr(lastLogin)
	return user, expiresAt, err
}

func (r *SessionRepository) Touch(ctx context.Context, token string, newExpiry time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE token = ?`, newExpiry, token)
	return err
}

func (r *SessionRepository) DeleteByToken(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (r *SessionRepository) DeleteByUser(ctx context.Context, userID int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (r *SessionRepository) DeleteByUserExcept(ctx context.Context, userID int, exceptToken string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token != ?`, userID, exceptToken)
	return err
}

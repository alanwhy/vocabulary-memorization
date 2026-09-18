package storage

import (
	"context"
	"database/sql"
	"time"

	"vocab-backend/internal/model"
)

// UserRepository 封装 users 表的数据访问。
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository 创建用户数据访问实现；连接生命周期仍由组合根统一管理。
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Insert(ctx context.Context, username, passwordHash string, isAdmin bool, now time.Time) (model.User, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, isAdmin, now,
	)
	if err != nil {
		return model.User{}, err
	}
	id, _ := res.LastInsertId()
	return model.User{ID: int(id), Username: username, IsAdmin: isAdmin, CreatedAt: now}, nil
}

// FindByUsername 返回用户及其密码哈希；未找到时 err 为 sql.ErrNoRows。
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (model.User, string, error) {
	var user model.User
	var hash string
	var lastLogin sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, is_admin, disabled, created_at, last_login_at FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &hash, &user.IsAdmin, &user.Disabled, &user.CreatedAt, &lastLogin)
	user.LastLoginAt = nullTimePtr(lastLogin)
	return user, hash, err
}

// RecordLogin 记录一次成功登录的时间。
func (r *UserRepository) RecordLogin(ctx context.Context, id int, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, now, id)
	return err
}

func (r *UserRepository) FindPasswordHash(ctx context.Context, id int) (string, error) {
	var hash string
	err := r.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, id).Scan(&hash)
	return hash, err
}

func (r *UserRepository) UpdatePasswordHash(ctx context.Context, id int, hash string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SetDisabled 把用户标记为禁用或启用，返回受影响行数；0 表示用户不存在。
func (r *UserRepository) SetDisabled(ctx context.Context, id int, disabled bool) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET disabled = ? WHERE id = ?`, disabled, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Delete 删除用户；调用方会先清理会话与单词，因为老库没有外键级联。
func (r *UserRepository) Delete(ctx context.Context, id int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// List 一次聚合每个用户录入的单词数，避免 Handler 对每个用户分别执行 COUNT。
func (r *UserRepository) List(ctx context.Context) ([]model.UserWithStats, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT u.id, u.username, u.is_admin, u.disabled, u.created_at, u.last_login_at, COUNT(w.id)
		 FROM users u LEFT JOIN words w ON w.user_id = u.id
		 GROUP BY u.id, u.username, u.is_admin, u.disabled, u.created_at, u.last_login_at
		 ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []model.UserWithStats{}
	for rows.Next() {
		var user model.UserWithStats
		var lastLogin sql.NullTime
		if err := rows.Scan(&user.ID, &user.Username, &user.IsAdmin, &user.Disabled, &user.CreatedAt, &lastLogin, &user.WordCount); err != nil {
			return nil, err
		}
		user.LastLoginAt = nullTimePtr(lastLogin)
		list = append(list, user)
	}
	return list, rows.Err()
}

func (r *UserRepository) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&count)
	return count, err
}

func (r *UserRepository) FirstAdminID(ctx context.Context) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT id FROM users WHERE is_admin = 1 ORDER BY id LIMIT 1`).Scan(&id)
	return id, err
}

// nullTimePtr 将可为 NULL 的数据库时间转换成模型使用的指针；NULL 在 JSON 中保持 null。
func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

package storage

import (
	"context"
	"database/sql"
	"strings"
)

// SettingsRepository 封装 settings 表的数据访问。
type SettingsRepository struct {
	db *sql.DB
}

// NewSettingsRepository 创建配置数据访问实现。
func NewSettingsRepository(db *sql.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) SeedIfMissing(ctx context.Context, name, value string) error {
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

func (r *SettingsRepository) LoadValues(ctx context.Context, names []string) (map[string]string, error) {
	if len(names) == 0 {
		return map[string]string{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	args := make([]interface{}, len(names))
	for index, name := range names {
		args[index] = name
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

// UpsertMany 在单个事务里原子更新多条配置。
func (r *SettingsRepository) UpsertMany(ctx context.Context, updates map[string]string) error {
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

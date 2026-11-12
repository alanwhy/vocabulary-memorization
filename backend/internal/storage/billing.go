// 金额预留与结算使用短 MySQL 事务；供应商 HTTP 请求不在事务中执行。
package storage

import (
	"context"
	"database/sql"
	"errors"
	"vocab-backend/internal/billing"
)

type BillingRepository struct{ db *sql.DB }

// NewBillingRepository 注入共享数据库连接，账本在进程重启后继续有效。
func NewBillingRepository(db *sql.DB) *BillingRepository { return &BillingRepository{db} }

// WordOwner 从持久化单词取得用户归属，恢复任务不能丢失计费主体。
func (r *BillingRepository) WordOwner(ctx context.Context, id int) (int, error) {
	var owner int
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM words WHERE id=?`, id).Scan(&owner)
	return owner, err
}

// Snapshot 读取实时额度与当日使用；新日没有行时按零显示，不修改旧账本。
func (r *BillingRepository) Snapshot(ctx context.Context, id int, date string) (billing.Snapshot, error) {
	s := billing.Snapshot{Date: date}
	var limit sql.NullInt64
	var disabled bool
	err := r.db.QueryRowContext(ctx, `SELECT daily_ai_limit_nano,disabled FROM users WHERE id=?`, id).Scan(&limit, &disabled)
	if err != nil {
		return s, err
	}
	if limit.Valid {
		n := limit.Int64
		s.Limit = &n
	}
	err = r.db.QueryRowContext(ctx, `SELECT spent_nano,reserved_nano FROM ai_daily_usage WHERE user_id=? AND usage_date=?`, id, date).Scan(&s.Spent, &s.Reserved)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_requests WHERE user_id=? AND usage_date=? AND state IN ('reserved','uncertain')`, id, date).Scan(&s.Pending)
	return s, err
}

// Reserve 锁用户和日账本后检查额度，父工作流预留可转移到子请求但不能重复占用。
func (r *BillingRepository) Reserve(ctx context.Context, b billing.Reservation) error {
	if b.Amount < 0 || b.Amount > billing.MaxLimit || b.UserID <= 0 {
		return billing.ErrUnavailable
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var limit sql.NullInt64
	var disabled bool
	err = tx.QueryRowContext(ctx, `SELECT daily_ai_limit_nano,disabled FROM users WHERE id=? FOR UPDATE`, b.UserID).Scan(&limit, &disabled)
	if errors.Is(err, sql.ErrNoRows) || err == nil && disabled {
		return billing.ErrDisabled
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO ai_daily_usage(user_id,usage_date) VALUES (?,?) ON DUPLICATE KEY UPDATE user_id=user_id`, b.UserID, b.Date); err != nil {
		return err
	}
	var spent, reserved int64
	if err = tx.QueryRowContext(ctx, `SELECT spent_nano,reserved_nano FROM ai_daily_usage WHERE user_id=? AND usage_date=? FOR UPDATE`, b.UserID, b.Date).Scan(&spent, &reserved); err != nil {
		return err
	}
	if b.Unpriced && limit.Valid {
		return billing.ErrUnavailable
	}
	extra := b.Amount
	if b.ParentID != "" {
		var remaining int64
		var state string
		var parentUser int
		var parentDay string
		err = tx.QueryRowContext(ctx, `SELECT reserved_nano,state,user_id,DATE_FORMAT(usage_date,'%Y-%m-%d') FROM ai_requests WHERE request_id=? FOR UPDATE`, b.ParentID).Scan(&remaining, &state, &parentUser, &parentDay)
		if err != nil {
			return err
		}
		if state != "reserved" || parentUser != b.UserID || parentDay != b.Date || remaining < b.Amount {
			return billing.ErrExceeded
		}
		extra = 0
	}
	// 从不限额改为有限额后，尚未确认价格的零金额请求不能被当成免费。
	if limit.Valid {
		var unpriced int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_requests WHERE user_id=? AND usage_date=? AND state IN ('reserved','uncertain') AND reserved_nano=0 AND RIGHT(purpose,9)<>'_workflow'`, b.UserID, b.Date).Scan(&unpriced); err != nil {
			return err
		}
		if unpriced > 0 {
			return billing.ErrUnavailable
		}
	}
	if limit.Valid && (limit.Int64 == 0 || spent+reserved+extra > limit.Int64) {
		return billing.ErrExceeded
	}
	if b.ParentID != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE ai_requests SET reserved_nano=reserved_nano-? WHERE request_id=?`, b.Amount, b.ParentID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO ai_requests(request_id,parent_id,user_id,usage_date,purpose,provider,model,reserved_nano,state,details) VALUES (?,?,?,?,?,?,?,?, 'reserved',?)`, b.ID, b.ParentID, b.UserID, b.Date, b.Purpose, b.Provider, b.Model, b.Amount, string(b.Details)); err != nil {
		return err
	}
	if extra != 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE ai_daily_usage SET reserved_nano=reserved_nano+? WHERE user_id=? AND usage_date=?`, extra, b.UserID, b.Date); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Settle 只结算未完成请求；未知费用保留预留，重复完成请求不会重复扣费。
func (r *BillingRepository) Settle(ctx context.Context, id string, amount int64, state string, details []byte) error {
	if amount < 0 || amount > billing.MaxLimit {
		return billing.ErrUnavailable
	}
	if state != "settled" && state != "released" && state != "uncertain" {
		return billing.ErrUnavailable
	}
	var user int
	var date string
	if err := r.db.QueryRowContext(ctx, `SELECT user_id,DATE_FORMAT(usage_date,'%Y-%m-%d') FROM ai_requests WHERE request_id=?`, id).Scan(&user, &date); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 与 Reserve 同一顺序锁日账本，再锁请求，避免子请求与结算互相死锁。
	var dailyReserved int64
	if err = tx.QueryRowContext(ctx, `SELECT reserved_nano FROM ai_daily_usage WHERE user_id=? AND usage_date=? FOR UPDATE`, user, date).Scan(&dailyReserved); err != nil {
		return err
	}
	var reserved int64
	var current string
	if err = tx.QueryRowContext(ctx, `SELECT reserved_nano,state FROM ai_requests WHERE request_id=? FOR UPDATE`, id).Scan(&reserved, &current); err != nil {
		return err
	}
	if current != "reserved" && current != "uncertain" {
		return tx.Commit()
	}
	if state == "uncertain" {
		_, err = tx.ExecContext(ctx, `UPDATE ai_requests SET state='uncertain' WHERE request_id=?`, id)
	} else {
		if reserved > dailyReserved || amount > reserved {
			return billing.ErrUnavailable
		}
		if _, err = tx.ExecContext(ctx, `UPDATE ai_daily_usage SET spent_nano=spent_nano+?,reserved_nano=reserved_nano-? WHERE user_id=? AND usage_date=?`, amount, reserved, user, date); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE ai_requests SET state=?,actual_nano=?,reserved_nano=0,details=COALESCE(?,details) WHERE request_id=?`, state, amount, nullableDetails(details), id)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// nullableDetails 缺结算明细时保留预留费率，返回可空 SQL 参数。
func nullableDetails(details []byte) any {
	if len(details) == 0 {
		return nil
	}
	return string(details)
}

package storage

import (
	"context"
	"database/sql"
	"errors"
	"github.com/go-sql-driver/mysql"
	"os"
	"strings"
	"sync"
	"testing"
	"vocab-backend/internal/billing"
)

// TestBillingMySQL 使用独立测试库验证真实 InnoDB 并发预留、幂等结算和调额。
// AI_BILLING_TEST_DSN 库名必须以 vocab_ai_billing_test 开头，避免操作业务库。
func TestBillingMySQL(t *testing.T) {
	dsn := os.Getenv("AI_BILLING_TEST_DSN")
	if dsn == "" {
		t.Skip("AI_BILLING_TEST_DSN 未设置；真实 MySQL 事务待验证")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(cfg.DBName, "vocab_ai_billing_test") {
		t.Fatal("必须使用独立 vocab_ai_billing_test 测试库")
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{`CREATE TABLE IF NOT EXISTS users(id INT PRIMARY KEY,is_admin BOOL NOT NULL DEFAULT 0,disabled BOOL NOT NULL DEFAULT 0) ENGINE=InnoDB`, `CREATE TABLE IF NOT EXISTS settings(name VARCHAR(64) PRIMARY KEY,value TEXT) ENGINE=InnoDB`, `DELETE FROM users`} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	migrateAIBilling(db)
	for _, query := range []string{`DELETE FROM ai_requests`, `DELETE FROM ai_daily_usage`, `INSERT INTO users(id,daily_ai_limit_nano) VALUES (1,5000000000)`} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	r := NewBillingRepository(db)
	ctx := context.Background()
	date := "2026-11-12"
	if err = r.Reserve(ctx, billing.Reservation{ID: "unpriced", UserID: 1, Date: date, Unpriced: true, Details: []byte(`{}`)}); !errors.Is(err, billing.ErrUnavailable) {
		t.Fatal("unpriced finite request", err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"one", "two"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			results <- r.Reserve(ctx, billing.Reservation{ID: id, UserID: 1, Date: date, Amount: 3 * billing.NanoPerYuan, Details: []byte(`{}`)})
		}(id)
	}
	wg.Wait()
	close(results)
	ok, blocked := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, billing.ErrExceeded) {
			blocked++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || blocked != 1 {
		t.Fatal("concurrent overspend", ok, blocked)
	}
	var id string
	if err = db.QueryRow(`SELECT request_id FROM ai_requests LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = r.Settle(ctx, id, billing.NanoPerYuan, "settled", []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	s, err := r.Snapshot(ctx, 1, date)
	if err != nil || s.Spent != billing.NanoPerYuan || s.Reserved != 0 {
		t.Fatal(s, err)
	}
	if _, err = db.Exec(`UPDATE users SET daily_ai_limit_nano=0 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if err = r.Reserve(ctx, billing.Reservation{ID: "zero", UserID: 1, Date: date, Amount: 1, Details: []byte(`{}`)}); !errors.Is(err, billing.ErrExceeded) {
		t.Fatal(err)
	}
}

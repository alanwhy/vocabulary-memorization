package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// TestSyncDictionaryToUserMySQL 使用显式隔离库验证真实 SQL；未配置时跳过，不触碰业务库。
// WORD_SYNC_TEST_DSN 的数据库名必须以 vocab_word_sync_test 开头，夹具会清空该库内三个测试表。
func TestSyncDictionaryToUserMySQL(t *testing.T) {
	dsn := os.Getenv("WORD_SYNC_TEST_DSN")
	if dsn == "" {
		t.Skip("WORD_SYNC_TEST_DSN 未配置；真实 MySQL 同步效果待手动验收")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(cfg.DBName, "vocab_word_sync_test") {
		t.Fatal("拒绝使用非专用测试库")
	}
	cfg.ParseTime = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS users (id INT PRIMARY KEY) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS word_dictionary (word_key VARCHAR(255) PRIMARY KEY, display_word VARCHAR(255) NOT NULL, senses JSON, occurrence_count INT NOT NULL) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS words (
			id INT AUTO_INCREMENT PRIMARY KEY, user_id INT NOT NULL, word_key VARCHAR(255) NOT NULL,
			display_word VARCHAR(255) NOT NULL, senses JSON, review_count INT NOT NULL DEFAULT 1,
			first_added_at DATETIME NOT NULL, last_reviewed_at DATETIME NOT NULL,
			archived TINYINT NOT NULL DEFAULT 0, important_glosses JSON,
			due_at DATETIME NULL, interval_days INT NOT NULL DEFAULT 0, ease_factor DECIMAL(4,2) NOT NULL DEFAULT 2.50,
			translating TINYINT NOT NULL DEFAULT 0, UNIQUE KEY uniq_user_word(user_id, word_key)
		) ENGINE=InnoDB`,
		`DELETE FROM words`, `DELETE FROM word_dictionary`, `DELETE FROM users`,
		`INSERT INTO users VALUES (1), (2)`,
		`INSERT INTO word_dictionary VALUES
			('apple', 'Apple', '[{"pos":"n.","translation":"苹果"}]', 50),
			('banana', 'Banana', NULL, 20), ('pear', 'Pear', '[]', 10)`,
		`INSERT INTO words (user_id, word_key, display_word, senses, review_count, first_added_at, last_reviewed_at, archived, important_glosses, due_at, interval_days, ease_factor)
			VALUES (1, 'apple', 'my apple', '[{"translation":"个人释义"}]', 9, '2026-01-01', '2026-02-01', 1, '["个人释义"]', '2026-12-01', 30, 2.80),
			(2, 'banana', 'other banana', NULL, 7, '2026-01-01', '2026-02-01', 0, NULL, NULL, 0, 2.50)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	r := NewWordRepository(db)
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	added, total, err := r.SyncDictionaryToUser(ctx, 1, now)
	if err != nil || added != 2 || total != 3 {
		t.Fatalf("added=%d total=%d err=%v", added, total, err)
	}
	var preserved, newWords, otherUser, dictCount int
	checks := []struct {
		query string
		dest  *int
		want  int
	}{
		{`SELECT COUNT(*) FROM words WHERE user_id=1 AND word_key='apple' AND review_count=9 AND archived=1 AND display_word='my apple'
			AND JSON_UNQUOTE(JSON_EXTRACT(senses,'$[0].translation'))='个人释义' AND JSON_CONTAINS(important_glosses, '"个人释义"')
			AND first_added_at='2026-01-01' AND last_reviewed_at='2026-02-01' AND due_at='2026-12-01' AND interval_days=30 AND ease_factor=2.80`, &preserved, 1},
		{`SELECT COUNT(*) FROM words WHERE user_id=1 AND word_key IN ('banana','pear') AND review_count=1 AND archived=0 AND interval_days=0
			AND first_added_at='2026-11-12 10:00:00' AND last_reviewed_at='2026-11-12 10:00:00'`, &newWords, 2},
		{`SELECT COUNT(*) FROM words WHERE user_id=2 AND review_count=7`, &otherUser, 1},
		{`SELECT SUM(occurrence_count) FROM word_dictionary`, &dictCount, 80},
	}
	for _, check := range checks {
		if err := db.QueryRow(check.query).Scan(check.dest); err != nil || *check.dest != check.want {
			t.Fatalf("check=%s got=%d err=%v", check.query, *check.dest, err)
		}
	}
	if added, total, err := r.SyncDictionaryToUser(ctx, 1, now); err != nil || added != 0 || total != 3 {
		t.Fatalf("repeat added=%d total=%d err=%v", added, total, err)
	}
	if _, _, err := r.SyncDictionaryToUser(ctx, 999, now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing user: %v", err)
	}

	// 同一目标并发请求应只创建一份新词，且复制母库释义。
	if _, err := db.Exec(`INSERT INTO word_dictionary VALUES ('grape', 'Grape', '[{"translation":"葡萄"}]', 15)`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan int64, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			added, total, err := r.SyncDictionaryToUser(ctx, 1, now)
			if err != nil || total != 4 {
				t.Errorf("concurrent total=%d err=%v", total, err)
			}
			results <- added
		}()
	}
	wg.Wait()
	close(results)
	var sum int64
	for n := range results {
		sum += n
	}
	if sum != 1 {
		t.Fatalf("concurrent added=%d", sum)
	}
	var copied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id=1 AND word_key='grape' AND review_count=1 AND display_word='Grape' AND JSON_UNQUOTE(JSON_EXTRACT(senses,'$[0].translation'))='葡萄'`).Scan(&copied); err != nil || copied != 1 {
		t.Fatalf("copied=%d err=%v", copied, err)
	}
	if _, err := db.Exec(`DELETE FROM word_dictionary`); err != nil {
		t.Fatal(err)
	}
	if added, total, err := r.SyncDictionaryToUser(ctx, 1, now); err != nil || added != 0 || total != 4 {
		t.Fatalf("empty dictionary added=%d total=%d err=%v", added, total, err)
	}

	// 插入阶段失败时整个事务回滚。
	if _, err := db.Exec(`INSERT INTO word_dictionary VALUES ('rollback', 'Rollback', NULL, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_word_sync BEFORE INSERT ON words FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test insert failure'`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DROP TRIGGER reject_word_sync`)
	if _, _, err := r.SyncDictionaryToUser(ctx, 1, now); err == nil {
		t.Fatal("expected insert failure")
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM words WHERE user_id=1`).Scan(&remaining); err != nil || remaining != 4 {
		t.Fatalf("rollback count=%d err=%v", remaining, err)
	}
}

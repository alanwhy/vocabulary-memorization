package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var db *sql.DB

// connectDB 建立数据库连接，容器刚启动时 MySQL 可能还没就绪，做几次重试
func connectDB() {
	host := getEnv("DB_HOST", "mysql")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "vocab")
	password := getEnv("DB_PASSWORD", "")
	name := getEnv("DB_NAME", "vocab")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local", user, password, host, port, name)

	var err error
	for attempt := 1; attempt <= 30; attempt++ {
		db, err = sql.Open("mysql", dsn)
		if err == nil {
			err = db.Ping()
		}
		if err == nil {
			log.Println("数据库连接成功")
			return
		}
		log.Printf("数据库连接失败（第 %d 次重试）: %v", attempt, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("多次重试后仍无法连接数据库: %v", err)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// migrateSchema 幂等迁移：新装机器上 schema.sql 已经建好全部表，这里的语句都是空操作；
// 老部署（只有最初的 words 表）在这里补齐 users/sessions/settings 表，并给 words 表加上
// user_id、senses 列，把历史数据迁移过去。
func migrateSchema() {
	mustExec(`CREATE TABLE IF NOT EXISTS users (
		id INT AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(64) NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		is_admin TINYINT(1) NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		UNIQUE KEY uniq_username (username)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	mustExec(`CREATE TABLE IF NOT EXISTS sessions (
		token VARCHAR(64) PRIMARY KEY,
		user_id INT NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	mustExec(`CREATE TABLE IF NOT EXISTS settings (
		name VARCHAR(64) PRIMARY KEY,
		value TEXT
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	migrateWordsSenses()
	migrateWordsUserIDColumn()
	migrateWordsTranslatingColumn()
	migrateWordsArchivedColumn()
}

// migrateWordsSenses 给 words 表补 senses 列，并把老的 pos/translation 单值列打包迁移过去
func migrateWordsSenses() {
	if columnExists("words", "senses") {
		return
	}
	mustExec(`ALTER TABLE words ADD COLUMN senses JSON NULL`)

	if columnExists("words", "pos") && columnExists("words", "translation") {
		mustExec(`UPDATE words SET senses = JSON_ARRAY(JSON_OBJECT('pos', IFNULL(pos, ''), 'translation', IFNULL(translation, ''))) WHERE senses IS NULL`)
	}
}

// migrateWordsUserIDColumn 给 words 表补 user_id 列：老数据统一归到超管账号名下，
// 并把唯一键从单纯的 word_key 换成 (user_id, word_key)
func migrateWordsUserIDColumn() {
	if columnExists("words", "user_id") {
		return
	}
	mustExec(`ALTER TABLE words ADD COLUMN user_id INT NULL AFTER id`)
}

// migrateWordsTranslatingColumn 给 words 表补 translating 列，标记查词是否还在后台异步进行中
func migrateWordsTranslatingColumn() {
	if columnExists("words", "translating") {
		return
	}
	mustExec(`ALTER TABLE words ADD COLUMN translating TINYINT(1) NOT NULL DEFAULT 0`)
}

// migrateWordsArchivedColumn 给 words 表补 archived 列，标记单词是否已被用户归档
func migrateWordsArchivedColumn() {
	if columnExists("words", "archived") {
		return
	}
	mustExec(`ALTER TABLE words ADD COLUMN archived TINYINT(1) NOT NULL DEFAULT 0`)
}

// finalizeWordsUserID 在超管账号创建完成后调用：把迁移阶段遗留的、还没有归属的历史单词
// 挂到超管账号下，然后把 user_id 收紧为 NOT NULL，并修正唯一键
func finalizeWordsUserID(adminID int) {
	if _, err := db.Exec(`UPDATE words SET user_id = ? WHERE user_id IS NULL`, adminID); err != nil {
		log.Fatalf("迁移历史单词归属失败: %v", err)
	}
	mustExec(`ALTER TABLE words MODIFY COLUMN user_id INT NOT NULL`)

	if indexExists("words", "uniq_word_key") {
		mustExec(`ALTER TABLE words DROP INDEX uniq_word_key`)
	}
	if !indexExists("words", "uniq_user_word") {
		mustExec(`ALTER TABLE words ADD UNIQUE KEY uniq_user_word (user_id, word_key)`)
	}
}

func columnExists(table, column string) bool {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		table, column,
	).Scan(&count)
	if err != nil {
		log.Fatalf("检查列是否存在失败 table=%s column=%s: %v", table, column, err)
	}
	return count > 0
}

func indexExists(table, indexName string) bool {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
		table, indexName,
	).Scan(&count)
	if err != nil {
		log.Fatalf("检查索引是否存在失败 table=%s index=%s: %v", table, indexName, err)
	}
	return count > 0
}

func mustExec(query string) {
	if _, err := db.Exec(query); err != nil {
		log.Fatalf("数据库迁移失败: %v\nSQL: %s", err, query)
	}
}

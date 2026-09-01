package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
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
			db.SetMaxOpenConns(getEnvInt("DB_MAX_OPEN_CONNS", 25))
			db.SetMaxIdleConns(getEnvInt("DB_MAX_IDLE_CONNS", 25))
			db.SetConnMaxLifetime(5 * time.Minute)
			db.SetConnMaxIdleTime(2 * time.Minute)
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

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
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

	mustExec(`CREATE TABLE IF NOT EXISTS word_dictionary (
		word_key VARCHAR(255) NOT NULL PRIMARY KEY,
		display_word VARCHAR(255) NOT NULL,
		senses JSON,
		occurrence_count INT NOT NULL DEFAULT 1,
		first_seen_at DATETIME NOT NULL,
		last_updated_at DATETIME NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	migrateWordsSenses()
	migrateWordsUserIDColumn()
	migrateWordsTranslatingColumn()
	migrateWordsTranslationStartedColumn()
	migrateWordsArchivedColumn()
	migrateWordsUserArchivedIndex()
	migrateWordsSRSColumns()
	migrateWordsImportantGlossesColumn()
	migrateUsersLastLoginColumn()
	migrateUsersDisabledColumn()
	mergeHistoricalWordSenses()
	backfillWordDictionary()
	migrateTimestampsToCST()
}

// migrateUsersLastLoginColumn 给 users 表补 last_login_at 列，记录最后一次成功登录的时间；
// 历史用户在下次登录前保持 NULL，前端据此显示“从未登录”
func migrateUsersLastLoginColumn() {
	if columnExists("users", "last_login_at") {
		return
	}
	mustExec(`ALTER TABLE users ADD COLUMN last_login_at DATETIME NULL`)
}

// migrateUsersDisabledColumn 给 users 表补 disabled 列，标记账号是否被管理员禁用。
// 历史用户默认 0（正常），禁用后登录被拒、已有会话立即失效。
func migrateUsersDisabledColumn() {
	if columnExists("users", "disabled") {
		return
	}
	mustExec(`ALTER TABLE users ADD COLUMN disabled TINYINT(1) NOT NULL DEFAULT 0`)
}

// migrateWordsUserArchivedIndex 给 words 表补 (user_id, archived) 索引，
// 分页列表和统计聚合都按这两列过滤，没有索引时会退化成全表扫描
func migrateWordsUserArchivedIndex() {
	if indexExists("words", "idx_words_user_archived") {
		return
	}
	mustExec(`ALTER TABLE words ADD KEY idx_words_user_archived (user_id, archived)`)
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

// migrateWordsTranslationStartedColumn 给 words 表补 translation_started_at 列，
// 记录最近一轮查词任务的开始时间，用于周期性扫描那些 goroutine 异常退出后永久卡死的任务
func migrateWordsTranslationStartedColumn() {
	if columnExists("words", "translation_started_at") {
		return
	}
	mustExec(`ALTER TABLE words ADD COLUMN translation_started_at DATETIME NULL`)
}

// migrateWordsArchivedColumn 给 words 表补 archived 列，标记单词是否已被用户归档
func migrateWordsArchivedColumn() {
	if columnExists("words", "archived") {
		return
	}
	mustExec(`ALTER TABLE words ADD COLUMN archived TINYINT(1) NOT NULL DEFAULT 0`)
}

// migrateWordsSRSColumns 给 words 表补间隔重复（SRS）的三列：due_at（下次到期时间，NULL=从未用闪卡复习过）、
// interval_days（当前间隔天数）、ease_factor（难度系数）。老词升级后 due_at 保持 NULL，
// 视为「从未用闪卡复习过」，首次开启闪卡时全部立即到期，符合业务语义。
func migrateWordsSRSColumns() {
	if !columnExists("words", "due_at") {
		mustExec(`ALTER TABLE words ADD COLUMN due_at DATETIME NULL`)
	}
	if !columnExists("words", "interval_days") {
		mustExec(`ALTER TABLE words ADD COLUMN interval_days INT NOT NULL DEFAULT 0`)
	}
	if !columnExists("words", "ease_factor") {
		mustExec(`ALTER TABLE words ADD COLUMN ease_factor DECIMAL(4,2) NOT NULL DEFAULT 2.50`)
	}
}

// migrateWordsImportantGlossesColumn 给 words 表补 important_glosses 列，存用户标记的「重要释义」义项文本数组。
// 与 senses 分列存放：senses 是 LLM 生成的释义、会被后台查词整段覆盖，而重要标记是用户手标、
// 独立读写，分列才能保证重查/补全不会冲掉标记。
func migrateWordsImportantGlossesColumn() {
	if columnExists("words", "important_glosses") {
		return
	}
	mustExec(`ALTER TABLE words ADD COLUMN important_glosses JSON NULL`)
}

// mergeHistoricalWordSenses 一次性把 words 表里历史遗留的、同词性被拆成多行的释义合并成一行；
// mergeSensesByPos 是幂等的，重复合并已经合并过的数据不会再变化，可以放心每次启动都跑一遍全表扫描
func mergeHistoricalWordSenses() {
	rows, err := db.Query(`SELECT id, senses FROM words WHERE senses IS NOT NULL AND JSON_LENGTH(senses) > 0`)
	if err != nil {
		log.Fatalf("扫描历史释义失败: %v", err)
	}
	type pendingUpdate struct {
		id     int
		senses []byte
	}
	var updates []pendingUpdate
	for rows.Next() {
		var id int
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			log.Fatalf("读取历史释义失败: %v", err)
		}
		var senses []Sense
		if err := json.Unmarshal(raw, &senses); err != nil {
			continue
		}
		merged := mergeSensesByPos(senses)
		if len(merged) == len(senses) {
			continue
		}
		mergedJSON, err := json.Marshal(merged)
		if err != nil {
			continue
		}
		updates = append(updates, pendingUpdate{id: id, senses: mergedJSON})
	}
	rows.Close()

	for _, u := range updates {
		if _, err := db.Exec(`UPDATE words SET senses = ? WHERE id = ?`, u.senses, u.id); err != nil {
			log.Printf("写回合并后的释义失败 id=%d: %v", u.id, err)
		}
	}
}

// backfillWordDictionary 老部署升级后的一次性回填：按 word_key 聚合现有 words 表数据，
// INSERT IGNORE 进 word_dictionary，已存在的 key 不会被覆盖，所以每次启动重跑也没有副作用
func backfillWordDictionary() {
	rows, err := db.Query(`SELECT word_key, display_word, senses, review_count, first_added_at, last_reviewed_at FROM words ORDER BY id`)
	if err != nil {
		log.Fatalf("扫描历史单词失败: %v", err)
	}
	type dictAgg struct {
		displayWord   string
		senses        []Sense
		occurrence    int
		firstSeenAt   time.Time
		lastUpdatedAt time.Time
	}
	aggs := make(map[string]*dictAgg)
	for rows.Next() {
		var wordKey, displayWord string
		var sensesRaw []byte
		var reviewCount int
		var firstAddedAt, lastReviewedAt time.Time
		if err := rows.Scan(&wordKey, &displayWord, &sensesRaw, &reviewCount, &firstAddedAt, &lastReviewedAt); err != nil {
			log.Fatalf("读取历史单词失败: %v", err)
		}
		a, ok := aggs[wordKey]
		if !ok {
			a = &dictAgg{displayWord: displayWord, firstSeenAt: firstAddedAt, lastUpdatedAt: lastReviewedAt}
			aggs[wordKey] = a
		}
		a.occurrence += reviewCount
		if firstAddedAt.Before(a.firstSeenAt) {
			a.firstSeenAt = firstAddedAt
		}
		if lastReviewedAt.After(a.lastUpdatedAt) {
			a.lastUpdatedAt = lastReviewedAt
		}
		if len(a.senses) == 0 && len(sensesRaw) > 0 {
			var senses []Sense
			if err := json.Unmarshal(sensesRaw, &senses); err == nil && len(senses) > 0 {
				a.senses = mergeSensesByPos(senses)
			}
		}
	}
	rows.Close()

	for wordKey, a := range aggs {
		sensesJSON, err := json.Marshal(a.senses)
		if err != nil {
			sensesJSON = []byte("[]")
		}
		if _, err := db.Exec(
			`INSERT IGNORE INTO word_dictionary (word_key, display_word, senses, occurrence_count, first_seen_at, last_updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			wordKey, a.displayWord, sensesJSON, a.occurrence, a.firstSeenAt, a.lastUpdatedAt,
		); err != nil {
			log.Printf("回填词库失败 word=%s: %v", wordKey, err)
		}
	}
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

// migrateTimestampsToCST 一次性把所有 DATETIME 列从 UTC 转换到 CST（+8 小时），
// 解决容器此前一直在 UTC 时区运行导致的时间戳偏移问题。用 settings 表的
// timestamps_migrated_to_cst 作为哨兵，确保只执行一次。
func migrateTimestampsToCST() {
	var done int
	err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = 'timestamps_migrated_to_cst' AND value = '1'`).Scan(&done)
	if err != nil {
		log.Fatalf("检查时间戳迁移哨兵失败: %v", err)
	}
	if done > 0 {
		return
	}

	log.Println("开始迁移历史时间戳 UTC → CST (+8h)...")

	// 所有含 DATETIME 列的表，统一加 8 小时
	mustExec(`UPDATE words SET first_added_at = DATE_ADD(first_added_at, INTERVAL 8 HOUR), last_reviewed_at = DATE_ADD(last_reviewed_at, INTERVAL 8 HOUR)`)
	mustExec(`UPDATE word_dictionary SET first_seen_at = DATE_ADD(first_seen_at, INTERVAL 8 HOUR), last_updated_at = DATE_ADD(last_updated_at, INTERVAL 8 HOUR)`)
	mustExec(`UPDATE users SET created_at = DATE_ADD(created_at, INTERVAL 8 HOUR)`)
	mustExec(`UPDATE users SET last_login_at = DATE_ADD(last_login_at, INTERVAL 8 HOUR) WHERE last_login_at IS NOT NULL`)
	mustExec(`UPDATE sessions SET created_at = DATE_ADD(created_at, INTERVAL 8 HOUR), expires_at = DATE_ADD(expires_at, INTERVAL 8 HOUR)`)

	// 写入哨兵，防止重复执行
	mustExec(`INSERT INTO settings (name, value) VALUES ('timestamps_migrated_to_cst', '1')`)

	log.Println("时间戳迁移完成")
}

func mustExec(query string) {
	if _, err := db.Exec(query); err != nil {
		log.Fatalf("数据库迁移失败: %v\nSQL: %s", err, query)
	}
}

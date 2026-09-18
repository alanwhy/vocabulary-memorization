package storage

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"vocab-backend/internal/model"
)

// Migrate 依次执行兼容老部署所需的幂等迁移。调用顺序保持固定，因为后续步骤会依赖
// 前面补齐的表或列；新安装中 schema.sql 已包含完整结构，这些操作会自然退化为空操作。
func Migrate(db *sql.DB) {
	mustExec(db, `CREATE TABLE IF NOT EXISTS users (
		id INT AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(64) NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		is_admin TINYINT(1) NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		UNIQUE KEY uniq_username (username)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	mustExec(db, `CREATE TABLE IF NOT EXISTS sessions (
		token VARCHAR(64) PRIMARY KEY,
		user_id INT NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	mustExec(db, `CREATE TABLE IF NOT EXISTS settings (
		name VARCHAR(64) PRIMARY KEY,
		value TEXT
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	mustExec(db, `CREATE TABLE IF NOT EXISTS word_dictionary (
		word_key VARCHAR(255) NOT NULL PRIMARY KEY,
		display_word VARCHAR(255) NOT NULL,
		senses JSON,
		occurrence_count INT NOT NULL DEFAULT 1,
		first_seen_at DATETIME NOT NULL,
		last_updated_at DATETIME NOT NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)

	migrateWordsSenses(db)
	migrateWordsUserIDColumn(db)
	migrateWordsTranslatingColumn(db)
	migrateWordsTranslationStartedColumn(db)
	migrateWordsArchivedColumn(db)
	migrateWordsUserArchivedIndex(db)
	migrateWordsSRSColumns(db)
	migrateWordsImportantGlossesColumn(db)
	migrateUsersLastLoginColumn(db)
	migrateUsersDisabledColumn(db)
	mergeHistoricalWordSenses(db)
	backfillWordDictionary(db)
	migrateTimestampsToCST(db)
}

// migrateUsersLastLoginColumn 给 users 表补 last_login_at 列，记录最后一次成功登录的时间；
// 历史用户在下次登录前保持 NULL，前端据此显示“从未登录”。
func migrateUsersLastLoginColumn(db *sql.DB) {
	if columnExists(db, "users", "last_login_at") {
		return
	}
	mustExec(db, `ALTER TABLE users ADD COLUMN last_login_at DATETIME NULL`)
}

// migrateUsersDisabledColumn 给 users 表补 disabled 列，标记账号是否被管理员禁用。
// 历史用户默认 0（正常），禁用后登录被拒、已有会话立即失效。
func migrateUsersDisabledColumn(db *sql.DB) {
	if columnExists(db, "users", "disabled") {
		return
	}
	mustExec(db, `ALTER TABLE users ADD COLUMN disabled TINYINT(1) NOT NULL DEFAULT 0`)
}

// migrateWordsUserArchivedIndex 给 words 表补 (user_id, archived) 索引，
// 分页列表和统计聚合都按这两列过滤，没有索引时会退化成全表扫描。
func migrateWordsUserArchivedIndex(db *sql.DB) {
	if indexExists(db, "words", "idx_words_user_archived") {
		return
	}
	mustExec(db, `ALTER TABLE words ADD KEY idx_words_user_archived (user_id, archived)`)
}

// migrateWordsSenses 给 words 表补 senses 列，并把老的 pos/translation 单值列打包迁移过去。
func migrateWordsSenses(db *sql.DB) {
	if columnExists(db, "words", "senses") {
		return
	}
	mustExec(db, `ALTER TABLE words ADD COLUMN senses JSON NULL`)

	if columnExists(db, "words", "pos") && columnExists(db, "words", "translation") {
		mustExec(db, `UPDATE words SET senses = JSON_ARRAY(JSON_OBJECT('pos', IFNULL(pos, ''), 'translation', IFNULL(translation, ''))) WHERE senses IS NULL`)
	}
}

// migrateWordsUserIDColumn 给 words 表补 user_id 列；历史数据稍后由 FinalizeWordsUserID
// 统一归到超管账号下，避免管理员尚未创建时提前写入无效归属。
func migrateWordsUserIDColumn(db *sql.DB) {
	if columnExists(db, "words", "user_id") {
		return
	}
	mustExec(db, `ALTER TABLE words ADD COLUMN user_id INT NULL AFTER id`)
}

// migrateWordsTranslatingColumn 给 words 表补 translating 列，标记查词是否还在后台异步进行中。
func migrateWordsTranslatingColumn(db *sql.DB) {
	if columnExists(db, "words", "translating") {
		return
	}
	mustExec(db, `ALTER TABLE words ADD COLUMN translating TINYINT(1) NOT NULL DEFAULT 0`)
}

// migrateWordsTranslationStartedColumn 记录最近一轮查词任务的开始时间，供后台识别卡死任务。
func migrateWordsTranslationStartedColumn(db *sql.DB) {
	if columnExists(db, "words", "translation_started_at") {
		return
	}
	mustExec(db, `ALTER TABLE words ADD COLUMN translation_started_at DATETIME NULL`)
}

// migrateWordsArchivedColumn 给 words 表补 archived 列，标记单词是否已被用户归档。
func migrateWordsArchivedColumn(db *sql.DB) {
	if columnExists(db, "words", "archived") {
		return
	}
	mustExec(db, `ALTER TABLE words ADD COLUMN archived TINYINT(1) NOT NULL DEFAULT 0`)
}

// migrateWordsSRSColumns 补齐间隔重复所需字段。老词的 due_at 保持 NULL，升级后仍应立即到期。
func migrateWordsSRSColumns(db *sql.DB) {
	if !columnExists(db, "words", "due_at") {
		mustExec(db, `ALTER TABLE words ADD COLUMN due_at DATETIME NULL`)
	}
	if !columnExists(db, "words", "interval_days") {
		mustExec(db, `ALTER TABLE words ADD COLUMN interval_days INT NOT NULL DEFAULT 0`)
	}
	if !columnExists(db, "words", "ease_factor") {
		mustExec(db, `ALTER TABLE words ADD COLUMN ease_factor DECIMAL(4,2) NOT NULL DEFAULT 2.50`)
	}
}

// migrateWordsImportantGlossesColumn 将用户手工标记与 LLM 生成的 senses 分列保存，
// 防止后台重新查词时覆盖用户标记。
func migrateWordsImportantGlossesColumn(db *sql.DB) {
	if columnExists(db, "words", "important_glosses") {
		return
	}
	mustExec(db, `ALTER TABLE words ADD COLUMN important_glosses JSON NULL`)
}

// mergeHistoricalWordSenses 合并历史数据中同词性多行释义。合并规则幂等，因此每次启动扫描安全。
func mergeHistoricalWordSenses(db *sql.DB) {
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
		var senses []model.Sense
		if err := json.Unmarshal(raw, &senses); err != nil {
			continue
		}
		merged := model.MergeSensesByPos(senses)
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

	for _, update := range updates {
		if _, err := db.Exec(`UPDATE words SET senses = ? WHERE id = ?`, update.senses, update.id); err != nil {
			log.Printf("写回合并后的释义失败 id=%d: %v", update.id, err)
		}
	}
}

// backfillWordDictionary 从现有 words 聚合全局词库；INSERT IGNORE 保证已有缓存不会被覆盖。
func backfillWordDictionary(db *sql.DB) {
	rows, err := db.Query(`SELECT word_key, display_word, senses, review_count, first_added_at, last_reviewed_at FROM words ORDER BY id`)
	if err != nil {
		log.Fatalf("扫描历史单词失败: %v", err)
	}
	type dictAgg struct {
		displayWord   string
		senses        []model.Sense
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
		agg, ok := aggs[wordKey]
		if !ok {
			agg = &dictAgg{displayWord: displayWord, firstSeenAt: firstAddedAt, lastUpdatedAt: lastReviewedAt}
			aggs[wordKey] = agg
		}
		agg.occurrence += reviewCount
		if firstAddedAt.Before(agg.firstSeenAt) {
			agg.firstSeenAt = firstAddedAt
		}
		if lastReviewedAt.After(agg.lastUpdatedAt) {
			agg.lastUpdatedAt = lastReviewedAt
		}
		if len(agg.senses) == 0 && len(sensesRaw) > 0 {
			var senses []model.Sense
			if err := json.Unmarshal(sensesRaw, &senses); err == nil && len(senses) > 0 {
				agg.senses = model.MergeSensesByPos(senses)
			}
		}
	}
	rows.Close()

	for wordKey, agg := range aggs {
		sensesJSON, err := json.Marshal(agg.senses)
		if err != nil {
			sensesJSON = []byte("[]")
		}
		if _, err := db.Exec(
			`INSERT IGNORE INTO word_dictionary (word_key, display_word, senses, occurrence_count, first_seen_at, last_updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			wordKey, agg.displayWord, sensesJSON, agg.occurrence, agg.firstSeenAt, agg.lastUpdatedAt,
		); err != nil {
			log.Printf("回填词库失败 word=%s: %v", wordKey, err)
		}
	}
}

// FinalizeWordsUserID 在超管创建后迁移无归属历史单词，再收紧列约束并修正唯一键。
func FinalizeWordsUserID(db *sql.DB, adminID int) {
	if _, err := db.Exec(`UPDATE words SET user_id = ? WHERE user_id IS NULL`, adminID); err != nil {
		log.Fatalf("迁移历史单词归属失败: %v", err)
	}
	mustExec(db, `ALTER TABLE words MODIFY COLUMN user_id INT NOT NULL`)

	if indexExists(db, "words", "uniq_word_key") {
		mustExec(db, `ALTER TABLE words DROP INDEX uniq_word_key`)
	}
	if !indexExists(db, "words", "uniq_user_word") {
		mustExec(db, `ALTER TABLE words ADD UNIQUE KEY uniq_user_word (user_id, word_key)`)
	}
}

func columnExists(db *sql.DB, table, column string) bool {
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

func indexExists(db *sql.DB, table, indexName string) bool {
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

// migrateTimestampsToCST 用持久化哨兵保证历史 UTC 时间只转换一次。
func migrateTimestampsToCST(db *sql.DB) {
	var done int
	err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = 'timestamps_migrated_to_cst' AND value = '1'`).Scan(&done)
	if err != nil {
		log.Fatalf("检查时间戳迁移哨兵失败: %v", err)
	}
	if done > 0 {
		return
	}

	log.Println("开始迁移历史时间戳 UTC → CST (+8h)...")

	mustExec(db, `UPDATE words SET first_added_at = DATE_ADD(first_added_at, INTERVAL 8 HOUR), last_reviewed_at = DATE_ADD(last_reviewed_at, INTERVAL 8 HOUR)`)
	mustExec(db, `UPDATE word_dictionary SET first_seen_at = DATE_ADD(first_seen_at, INTERVAL 8 HOUR), last_updated_at = DATE_ADD(last_updated_at, INTERVAL 8 HOUR)`)
	mustExec(db, `UPDATE users SET created_at = DATE_ADD(created_at, INTERVAL 8 HOUR)`)
	mustExec(db, `UPDATE users SET last_login_at = DATE_ADD(last_login_at, INTERVAL 8 HOUR) WHERE last_login_at IS NOT NULL`)
	mustExec(db, `UPDATE sessions SET created_at = DATE_ADD(created_at, INTERVAL 8 HOUR), expires_at = DATE_ADD(expires_at, INTERVAL 8 HOUR)`)

	mustExec(db, `INSERT INTO settings (name, value) VALUES ('timestamps_migrated_to_cst', '1')`)

	log.Println("时间戳迁移完成")
}

func mustExec(db *sql.DB, query string) {
	if _, err := db.Exec(query); err != nil {
		log.Fatalf("数据库迁移失败: %v\nSQL: %s", err, query)
	}
}

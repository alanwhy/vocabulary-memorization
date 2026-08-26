package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// escapeLikePattern 转义 LIKE 模式里的元字符，让用户输入的 % 和 _ 只当普通字符匹配。
// MySQL 的 LIKE 默认转义符是反斜杠，所以反斜杠自身也要先转义（顺序不能反）。
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// likeContains 把关键字包成「包含匹配」的 LIKE 模式；关键字为空时退化为匹配全部
func likeContains(keyword string) string {
	if keyword == "" {
		return "%"
	}
	return "%" + escapeLikePattern(keyword) + "%"
}

// nullTimePtr 把可为 NULL 的时间列转成 *time.Time，NULL 时返回 nil（JSON 里序列化为 null）
func nullTimePtr(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	t := nt.Time
	return &t
}

// cloudMeaning 从 senses JSON 里提取词云 tooltip 要展示的中文释义：
// 复用 mergeSensesByPos 合并同词性，再拼出所有 translation 用「；」连接；无释义返回空串。
func cloudMeaning(sensesRaw []byte) string {
	if len(sensesRaw) == 0 {
		return ""
	}
	var senses []Sense
	if err := json.Unmarshal(sensesRaw, &senses); err != nil {
		return ""
	}
	merged := mergeSensesByPos(senses)
	translations := make([]string, 0, len(merged))
	for _, s := range merged {
		translations = append(translations, s.Translation)
	}
	return strings.Join(translations, "；")
}

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
	var lastLogin sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, is_admin, created_at, last_login_at FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &hash, &u.IsAdmin, &u.CreatedAt, &lastLogin)
	u.LastLoginAt = nullTimePtr(lastLogin)
	return u, hash, err
}

// RecordLogin 记录一次成功登录的时间
func (r *userRepo) RecordLogin(ctx context.Context, id int, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, now, id)
	return err
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

// List 管理员用户列表：LEFT JOIN 一次性把每个用户录入的单词数（含已归档）聚合出来，
// 避免在 handler 里对每个用户各发一条 COUNT 查询
func (r *userRepo) List(ctx context.Context) ([]UserWithStats, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT u.id, u.username, u.is_admin, u.created_at, u.last_login_at, COUNT(w.id)
		 FROM users u LEFT JOIN words w ON w.user_id = u.id
		 GROUP BY u.id, u.username, u.is_admin, u.created_at, u.last_login_at
		 ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []UserWithStats{}
	for rows.Next() {
		var u UserWithStats
		var lastLogin sql.NullTime
		if err := rows.Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &lastLogin, &u.WordCount); err != nil {
			return nil, err
		}
		u.LastLoginAt = nullTimePtr(lastLogin)
		list = append(list, u)
	}
	return list, rows.Err()
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
	var lastLogin sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.is_admin, u.created_at, u.last_login_at, s.expires_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ?`,
		token,
	).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &lastLogin, &expiresAt)
	u.LastLoginAt = nullTimePtr(lastLogin)
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

// wordColumns words 表的完整列清单，配合 scanWordRows 使用，保证列顺序和扫描顺序一致
const wordColumns = `id, word_key, display_word, senses, translating, archived, review_count, first_added_at, last_reviewed_at, due_at, interval_days, ease_factor`

// scanWordRows 按 wordColumns 的列顺序把结果集扫成 []Word 并解开 senses JSON
func scanWordRows(rows *sql.Rows) ([]Word, error) {
	defer rows.Close()

	list := []Word{}
	for rows.Next() {
		var wd Word
		var sensesRaw []byte
		var dueAt sql.NullTime
		if err := rows.Scan(&wd.ID, &wd.WordKey, &wd.DisplayWord, &sensesRaw, &wd.Translating, &wd.Archived, &wd.ReviewCount, &wd.FirstAddedAt, &wd.LastReviewedAt, &dueAt, &wd.IntervalDays, &wd.EaseFactor); err != nil {
			return nil, err
		}
		wd.DueAt = nullTimePtr(dueAt)
		if len(sensesRaw) > 0 {
			if err := json.Unmarshal(sensesRaw, &wd.Senses); err != nil {
				return nil, err
			}
		}
		list = append(list, wd)
	}
	return list, rows.Err()
}

// wordOrderBy 把前端传来的排序模式映射成固定的 ORDER BY 片段。
// 这里必须走白名单：SQL 片段是拼接进语句的，绝不能让请求参数直接落进去。
// 每种排序都以 id 收尾——review_count / last_reviewed_at / word_key 都不唯一，
// 缺少唯一 tiebreaker 时 LIMIT/OFFSET 翻页会出现跨页重复或漏行。
func wordOrderBy(sort string) string {
	switch sort {
	case "time":
		return `last_reviewed_at DESC, id DESC`
	case "time_asc":
		return `last_reviewed_at ASC, id ASC`
	case "alpha":
		return `word_key ASC, id ASC`
	case "alpha_desc":
		return `word_key DESC, id DESC`
	case "count_asc":
		return `review_count ASC, last_reviewed_at ASC, id ASC`
	default: // count：默认按背诵次数倒序
		return `review_count DESC, last_reviewed_at DESC, id DESC`
	}
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

// ApplyFlashcardReview 记录一次闪卡自评：累加背诵次数、刷新最近复习时间，并写入 SRS 排期结果
// （下次到期时间、新间隔、新难度系数）与归档状态。「记住」时归档（不再复习），模糊/不认识保持未归档。
// WHERE 同时限定 id 与 user_id，保证只更新当前用户自己的词。
func (r *wordRepo) ApplyFlashcardReview(ctx context.Context, id, userID, newCount, intervalDays int, easeFactor float64, dueAt, now time.Time, archived bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE words SET review_count = ?, last_reviewed_at = ?, interval_days = ?, ease_factor = ?, due_at = ?, archived = ? WHERE id = ? AND user_id = ?`,
		newCount, now, intervalDays, easeFactor, dueAt, archived, id, userID,
	)
	return err
}

// DueFlashcards 返回当前用户到期的闪卡队列：从未复习过（due_at 为 NULL）的词 + 已到期（due_at <= now）的词。
// 只含未归档词。排序遵循「新词优先 → 到期时间先后 → 同一天到期按背诵次数最多优先 → id 收尾」，
// id 收尾保证顺序确定。limit 控制每组取多少张，背完一组后这些词被排期/归档，下次再取自然轮到下一批。
func (r *wordRepo) DueFlashcards(ctx context.Context, userID, limit int, now time.Time) ([]Word, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+wordColumns+` FROM words
		 WHERE user_id = ? AND archived = 0 AND (due_at IS NULL OR due_at <= ?)
		 ORDER BY (due_at IS NULL) DESC, due_at ASC, review_count DESC, id ASC
		 LIMIT ?`,
		userID, now, limit,
	)
	if err != nil {
		return nil, err
	}
	return scanWordRows(rows)
}

// ListPage 按 sort 指定的顺序取一页单词；sort 只能是 wordOrderBy 认识的白名单值。
// keyword / status 通过 senseFilterWhere 下沉到数据库做模糊匹配与释义状态过滤。
func (r *wordRepo) ListPage(ctx context.Context, userID int, archived bool, keyword, status, sort string, limit, offset int) ([]Word, error) {
	conds, args := senseFilterWhere(keyword, status)
	if conds != "" {
		conds = " AND " + conds
	}
	args = append([]interface{}{userID, archived}, args...)
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+wordColumns+` FROM words WHERE user_id = ? AND archived = ?`+conds+
			` ORDER BY `+wordOrderBy(sort)+` LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	return scanWordRows(rows)
}

func (r *wordRepo) CountByUser(ctx context.Context, userID int, archived bool, keyword, status string) (int, error) {
	conds, args := senseFilterWhere(keyword, status)
	if conds != "" {
		conds = " AND " + conds
	}
	args = append([]interface{}{userID, archived}, args...)
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM words WHERE user_id = ? AND archived = ?`+conds, args...).Scan(&count)
	return count, err
}

// ResetReviewCounts 把某用户所有单词的背诵次数重置为 1；只动 review_count，不碰 SRS 排期字段。
func (r *wordRepo) ResetReviewCounts(ctx context.Context, userID int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE words SET review_count = 1 WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *wordRepo) Delete(ctx context.Context, id, userID int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM words WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteByWordKey 删除所有用户里 word_key 匹配的记录。管理侧删除词库缓存时一并调用，
// 让用户已保存的同名单词也从各自单词表里移除。
func (r *wordRepo) DeleteByWordKey(ctx context.Context, wordKey string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM words WHERE word_key = ?`, wordKey)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteByWordKeys 批量删除所有用户里 word_key 命中任一给定值的记录；wordKeys 为空时直接返回。
// 占位符按数量生成，wordKey 本身仍走参数绑定，不拼接任何用户输入。
func (r *wordRepo) DeleteByWordKeys(ctx context.Context, wordKeys []string) (int64, error) {
	if len(wordKeys) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(wordKeys)), ",")
	args := make([]interface{}, len(wordKeys))
	for i, k := range wordKeys {
		args[i] = k
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM words WHERE word_key IN (`+placeholders+`)`, args...)
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

// MarkTranslationStarted 记录一轮查词任务的开始时间，供周期性扫描判断任务是否卡死。
// 每次重新触发查词都会刷新这个时间戳，避免把正在合法重试中的任务误判成卡死。
func (r *wordRepo) MarkTranslationStarted(ctx context.Context, id int, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE words SET translation_started_at = ? WHERE id = ?`, now, id)
	return err
}

// MarkTranslating 把单词标记为「查词中」并刷新查词开始时间，用于再次录入发现未强化时触发补全：
// translating=1 让前端显示「查词中」并轮询，同时充当防重标志，避免快速连点重复触发查词。
func (r *wordRepo) MarkTranslating(ctx context.Context, id int, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE words SET translating = 1, translation_started_at = ? WHERE id = ?`, now, id)
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
	return list, rows.Err()
}

// FindTranslatingStale 返回 translating=1 且查词开始时间早于阈值（或从未标记开始时间）的记录，
// 用于周期性扫描那些 goroutine 在写回结果前意外退出、永久卡死在“查询中”的任务。
// 正在合法重试中的任务开始时间刚被刷新过，不会落入这里。
func (r *wordRepo) FindTranslatingStale(ctx context.Context, threshold time.Time) ([]Word, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, word_key FROM words WHERE translating = 1 AND (translation_started_at IS NULL OR translation_started_at < ?)`,
		threshold,
	)
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
	return list, rows.Err()
}

// FindTranslatingByUser 返回当前用户还在查词中的完整记录。前端轮询只拉这几条，
// 按 id 就地替换已加载的列表项，不用整表重载（分页后重载会把滚动位置打回顶部）。
func (r *wordRepo) FindTranslatingByUser(ctx context.Context, userID int) ([]Word, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+wordColumns+` FROM words WHERE user_id = ? AND translating = 1`, userID)
	if err != nil {
		return nil, err
	}
	return scanWordRows(rows)
}

// FindByIDs 按 id 批量取当前用户的单词。前端轮询查词结果时要拿到这几条的**当前**状态
// （包含已经查完、translating 已置 0 的），只查 translating = 1 的话释义永远补不回列表。
func (r *wordRepo) FindByIDs(ctx context.Context, userID int, ids []int) ([]Word, error) {
	if len(ids) == 0 {
		return []Word{}, nil
	}
	// 占位符个数按 id 数量生成，id 本身仍然全部走参数绑定，语句里不拼接任何用户输入
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+wordColumns+` FROM words WHERE user_id = ? AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	return scanWordRows(rows)
}

// Stats 统计页需要的聚合数值，用 3 条聚合 SQL 算出，不再把全量单词拉到前端。
// SUM 在 0 行时返回 NULL，所以统一套 COALESCE 兜成 0。since 为本地时区的起始时间。
// todaySince / todayUntil 划定今日背诵次数的统计窗口：[00:00:00, 23:59:59.999...)，
// 对窗口内被复习过的每个单词计数 1（不累计历史 review_count），避免把历史背诵次数也加进来；
// 闪卡「记住」会直接归档，所以这里不筛 archived，今天背过的词（含已归档）都算进今日背诵。
func (r *wordRepo) Stats(ctx context.Context, userID int, since, since7, todaySince, todayUntil time.Time) (WordStats, error) {
	var s WordStats

	err := r.db.QueryRowContext(ctx,
		`SELECT
		   COALESCE(SUM(archived = 0), 0),
		   COALESCE(SUM(archived = 1), 0),
		   COUNT(*),
		   COALESCE(SUM(CASE WHEN archived = 0 THEN review_count ELSE 0 END), 0),
		   COALESCE(SUM(archived = 0 AND translating = 1), 0),
		   COALESCE(SUM(CASE WHEN last_reviewed_at >= ? AND last_reviewed_at < ? THEN 1 ELSE 0 END), 0)
		 FROM words WHERE user_id = ?`,
		todaySince, todayUntil, userID,
	).Scan(&s.TotalWords, &s.ArchivedWords, &s.TotalAllWords, &s.TotalReviews, &s.TranslatingCount, &s.TodayReviews)
	if err != nil {
		return WordStats{}, err
	}

	// 分档口径与前端 reviewBuckets 的 5 个标签一一对应：1 次 / 2-3 次 / 4-6 次 / 7-10 次 / 10 次以上
	buckets := make([]int, 5)
	err = r.db.QueryRowContext(ctx,
		`SELECT
		   COALESCE(SUM(review_count = 1), 0),
		   COALESCE(SUM(review_count BETWEEN 2 AND 3), 0),
		   COALESCE(SUM(review_count BETWEEN 4 AND 6), 0),
		   COALESCE(SUM(review_count BETWEEN 7 AND 10), 0),
		   COALESCE(SUM(review_count >= 11), 0)
		 FROM words WHERE user_id = ? AND archived = 0`,
		userID,
	).Scan(&buckets[0], &buckets[1], &buckets[2], &buckets[3], &buckets[4])
	if err != nil {
		return WordStats{}, err
	}
	s.ReviewBuckets = buckets

	rows, err := r.db.QueryContext(ctx,
		`SELECT DATE_FORMAT(first_added_at, '%Y-%m-%d') d, COUNT(*)
		 FROM words WHERE user_id = ? AND archived = 0 AND first_added_at >= ?
		 GROUP BY d ORDER BY d`,
		userID, since,
	)
	if err != nil {
		return WordStats{}, err
	}
	defer rows.Close()

	s.DailyAdditions = []dailyCount{}
	for rows.Next() {
		var d dailyCount
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			return WordStats{}, err
		}
		s.DailyAdditions = append(s.DailyAdditions, d)
	}
	if err := rows.Err(); err != nil {
		return WordStats{}, err
	}

	// 词云：近 7 天复习过的单词，权重用累计背诵次数，按次数降序取前 50 个避免词云过密；
	// 顺带取出 senses 解析成中文释义，供前端 tooltip 展示
	cloudRows, err := r.db.QueryContext(ctx,
		`SELECT display_word, review_count, senses FROM words
		 WHERE user_id = ? AND archived = 0 AND last_reviewed_at >= ?
		 ORDER BY review_count DESC, display_word ASC LIMIT 50`,
		userID, since7,
	)
	if err != nil {
		return WordStats{}, err
	}
	defer cloudRows.Close()

	s.WordCloud = []wordCloudItem{}
	for cloudRows.Next() {
		var it wordCloudItem
		var sensesRaw []byte
		if err := cloudRows.Scan(&it.Word, &it.Count, &sensesRaw); err != nil {
			return WordStats{}, err
		}
		it.Meaning = cloudMeaning(sensesRaw)
		s.WordCloud = append(s.WordCloud, it)
	}
	if err := cloudRows.Err(); err != nil {
		return WordStats{}, err
	}

	// 开头字母统计：按展示拼写的首字母（大写归一）分组计数
	letterRows, err := r.db.QueryContext(ctx,
		`SELECT UPPER(LEFT(display_word, 1)) l, COUNT(*) c FROM words
		 WHERE user_id = ? AND archived = 0
		 GROUP BY l ORDER BY l`,
		userID,
	)
	if err != nil {
		return WordStats{}, err
	}
	defer letterRows.Close()

	s.LetterStats = []letterStat{}
	for letterRows.Next() {
		var ls letterStat
		if err := letterRows.Scan(&ls.Letter, &ls.Count); err != nil {
			return WordStats{}, err
		}
		s.LetterStats = append(s.LetterStats, ls)
	}
	if err := letterRows.Err(); err != nil {
		return WordStats{}, err
	}
	return s, nil
}

// dictionaryRepo 封装 word_dictionary 表的数据访问
type dictionaryRepo struct{ db *sql.DB }

// vocabularyItem 词库索引里的一项：word_key 到全局出现次数的映射，
// 供前端渲染例句/近反义/形近词时做「是否在词库、出现多少次」的高亮判断。
type vocabularyItem struct {
	WordKey         string `json:"word_key"`
	OccurrenceCount int    `json:"occurrence_count"`
}

// VocabularyIndex 返回全局词库的全量 word_key -> occurrence_count 索引。
// 前端据此高亮例句/近反义/形近词里出现过的词，并按出现次数分级着色。
func (r *dictionaryRepo) VocabularyIndex(ctx context.Context) ([]vocabularyItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT word_key, occurrence_count FROM word_dictionary`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []vocabularyItem{}
	for rows.Next() {
		var it vocabularyItem
		if err := rows.Scan(&it.WordKey, &it.OccurrenceCount); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

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

// SaveSenses 只在「缓存里还没有强化数据」时写入，避免并发下互相覆盖。
// 条件里额外判 $[0].phonetic：存量旧数据只含 pos/translation（无 phonetic），
// 补全后需要被新数据升级覆盖，所以把「未强化」也当作可写。
func (r *dictionaryRepo) SaveSenses(ctx context.Context, wordKey string, sensesJSON []byte) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE word_dictionary SET senses = ?
		 WHERE word_key = ?
		   AND (senses IS NULL
		        OR JSON_LENGTH(senses) = 0
		        OR JSON_EXTRACT(senses, '$[0].phonetic') IS NULL
		        OR JSON_EXTRACT(senses, '$[0].phonetic') = '')`,
		sensesJSON, wordKey,
	)
	return err
}

// dictColumns word_dictionary 的列清单，配合 scanDictRows 使用
const dictColumns = `word_key, display_word, senses, occurrence_count, last_updated_at`

func scanDictRows(rows *sql.Rows) ([]dictionaryEntry, error) {
	defer rows.Close()

	entries := []dictionaryEntry{}
	for rows.Next() {
		var e dictionaryEntry
		var sensesRaw []byte
		if err := rows.Scan(&e.WordKey, &e.DisplayWord, &sensesRaw, &e.OccurrenceCount, &e.LastUpdatedAt); err != nil {
			return nil, err
		}
		if len(sensesRaw) > 0 {
			if err := json.Unmarshal(sensesRaw, &e.Senses); err != nil {
				return nil, err
			}
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// List 返回全量词库，供 CSV 导出使用（导出不分页）
func (r *dictionaryRepo) List(ctx context.Context) ([]dictionaryEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+dictColumns+` FROM word_dictionary ORDER BY last_updated_at DESC, word_key ASC`)
	if err != nil {
		return nil, err
	}
	return scanDictRows(rows)
}

// senseFilterWhere 根据关键词和释义状态拼出过滤条件（不含 WHERE 关键字）及对应参数。
// keyword、status 都为空时返回空字符串和空参数，表示不过滤。words 与 word_dictionary 两表的
// word_key / senses 列名一致，因此共用同一份过滤逻辑；调用方自行决定前置 " WHERE "（作为首个条件）
// 还是 " AND "（追加到已有 WHERE 之后）。status 取值：
// "no_definition"（暂无释义）/ "has_definition"（已有释义）/ 其它或空（不过滤）。
func senseFilterWhere(keyword, status string) (string, []interface{}) {
	conds := []string{}
	args := []interface{}{}
	if keyword != "" {
		// JSON_SEARCH 第二个参数 'one' 表示匹配到一条即可；senses 数组里的对象形如
		// {"pos":"n.","translation":"名词"}，'$.translation' 指只对 translation 字段做匹配。
		like := likeContains(keyword)
		conds = append(conds, `(word_key LIKE ? OR JSON_SEARCH(senses, 'one', ?, NULL, '$.translation') IS NOT NULL)`)
		args = append(args, like, like)
	}
	switch status {
	case "no_definition":
		conds = append(conds, `(senses IS NULL OR JSON_LENGTH(senses) = 0)`)
	case "has_definition":
		conds = append(conds, `(senses IS NOT NULL AND JSON_LENGTH(senses) > 0)`)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return strings.Join(conds, " AND "), args
}

// ListPage 取一页词库记录。keyword 非空时同时匹配单词和释义（释义通过 JSON_SEARCH
// 在 senses JSON 数组里查找），status 按释义有无过滤；两者都下沉到数据库，前端不再本地过滤。
func (r *dictionaryRepo) ListPage(ctx context.Context, keyword, status string, limit, offset int) ([]dictionaryEntry, error) {
	conds, args := senseFilterWhere(keyword, status)
	where := ""
	if conds != "" {
		where = " WHERE " + conds
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+dictColumns+` FROM word_dictionary`+where+
			` ORDER BY last_updated_at DESC, word_key ASC LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	return scanDictRows(rows)
}

func (r *dictionaryRepo) Count(ctx context.Context, keyword, status string) (int, error) {
	conds, args := senseFilterWhere(keyword, status)
	where := ""
	if conds != "" {
		where = " WHERE " + conds
	}
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM word_dictionary`+where, args...).Scan(&count)
	return count, err
}

func (r *dictionaryRepo) Delete(ctx context.Context, wordKey string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM word_dictionary WHERE word_key = ?`, wordKey)
	return err
}

// DeleteMany 批量删除词库缓存；wordKeys 为空时直接返回。占位符按数量生成，
// 但 wordKey 本身仍然走参数绑定，语句里不拼接任何用户输入。
// 上限取 maxPageLimit，避免一次提交删除过多记录。
func (r *dictionaryRepo) DeleteMany(ctx context.Context, wordKeys []string) (int64, error) {
	if len(wordKeys) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(wordKeys)), ",")
	args := make([]interface{}, len(wordKeys))
	for i, k := range wordKeys {
		args[i] = k
	}
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM word_dictionary WHERE word_key IN (`+placeholders+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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

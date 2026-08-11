package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

const staticDir = "./static"

// spaHandler 让 Vue Router 的 history 模式深链接（如直接访问 /profile 并刷新）不 404：
// 静态资源存在就直接返回，否则一律回退到 index.html，交给前端路由接管。
func spaHandler(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
}

// wordPattern 简单校验：以英文字母开头，只允许字母、空格、连字符、单引号，长度不超过 64
var wordPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z'\- ]{0,63}$`)

const (
	defaultRequestTimeout = 10 * time.Second
	exportRequestTimeout  = 60 * time.Second
	shutdownTimeout       = 15 * time.Second
	bgTaskGracePeriod     = 10 * time.Second
)

func main() {
	connectDB()
	defer db.Close()

	app := NewApp(db, Config{CookieSecure: getEnvBool("COOKIE_SECURE", false)})

	migrateSchema()
	adminID := app.bootstrapAdmin()
	finalizeWordsUserID(adminID)
	app.loadSettings()
	app.resumeStuckTranslations()

	go app.loginLimiter.sweep(10*time.Minute, app.bgCtx.Done())
	go app.pwLimiter.sweep(10*time.Minute, app.bgCtx.Done())

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", withTimeout(defaultRequestTimeout)(app.handleLogin))
	mux.HandleFunc("POST /api/logout", withTimeout(defaultRequestTimeout)(app.handleLogout))
	mux.HandleFunc("GET /api/me", withTimeout(defaultRequestTimeout)(app.requireAuth(app.handleMe)))
	mux.HandleFunc("PUT /api/me/password", withTimeout(defaultRequestTimeout)(app.requireAuth(app.handleChangePassword)))

	mux.HandleFunc("POST /api/words", withTimeout(defaultRequestTimeout)(app.requireAuth(app.handleAddWord)))
	mux.HandleFunc("GET /api/words", withTimeout(defaultRequestTimeout)(app.requireAuth(app.handleListWords)))
	mux.HandleFunc("GET /api/words/translating", withTimeout(defaultRequestTimeout)(app.requireAuth(app.handleListTranslatingWords)))
	mux.HandleFunc("GET /api/stats", withTimeout(defaultRequestTimeout)(app.requireAuth(app.handleWordStats)))
	mux.HandleFunc("DELETE /api/words/{id}", withTimeout(defaultRequestTimeout)(app.requireAuth(app.handleDeleteWord)))
	mux.HandleFunc("POST /api/words/{id}/archive", withTimeout(defaultRequestTimeout)(app.requireAuth(app.handleArchiveWord)))
	mux.HandleFunc("POST /api/words/{id}/unarchive", withTimeout(defaultRequestTimeout)(app.requireAuth(app.handleUnarchiveWord)))

	mux.HandleFunc("POST /api/admin/users", withTimeout(defaultRequestTimeout)(app.requireAdmin(app.handleCreateUser)))
	mux.HandleFunc("GET /api/admin/users", withTimeout(defaultRequestTimeout)(app.requireAdmin(app.handleListUsers)))
	mux.HandleFunc("POST /api/admin/users/{id}/reset-password", withTimeout(defaultRequestTimeout)(app.requireAdmin(app.handleResetUserPassword)))
	mux.HandleFunc("GET /api/admin/settings", withTimeout(defaultRequestTimeout)(app.requireAdmin(app.handleGetSettings)))
	mux.HandleFunc("PUT /api/admin/settings", withTimeout(defaultRequestTimeout)(app.requireAdmin(app.handleUpdateSettings)))
	mux.HandleFunc("GET /api/admin/dictionary", withTimeout(defaultRequestTimeout)(app.requireAdmin(app.handleListDictionary)))
	mux.HandleFunc("GET /api/admin/dictionary/export", withTimeout(exportRequestTimeout)(app.requireAdmin(app.handleExportDictionary)))
	mux.HandleFunc("DELETE /api/admin/dictionary/{word_key}", withTimeout(defaultRequestTimeout)(app.requireAdmin(app.handleDeleteDictionaryEntry)))
	mux.HandleFunc("POST /api/admin/dictionary/batch-delete", withTimeout(defaultRequestTimeout)(app.requireAdmin(app.handleDeleteDictionaryBatch)))

	mux.HandleFunc("/", spaHandler)

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      recoverMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("服务启动，监听 %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("收到关闭信号，开始优雅关闭...")

	// 先让后台查词任务停止等待/重试，再关 HTTP 服务，减少新增在途任务
	app.bgCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("优雅关闭 HTTP 服务失败: %v", err)
	}

	waitDone := make(chan struct{})
	go func() {
		app.translateWG.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
		log.Println("后台查词任务已全部完成")
	case <-time.After(bgTaskGracePeriod):
		log.Println("等待后台查词任务超时，放弃剩余任务")
	}

	log.Println("服务已关闭")
}

type addWordRequest struct {
	Word string `json:"word"`
}

func (a *App) handleAddWord(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	var req addWordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	raw := strings.TrimSpace(req.Word)
	if raw == "" {
		writeError(w, http.StatusBadRequest, "单词不能为空")
		return
	}
	if !wordPattern.MatchString(raw) {
		writeError(w, http.StatusBadRequest, "请输入有效的英文单词（仅支持英文字母、空格、连字符、单引号）")
		return
	}
	wordKey := strings.ToLower(raw)
	now := time.Now()

	// 已存在则次数 +1 并直接返回
	if handled := a.tryIncrementExisting(w, r, user.ID, wordKey, now); handled {
		return
	}

	// 不管是不是这个用户第一次输入，先登记一次全局词库的出现次数
	a.upsertDictionaryOccurrence(r.Context(), wordKey, raw, now)

	// 全局词库如果已经缓存过这个词的释义，直接复用，不用再问一次大模型
	cachedSenses, cacheHit := a.lookupDictionarySenses(r.Context(), wordKey)
	initialSenses := []Sense{}
	if cacheHit {
		initialSenses = cachedSenses
	}
	sensesJSON, err := json.Marshal(initialSenses)
	if err != nil {
		sensesJSON = []byte("[]")
	}

	wordID, err := a.words.Insert(r.Context(), user.ID, wordKey, raw, sensesJSON, !cacheHit, now)
	if err != nil {
		// 并发下可能有另一个请求刚好抢先插入了同一个单词，退化为累加次数
		if mysqlErr, ok := err.(*mysqldriver.MySQLError); ok && mysqlErr.Number == 1062 {
			if handled := a.tryIncrementExisting(w, r, user.ID, wordKey, now); handled {
				return
			}
		}
		log.Printf("插入单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "保存失败")
		return
	}

	if !cacheHit {
		a.spawnTranslation(wordID, wordKey)
	}

	newWord := Word{
		ID:             wordID,
		WordKey:        wordKey,
		DisplayWord:    raw,
		Senses:         initialSenses,
		Translating:    !cacheHit,
		ReviewCount:    1,
		FirstAddedAt:   now,
		LastReviewedAt: now,
	}
	writeJSON(w, http.StatusCreated, newWord)
}

// translateRetryDelays 查词失败时的重试间隔（指数退避），用完仍失败才彻底放弃
var translateRetryDelays = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second}

// spawnTranslation 以受信号量限流、可被进程关闭信号取消的方式启动后台查词任务；
// 信号量获取是阻塞式的（不丢弃），避免单词永久卡在 translating=1 无法自愈。
func (a *App) spawnTranslation(wordID int, wordKey string) {
	a.translateWG.Add(1)
	go func() {
		defer a.translateWG.Done()
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("后台查词任务发生 panic word=%s: %v", wordKey, rec)
			}
		}()

		select {
		case a.translateSem <- struct{}{}:
		case <-a.bgCtx.Done():
			log.Printf("进程正在关闭，取消尚未开始的查词任务 word=%s", wordKey)
			return
		}
		defer func() { <-a.translateSem }()

		a.translateAndSave(a.bgCtx, wordID, wordKey)
	}()
}

// resumeStuckTranslations 扫描进程重启前 translating=1 却没能写回释义的记录，重新触发查词，
// 否则这些单词会永久卡在“翻译中”状态，没有其它机制能让它们自愈。
func (a *App) resumeStuckTranslations() {
	stuck, err := a.words.FindTranslating(context.Background())
	if err != nil {
		log.Printf("扫描未完成的查词任务失败: %v", err)
		return
	}
	for _, wd := range stuck {
		log.Printf("重新触发进程重启前未完成的查词任务 word=%s", wd.WordKey)
		a.spawnTranslation(wd.ID, wd.WordKey)
	}
}

// translateAndSave 在后台异步查词，查完再把释义写回数据库和全局词库缓存，不阻塞单词的录入请求；
// 查词失败会按退避间隔重试，避免偶发失败导致释义永久空白；ctx 取消时（进程关闭）提前退出。
func (a *App) translateAndSave(ctx context.Context, wordID int, wordKey string) {
	for attempt := 0; ; attempt++ {
		cfg := a.getDeepSeekConfig()
		result := translateWord(ctx, wordKey, cfg)
		merged := mergeSensesByPos(result.Senses)
		if len(merged) > 0 {
			a.saveWordSenses(ctx, wordID, wordKey, merged)
			a.saveDictionarySenses(ctx, wordKey, merged)
			return
		}
		if attempt >= len(translateRetryDelays) {
			log.Printf("查词多次重试后仍失败，放弃 word=%s", wordKey)
			a.saveWordSenses(ctx, wordID, wordKey, []Sense{})
			return
		}
		delay := translateRetryDelays[attempt]
		log.Printf("查词失败，%s 后重试 word=%s attempt=%d", delay, wordKey, attempt+1)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			log.Printf("进程正在关闭，取消等待重试的查词任务 word=%s", wordKey)
			return
		}
	}
}

func (a *App) saveWordSenses(ctx context.Context, wordID int, wordKey string, senses []Sense) {
	sensesJSON, err := json.Marshal(senses)
	if err != nil {
		log.Printf("序列化释义失败 word=%s: %v", wordKey, err)
		sensesJSON = []byte("[]")
	}
	if err := a.words.UpdateSenses(ctx, wordID, sensesJSON); err != nil {
		log.Printf("写回释义失败 word=%s id=%d: %v", wordKey, wordID, err)
	}
}

// tryIncrementExisting 如果当前用户名下 wordKey 已存在，则次数 +1、更新最近背诵时间，并写响应；
// 返回 true 表示请求已经处理完毕（无论是成功还是出错），调用方不需要再做任何事。
func (a *App) tryIncrementExisting(w http.ResponseWriter, r *http.Request, userID int, wordKey string, now time.Time) bool {
	existing, sensesRaw, err := a.words.FindByUserAndKey(r.Context(), userID, wordKey)

	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		log.Printf("查询单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return true
	}
	if len(sensesRaw) > 0 {
		if err := json.Unmarshal(sensesRaw, &existing.Senses); err != nil {
			log.Printf("解析释义失败: %v", err)
		}
	}

	newCount := existing.ReviewCount + 1
	if err := a.words.IncrementReview(r.Context(), existing.ID, newCount, now); err != nil {
		log.Printf("更新单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "更新失败")
		return true
	}
	a.upsertDictionaryOccurrence(r.Context(), existing.WordKey, existing.DisplayWord, now)
	existing.ReviewCount = newCount
	existing.LastReviewedAt = now
	writeJSON(w, http.StatusOK, existing)
	return true
}

const (
	defaultPageLimit = 100
	maxPageLimit     = 200
	statsTrendDays   = 14
)

// parsePagination 解析并夹取分页参数，非法或缺失时退回默认值，避免一次拉走整张表
func parsePagination(r *http.Request) (page, limit, offset int) {
	page = 1
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 1 {
		page = v
	}
	limit = defaultPageLimit
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
		limit = v
		if limit > maxPageLimit {
			limit = maxPageLimit
		}
	}
	return page, limit, (page - 1) * limit
}

// parseIDList 解析逗号分隔的 id 列表，跳过非法项并去重，最多取 max 个：
// 轮询的 ids 来自浏览器，不限长度的话一个超长列表会生成同样长的 IN 占位符
func parseIDList(s string, max int) []int {
	if s == "" {
		return nil
	}
	seen := make(map[int]bool)
	ids := []int{}
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || v <= 0 || seen[v] {
			continue
		}
		seen[v] = true
		ids = append(ids, v)
		if len(ids) >= max {
			break
		}
	}
	return ids
}

func (a *App) handleListWords(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	archived := r.URL.Query().Get("archived") == "1"
	sort := r.URL.Query().Get("sort")
	page, limit, offset := parsePagination(r)

	total, err := a.words.CountByUser(r.Context(), user.ID, archived)
	if err != nil {
		log.Printf("统计单词总数失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	list, err := a.words.ListPage(r.Context(), user.ID, archived, sort, limit, offset)
	if err != nil {
		log.Printf("查询列表失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	writeJSON(w, http.StatusOK, newPageResult(list, total, page, limit))
}

// handleListTranslatingWords 供前端轮询查词进度。
// 带 ids 时返回这些 id 的当前状态（哪怕已经查完）——前端就是靠这次响应把释义补回列表；
// 不带 ids 时返回该用户所有还在查词中的单词，用于刷新页面后重新接上轮询。
func (a *App) handleListTranslatingWords(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ids := parseIDList(r.URL.Query().Get("ids"), maxPageLimit)

	var (
		list []Word
		err  error
	)
	if len(ids) > 0 {
		list, err = a.words.FindByIDs(r.Context(), user.ID, ids)
	} else {
		list, err = a.words.FindTranslatingByUser(r.Context(), user.ID)
	}
	if err != nil {
		log.Printf("查询查词中的单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleWordStats 返回统计页需要的聚合数值。列表改成分页后前端拿不到全量数据，
// 这些数字必须由后端用 SQL 聚合算出。
func (a *App) handleWordStats(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	// 趋势图展示最近 statsTrendDays 天（含今天），起点取本地时区当天零点
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	since := midnight.AddDate(0, 0, -(statsTrendDays - 1))

	stats, err := a.words.Stats(r.Context(), user.ID, since, midnight)
	if err != nil {
		log.Printf("查询统计数据失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *App) handleDeleteWord(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}
	affected, err := a.words.Delete(r.Context(), id, user.ID)
	if err != nil {
		log.Printf("删除失败: %v", err)
		writeError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	if affected == 0 {
		writeError(w, http.StatusNotFound, "单词不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleArchiveWord(w http.ResponseWriter, r *http.Request) {
	a.setWordArchived(w, r, true)
}

func (a *App) handleUnarchiveWord(w http.ResponseWriter, r *http.Request) {
	a.setWordArchived(w, r, false)
}

// setWordArchived 归档/取消归档只是给单词打个标记，不涉及删除，不需要二次确认
func (a *App) setWordArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	user := currentUser(r)
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}
	affected, err := a.words.SetArchived(r.Context(), id, user.ID, archived)
	if err != nil {
		log.Printf("更新归档状态失败: %v", err)
		writeError(w, http.StatusInternalServerError, "更新失败")
		return
	}
	if affected == 0 {
		writeError(w, http.StatusNotFound, "单词不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
}

func (a *App) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "用户名不能为空")
		return
	}
	if len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "密码长度至少 6 位")
		return
	}

	user, err := a.createUser(r.Context(), username, req.Password, req.IsAdmin)
	if err != nil {
		if mysqlErr, ok := err.(*mysqldriver.MySQLError); ok && mysqlErr.Number == 1062 {
			writeError(w, http.StatusConflict, "用户名已存在")
			return
		}
		log.Printf("创建用户失败: %v", err)
		writeError(w, http.StatusInternalServerError, "创建失败")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request) {
	list, err := a.users.List(r.Context())
	if err != nil {
		log.Printf("查询用户列表失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("写入响应失败: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

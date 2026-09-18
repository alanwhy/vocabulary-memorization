// Package app 的单词业务入口：HTTP handler 读取请求与登录用户，调用注入的 Store，
// 后台翻译任务再经 DeepSeek/TTS 适配器写回结果。路由装配集中在 routes.go，
// 领域模型与纯规则由 internal/model 提供，避免应用层复制业务算法。
package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// wordPattern 简单校验：以英文字母开头，只允许字母、空格、连字符、单引号，长度不超过 64。
var wordPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z'\- ]{0,63}$`)

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

	// 全局词库如果已经缓存过这个词的释义，直接复用，不用再问一次大模型；
	// 但缓存命中的可能是升级前的旧数据（缺音标等强化字段），此时仍需后台补全一次。
	cachedSenses, cacheHit := a.lookupDictionarySenses(r.Context(), wordKey)
	needsEnrichment := cacheHit && !sensesEnriched(cachedSenses)
	translating := !cacheHit || needsEnrichment
	initialSenses := []Sense{}
	if cacheHit {
		initialSenses = cachedSenses
	}
	sensesJSON, err := json.Marshal(initialSenses)
	if err != nil {
		sensesJSON = []byte("[]")
	}

	wordID, err := a.words.Insert(r.Context(), user.ID, wordKey, raw, sensesJSON, translating, now)
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

	if translating {
		a.spawnTranslation(wordID, wordKey)
	}

	newWord := Word{
		ID:               wordID,
		WordKey:          wordKey,
		DisplayWord:      raw,
		Senses:           initialSenses,
		ImportantGlosses: []string{},
		Translating:      translating,
		ReviewCount:      1,
		FirstAddedAt:     now,
		LastReviewedAt:   now,
	}
	writeJSON(w, http.StatusCreated, newWord)
}

// translateRetryDelays 查词失败时的重试间隔，用完仍失败才彻底放弃。
// 收紧到 2 次重试（首次 + 2 次 = 共 3 次尝试），缩短用户看到「查询中」的最长时间。
var translateRetryDelays = []time.Duration{2 * time.Second, 5 * time.Second}

// failedSenses 查词彻底失败后写入用户单词表的占位释义。pos 统一用 "error"，前端据此
// 区分「错误」与「正常释义」：错误时显示重试按钮而非归档。
// 注意：它只写回用户个人的 words.senses，绝不写进全局词库缓存 word_dictionary，
// 否则同一个词会被其它用户命中缓存、永远锁死在错误提示上，无法再被正常重试。
var failedSenses = []Sense{{Pos: "error", Translation: "查询失败，请稍后重试"}}

// spellingErrorSenses 模型判定单词拼写错误时写入的占位释义：提示用户检查拼写。
// 同样只写用户个人 words，不写全局词库缓存。
var spellingErrorSenses = []Sense{{Pos: "error", Translation: "请检查单词拼写是否正常"}}

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

// startStuckTranslationSweeper 周期性扫描并重新触发长时间卡在 translating=1 的查词任务。
// 正常情况下，重试预算耗尽后任务会写回失败占位提示并置 translating=0；但仍有一类罕见故障：
// goroutine 在写最终结果这一步本身失败或异常退出，导致 translating 永远停在 1。这个周期扫描
// 是唯一的自愈手段，否则这些单词会永久显示「查询中」直到下次进程重启。
func (a *App) startStuckTranslationSweeper() {
	ticker := time.NewTicker(stuckSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.sweepStuckTranslations()
		case <-a.bgCtx.Done():
			return
		}
	}
}

// sweepStuckTranslations 单次扫描：只捞取 translating=1 且查词开始时间早于阈值的记录重新触发，
// 正在合法重试中的任务开始时间刚被刷新过，不会误伤。
func (a *App) sweepStuckTranslations() {
	threshold := time.Now().Add(-stuckTranslationThreshold)
	stuck, err := a.words.FindTranslatingStale(context.Background(), threshold)
	if err != nil {
		log.Printf("扫描卡死的查词任务失败: %v", err)
		return
	}
	for _, wd := range stuck {
		log.Printf("重新触发长时间卡死的查词任务 word=%s", wd.WordKey)
		a.spawnTranslation(wd.ID, wd.WordKey)
	}
}

// translateAndSave 在后台异步查词，查完再把释义写回数据库和全局词库缓存，不阻塞单词的录入请求；
// 查词失败会按退避间隔重试，避免偶发失败导致释义永久空白；ctx 取消时（进程关闭）提前退出。
func (a *App) translateAndSave(ctx context.Context, wordID int, wordKey string) {
	// 标记本轮查词开始时间，供周期性扫描判断任务是否卡死。每次重新触发都会刷新，
	// 避免把正在合法重试中的任务误判成卡死。
	if err := a.words.MarkTranslationStarted(ctx, wordID, time.Now()); err != nil {
		log.Printf("标记查词开始时间失败 word=%s id=%d: %v", wordKey, wordID, err)
	}

	for attempt := 0; ; attempt++ {
		cfg := a.getDeepSeekConfig()
		result := translateWord(ctx, wordKey, cfg)
		if result.IsSpellingError {
			// 拼写错误不是重试能解决的，直接写回拼写错误占位，不进入重试循环
			a.saveWordSenses(ctx, wordID, wordKey, spellingErrorSenses)
			return
		}
		merged := mergeSensesByPos(result.Senses)
		if len(merged) > 0 {
			a.saveWordSenses(ctx, wordID, wordKey, merged)
			a.saveDictionarySenses(ctx, wordKey, merged)
			// 查词成功后后台预生成发音（豆包 TTS），点喇叭时秒响；失败静默，播放时再兜底
			a.spawnTTS(wordKey)
			return
		}
		if attempt >= len(translateRetryDelays) {
			log.Printf("查词多次重试后仍失败，放弃 word=%s", wordKey)
			// 写入失败占位提示而非空释义，让用户和管理员都能明确看到「查询失败」。
			// 只写用户自己的 words，不写全局词库缓存，避免污染后续命中缓存的其它用户。
			a.saveWordSenses(ctx, wordID, wordKey, failedSenses)
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

	// 再次录入同一词时，如果释义还是旧数据（缺音标等强化字段）且不在查词中，后台补全一次；
	// translating 标志既让前端显示「查词中」并轮询，也防止快速连点重复触发查词。
	if !sensesEnriched(existing.Senses) && !existing.Translating {
		if err := a.words.MarkTranslating(r.Context(), existing.ID, now); err != nil {
			log.Printf("标记补全状态失败 word=%s: %v", wordKey, err)
		} else {
			existing.Translating = true
			a.spawnTranslation(existing.ID, existing.WordKey)
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
	defaultPageLimit = 20
	maxPageLimit     = 200
	statsTrendDays   = 14
	// flashcardGroupSize 闪卡复习每组取出的卡片数量，背完一组后「再来一组」取下一批
	flashcardGroupSize = 30
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
	keyword := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("keyword")))
	status := r.URL.Query().Get("status")
	page, limit, offset := parsePagination(r)

	total, err := a.words.CountByUser(r.Context(), user.ID, archived, keyword, status)
	if err != nil {
		log.Printf("统计单词总数失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	list, err := a.words.ListPage(r.Context(), user.ID, archived, keyword, status, sort, limit, offset)
	if err != nil {
		log.Printf("查询列表失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	writeJSON(w, http.StatusOK, newPageResult(list, total, page, limit))
}

// handleResetReviewCounts 把当前用户所有单词的背诵次数重置为 1（幂等）。个人中心的
// 「重置次数」按钮调用，前端做二次确认。只动 review_count，不影响 SRS 排期与最近复习时间。
func (a *App) handleResetReviewCounts(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if _, err := a.words.ResetReviewCounts(r.Context(), user.ID); err != nil {
		log.Printf("重置单词次数失败 user_id=%d: %v", user.ID, err)
		writeError(w, http.StatusInternalServerError, "重置失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// handleLookupWord 精确查当前用户是否已录入某个单词，供例句/近反义/形近词点击查询用。
// 命中返回该词完整信息（含释义），未命中返回 null（JSON），前端据此决定是否「自动录入」一次。
func (a *App) handleLookupWord(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	wordKey := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("word")))
	if wordKey == "" {
		writeError(w, http.StatusBadRequest, "单词不能为空")
		return
	}

	wd, sensesRaw, err := a.words.FindByUserAndKey(r.Context(), user.ID, wordKey)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	if err != nil {
		log.Printf("查询单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if len(sensesRaw) > 0 {
		if err := json.Unmarshal(sensesRaw, &wd.Senses); err != nil {
			log.Printf("解析释义失败: %v", err)
		}
	}
	writeJSON(w, http.StatusOK, wd)
}

// handleWordStats 返回统计页需要的聚合数值。列表改成分页后前端拿不到全量数据，
// 这些数字必须由后端用 SQL 聚合算出。
func (a *App) handleWordStats(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	// 趋势图展示最近 statsTrendDays 天（含今天），起点取本地时区当天零点。
	// todaySince / todayUntil 划定今日背诵次数的统计窗口：[00:00:00, 23:59:59.999...)
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowMidnight := midnight.AddDate(0, 0, 1)
	since := midnight.AddDate(0, 0, -(statsTrendDays - 1))
	// since7 划定词云的「近 7 天」窗口（含今天，共 7 天）
	since7 := midnight.AddDate(0, 0, -6)

	stats, err := a.words.Stats(r.Context(), user.ID, since, since7, midnight, tomorrowMidnight)
	if err != nil {
		log.Printf("查询统计数据失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// flashcardReviewRequest 一次闪卡自评请求：Rating 只能是 good / hard / again 三档之一
type flashcardReviewRequest struct {
	ID     int    `json:"id"`
	Rating string `json:"rating"`
}

// validFlashcardRating 校验自评档位，只接受四个合法值，非法值一律拒绝。
func validFlashcardRating(rating string) bool {
	return rating == "good" || rating == "hard" || rating == "fuzzy" || rating == "again"
}

// handleReviewColors 只暴露非敏感的渐变配置给登录用户；写入仍由管理员设置接口负责。
func (a *App) handleReviewColors(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.getReviewColorConfig())
}

// handleFlashcardQueue 返回当前用户到期闪卡队列的一组（最多 flashcardGroupSize 张），前端本地翻卡自评
func (a *App) handleFlashcardQueue(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	list, err := a.words.DueFlashcards(r.Context(), user.ID, flashcardGroupSize, time.Now())
	if err != nil {
		log.Printf("查询闪卡队列失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleFlashcardReview 处理一次闪卡自评：读当前排期状态 → 按 SRS 算新间隔/难度 → 写回，返回更新后的词
func (a *App) handleFlashcardReview(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	var req flashcardReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	if req.ID <= 0 || !validFlashcardRating(req.Rating) {
		writeError(w, http.StatusBadRequest, "无效的评分")
		return
	}

	list, err := a.words.FindByIDs(r.Context(), user.ID, []int{req.ID})
	if err != nil {
		log.Printf("查询单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if len(list) == 0 {
		writeError(w, http.StatusNotFound, "单词不存在")
		return
	}
	wd := list[0]

	now := time.Now()
	intervalDays, easeFactor := applySRSScheduling(wd.IntervalDays, wd.EaseFactor, req.Rating)
	dueAt := now.AddDate(0, 0, intervalDays)
	newCount := wd.ReviewCount + 1
	// 「记住」直接归档（学完不再复习），困难/模糊/不认识保持未归档、按 SRS 排期
	archived := req.Rating == "good"

	if err := a.words.ApplyFlashcardReview(r.Context(), req.ID, user.ID, newCount, intervalDays, easeFactor, dueAt, now, archived); err != nil {
		log.Printf("更新闪卡复习状态失败: %v", err)
		writeError(w, http.StatusInternalServerError, "更新失败")
		return
	}

	wd.ReviewCount = newCount
	wd.LastReviewedAt = now
	wd.IntervalDays = intervalDays
	wd.EaseFactor = easeFactor
	wd.DueAt = &dueAt
	wd.Archived = archived

	// 释义缺扩展信息且用户点「不认识」时，后台补一次查词（复用录入查词链路）
	if req.Rating == "again" && !wd.Translating && sensesNeedEnrichment(wd.Senses) {
		if err := a.words.MarkTranslating(r.Context(), req.ID, now); err != nil {
			log.Printf("标记释义补全失败 word=%s id=%d: %v", wd.WordKey, req.ID, err)
		} else {
			a.spawnTranslation(req.ID, wd.WordKey)
		}
	}

	writeJSON(w, http.StatusOK, wd)
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

// handleRetryWord 用户对「无释义 / 查询失败 / 拼写错误」的单词主动重新触发一次查词。
// 复用后台查词链路：置 translating=1 并 spawnTranslation，前端随后轮询拿回新结果。
func (a *App) handleRetryWord(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}

	list, err := a.words.FindByIDs(r.Context(), user.ID, []int{id})
	if err != nil {
		log.Printf("查询单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if len(list) == 0 {
		writeError(w, http.StatusNotFound, "单词不存在")
		return
	}
	wd := list[0]
	if wd.Translating {
		// 已在查询中，直接返回，避免重复触发
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := a.words.MarkTranslating(r.Context(), id, time.Now()); err != nil {
		log.Printf("标记重查状态失败 word=%s id=%d: %v", wd.WordKey, id, err)
		writeError(w, http.StatusInternalServerError, "重新查询失败")
		return
	}
	a.spawnTranslation(id, wd.WordKey)
	w.WriteHeader(http.StatusNoContent)
}

// importantGlossesRequest 设置某单词「重要释义」的请求：glosses 是被标记为重要的义项文本数组，全量覆盖
type importantGlossesRequest struct {
	Glosses []string `json:"glosses"`
}

// importantGlossesResponse 覆盖写成功后的返回：规范化后的义项列表，前端据此同步本地状态
type importantGlossesResponse struct {
	ID               int      `json:"id"`
	ImportantGlosses []string `json:"important_glosses"`
}

// handleSetWordImportant 覆盖设置某单词的重要释义义项列表。重要标记是用户手标的个人数据，
// 存 words.important_glosses 独立列，与 LLM 生成的 senses 分列，后台查词写回时不会被冲掉。
func (a *App) handleSetWordImportant(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}

	var req importantGlossesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Glosses == nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	glosses := normalizeGlosses(req.Glosses)

	list, err := a.words.FindByIDs(r.Context(), user.ID, []int{id})
	if err != nil {
		log.Printf("查询单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	if len(list) == 0 {
		writeError(w, http.StatusNotFound, "单词不存在")
		return
	}

	glossesJSON, err := json.Marshal(glosses)
	if err != nil {
		glossesJSON = []byte("[]")
	}
	if err := a.words.UpdateImportantGlosses(r.Context(), id, user.ID, glossesJSON); err != nil {
		log.Printf("更新重要释义失败: %v", err)
		writeError(w, http.StatusInternalServerError, "更新失败")
		return
	}
	writeJSON(w, http.StatusOK, importantGlossesResponse{ID: id, ImportantGlosses: glosses})
}

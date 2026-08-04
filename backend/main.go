package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

func main() {
	connectDB()
	defer db.Close()

	migrateSchema()
	adminID := bootstrapAdmin()
	finalizeWordsUserID(adminID)
	loadSettings()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", handleLogin)
	mux.HandleFunc("POST /api/logout", handleLogout)
	mux.HandleFunc("GET /api/me", requireAuth(handleMe))
	mux.HandleFunc("PUT /api/me/password", requireAuth(handleChangePassword))

	mux.HandleFunc("POST /api/words", requireAuth(handleAddWord))
	mux.HandleFunc("GET /api/words", requireAuth(handleListWords))
	mux.HandleFunc("DELETE /api/words/{id}", requireAuth(handleDeleteWord))
	mux.HandleFunc("POST /api/words/{id}/archive", requireAuth(handleArchiveWord))
	mux.HandleFunc("POST /api/words/{id}/unarchive", requireAuth(handleUnarchiveWord))

	mux.HandleFunc("POST /api/admin/users", requireAdmin(handleCreateUser))
	mux.HandleFunc("GET /api/admin/users", requireAdmin(handleListUsers))
	mux.HandleFunc("POST /api/admin/users/{id}/reset-password", requireAdmin(handleResetUserPassword))
	mux.HandleFunc("GET /api/admin/settings", requireAdmin(handleGetSettings))
	mux.HandleFunc("PUT /api/admin/settings", requireAdmin(handleUpdateSettings))
	mux.HandleFunc("GET /api/admin/dictionary", requireAdmin(handleListDictionary))
	mux.HandleFunc("GET /api/admin/dictionary/export", requireAdmin(handleExportDictionary))
	mux.HandleFunc("DELETE /api/admin/dictionary/{word_key}", requireAdmin(handleDeleteDictionaryEntry))

	mux.HandleFunc("/", spaHandler)

	addr := ":8080"
	log.Printf("服务启动，监听 %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

type addWordRequest struct {
	Word string `json:"word"`
}

func handleAddWord(w http.ResponseWriter, r *http.Request) {
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
	if handled := tryIncrementExisting(w, user.ID, wordKey, now); handled {
		return
	}

	// 不管是不是这个用户第一次输入，先登记一次全局词库的出现次数
	upsertDictionaryOccurrence(wordKey, raw, now)

	// 全局词库如果已经缓存过这个词的释义，直接复用，不用再问一次大模型
	cachedSenses, cacheHit := lookupDictionarySenses(wordKey)
	initialSenses := []Sense{}
	if cacheHit {
		initialSenses = cachedSenses
	}
	sensesJSON, err := json.Marshal(initialSenses)
	if err != nil {
		sensesJSON = []byte("[]")
	}

	res, err := db.Exec(
		`INSERT INTO words (user_id, word_key, display_word, senses, translating, review_count, first_added_at, last_reviewed_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		user.ID, wordKey, raw, sensesJSON, !cacheHit, now, now,
	)
	if err != nil {
		// 并发下可能有另一个请求刚好抢先插入了同一个单词，退化为累加次数
		if mysqlErr, ok := err.(*mysqldriver.MySQLError); ok && mysqlErr.Number == 1062 {
			if handled := tryIncrementExisting(w, user.ID, wordKey, now); handled {
				return
			}
		}
		log.Printf("插入单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "保存失败")
		return
	}
	id, _ := res.LastInsertId()
	wordID := int(id)

	if !cacheHit {
		go translateAndSave(wordID, wordKey)
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

// translateAndSave 在后台异步查词，查完再把释义写回数据库和全局词库缓存，不阻塞单词的录入请求；
// 查词失败会按退避间隔重试，避免偶发失败导致释义永久空白
func translateAndSave(wordID int, wordKey string) {
	for attempt := 0; ; attempt++ {
		result := translateWord(wordKey)
		merged := mergeSensesByPos(result.Senses)
		if len(merged) > 0 {
			saveWordSenses(wordID, wordKey, merged)
			saveDictionarySenses(wordKey, merged)
			return
		}
		if attempt >= len(translateRetryDelays) {
			log.Printf("查词多次重试后仍失败，放弃 word=%s", wordKey)
			saveWordSenses(wordID, wordKey, []Sense{})
			return
		}
		delay := translateRetryDelays[attempt]
		log.Printf("查词失败，%s 后重试 word=%s attempt=%d", delay, wordKey, attempt+1)
		time.Sleep(delay)
	}
}

func saveWordSenses(wordID int, wordKey string, senses []Sense) {
	sensesJSON, err := json.Marshal(senses)
	if err != nil {
		log.Printf("序列化释义失败 word=%s: %v", wordKey, err)
		sensesJSON = []byte("[]")
	}
	if _, err := db.Exec(`UPDATE words SET senses = ?, translating = 0 WHERE id = ?`, sensesJSON, wordID); err != nil {
		log.Printf("写回释义失败 word=%s id=%d: %v", wordKey, wordID, err)
	}
}

// tryIncrementExisting 如果当前用户名下 wordKey 已存在，则次数 +1、更新最近背诵时间，并写响应；
// 返回 true 表示请求已经处理完毕（无论是成功还是出错），调用方不需要再做任何事。
func tryIncrementExisting(w http.ResponseWriter, userID int, wordKey string, now time.Time) bool {
	var existing Word
	var sensesRaw []byte
	err := db.QueryRow(
		`SELECT id, word_key, display_word, senses, translating, archived, review_count, first_added_at, last_reviewed_at FROM words WHERE user_id = ? AND word_key = ?`,
		userID, wordKey,
	).Scan(&existing.ID, &existing.WordKey, &existing.DisplayWord, &sensesRaw, &existing.Translating, &existing.Archived, &existing.ReviewCount, &existing.FirstAddedAt, &existing.LastReviewedAt)

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
	if _, err := db.Exec(`UPDATE words SET review_count = ?, last_reviewed_at = ? WHERE id = ?`, newCount, now, existing.ID); err != nil {
		log.Printf("更新单词失败: %v", err)
		writeError(w, http.StatusInternalServerError, "更新失败")
		return true
	}
	upsertDictionaryOccurrence(existing.WordKey, existing.DisplayWord, now)
	existing.ReviewCount = newCount
	existing.LastReviewedAt = now
	writeJSON(w, http.StatusOK, existing)
	return true
}

func handleListWords(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	archived := r.URL.Query().Get("archived") == "1"
	rows, err := db.Query(
		`SELECT id, word_key, display_word, senses, translating, archived, review_count, first_added_at, last_reviewed_at FROM words WHERE user_id = ? AND archived = ? ORDER BY review_count DESC, last_reviewed_at DESC`,
		user.ID, archived,
	)
	if err != nil {
		log.Printf("查询列表失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()

	list := []Word{}
	for rows.Next() {
		var wd Word
		var sensesRaw []byte
		if err := rows.Scan(&wd.ID, &wd.WordKey, &wd.DisplayWord, &sensesRaw, &wd.Translating, &wd.Archived, &wd.ReviewCount, &wd.FirstAddedAt, &wd.LastReviewedAt); err != nil {
			log.Printf("读取记录失败: %v", err)
			continue
		}
		if len(sensesRaw) > 0 {
			if err := json.Unmarshal(sensesRaw, &wd.Senses); err != nil {
				log.Printf("解析释义失败: %v", err)
			}
		}
		list = append(list, wd)
	}
	writeJSON(w, http.StatusOK, list)
}

func handleDeleteWord(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}
	if _, err := db.Exec(`DELETE FROM words WHERE id = ? AND user_id = ?`, id, user.ID); err != nil {
		log.Printf("删除失败: %v", err)
		writeError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleArchiveWord(w http.ResponseWriter, r *http.Request) {
	setWordArchived(w, r, true)
}

func handleUnarchiveWord(w http.ResponseWriter, r *http.Request) {
	setWordArchived(w, r, false)
}

// setWordArchived 归档/取消归档只是给单词打个标记，不涉及删除，不需要二次确认
func setWordArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	user := currentUser(r)
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}
	if _, err := db.Exec(`UPDATE words SET archived = ? WHERE id = ? AND user_id = ?`, archived, id, user.ID); err != nil {
		log.Printf("更新归档状态失败: %v", err)
		writeError(w, http.StatusInternalServerError, "更新失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
}

func handleCreateUser(w http.ResponseWriter, r *http.Request) {
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

	user, err := createUser(username, req.Password, req.IsAdmin)
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

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, username, is_admin, created_at FROM users ORDER BY id`)
	if err != nil {
		log.Printf("查询用户列表失败: %v", err)
		writeError(w, http.StatusInternalServerError, "查询失败")
		return
	}
	defer rows.Close()

	list := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt); err != nil {
			log.Printf("读取用户记录失败: %v", err)
			continue
		}
		list = append(list, u)
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

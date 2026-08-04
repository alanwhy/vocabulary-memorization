package main

import (
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

	mux.HandleFunc("POST /api/words", requireAuth(handleAddWord))
	mux.HandleFunc("GET /api/words", requireAuth(handleListWords))
	mux.HandleFunc("DELETE /api/words/{id}", requireAuth(handleDeleteWord))
	mux.HandleFunc("POST /api/words/{id}/archive", requireAuth(handleArchiveWord))
	mux.HandleFunc("POST /api/words/{id}/unarchive", requireAuth(handleUnarchiveWord))

	mux.HandleFunc("POST /api/admin/users", requireAdmin(handleCreateUser))
	mux.HandleFunc("GET /api/admin/users", requireAdmin(handleListUsers))
	mux.HandleFunc("GET /api/admin/settings", requireAdmin(handleGetSettings))
	mux.HandleFunc("PUT /api/admin/settings", requireAdmin(handleUpdateSettings))

	mux.Handle("/", http.FileServer(http.Dir("./static")))

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

	// 不存在：先原样插入，标记为查词中，立即返回；释义交给后台异步查询后再写回
	res, err := db.Exec(
		`INSERT INTO words (user_id, word_key, display_word, senses, translating, review_count, first_added_at, last_reviewed_at) VALUES (?, ?, ?, ?, 1, 1, ?, ?)`,
		user.ID, wordKey, raw, []byte("[]"), now, now,
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

	go translateAndSave(wordID, wordKey)

	newWord := Word{
		ID:             wordID,
		WordKey:        wordKey,
		DisplayWord:    raw,
		Senses:         []Sense{},
		Translating:    true,
		ReviewCount:    1,
		FirstAddedAt:   now,
		LastReviewedAt: now,
	}
	writeJSON(w, http.StatusCreated, newWord)
}

// translateAndSave 在后台异步查词，查完再把释义写回数据库，不阻塞单词的录入请求
func translateAndSave(wordID int, wordKey string) {
	result := translateWord(wordKey)
	sensesJSON, err := json.Marshal(result.Senses)
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

package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "session_token"
const sessionTTL = 30 * 24 * time.Hour

type contextKey string

const userContextKey contextKey = "user"

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// bootstrapAdmin 确保至少存在一个超管账号；如果没有，用环境变量里的用户名密码创建一个。
// 返回超管的 user id，供历史数据迁移时兜底使用。
func bootstrapAdmin() int {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&count); err != nil {
		log.Fatalf("查询超管账号失败: %v", err)
	}

	if count == 0 {
		username := getEnv("ADMIN_USERNAME", "admin")
		password := os.Getenv("ADMIN_PASSWORD")
		if password == "" {
			password = randomToken()[:16]
			log.Printf("未设置 ADMIN_PASSWORD，随机生成了超管密码，请妥善记录：用户名=%s 密码=%s", username, password)
		}
		if _, err := createUser(username, password, true); err != nil {
			log.Fatalf("创建超管账号失败: %v", err)
		}
		log.Printf("超管账号已创建：%s", username)
	}

	var adminID int
	if err := db.QueryRow(`SELECT id FROM users WHERE is_admin = 1 ORDER BY id LIMIT 1`).Scan(&adminID); err != nil {
		log.Fatalf("读取超管账号失败: %v", err)
	}
	return adminID
}

func createUser(username, password string, isAdmin bool) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	now := time.Now()
	res, err := db.Exec(
		`INSERT INTO users (username, password_hash, is_admin, created_at) VALUES (?, ?, ?, ?)`,
		username, string(hash), isAdmin, now,
	)
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{ID: int(id), Username: username, IsAdmin: isAdmin, CreatedAt: now}, nil
}

type resetPasswordRequest struct {
	Password string `json:"password"`
}

// handleResetUserPassword 管理员重置指定用户的密码；重置后该用户所有已登录会话立即失效，需要重新登录
func handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}
	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	if len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "密码长度至少 6 位")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("生成密码哈希失败: %v", err)
		writeError(w, http.StatusInternalServerError, "重置失败")
		return
	}
	if _, err := db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), id); err != nil {
		log.Printf("重置密码失败: %v", err)
		writeError(w, http.StatusInternalServerError, "重置失败")
		return
	}
	db.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)
	w.WriteHeader(http.StatusNoContent)
}

func randomToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("生成随机 token 失败: %v", err)
	}
	return hex.EncodeToString(buf)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	username := strings.TrimSpace(req.Username)

	var user User
	var hash string
	err := db.QueryRow(
		`SELECT id, username, password_hash, is_admin, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &hash, &user.IsAdmin, &user.CreatedAt)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if err != nil {
		log.Printf("查询用户失败: %v", err)
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	token := randomToken()
	now := time.Now()
	expiresAt := now.Add(sessionTTL)
	if _, err := db.Exec(
		`INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		token, user.ID, expiresAt, now,
	); err != nil {
		log.Printf("创建会话失败: %v", err)
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, user)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		db.Exec(`DELETE FROM sessions WHERE token = ?`, cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

// currentUser 从请求上下文里取出 requireAuth 已经校验好的当前用户
func currentUser(r *http.Request) *User {
	u, _ := r.Context().Value(userContextKey).(*User)
	return u
}

// requireAuth 校验会话 cookie，未登录返回 401；登录成功后把用户信息塞进 context 供后续 handler 使用，
// 同时按滑动过期策略续期会话。
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}

		var user User
		var expiresAt time.Time
		err = db.QueryRow(
			`SELECT u.id, u.username, u.is_admin, u.created_at, s.expires_at
			 FROM sessions s JOIN users u ON u.id = s.user_id
			 WHERE s.token = ?`,
			cookie.Value,
		).Scan(&user.ID, &user.Username, &user.IsAdmin, &user.CreatedAt, &expiresAt)

		if err == sql.ErrNoRows || (err == nil && time.Now().After(expiresAt)) {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		if err != nil {
			log.Printf("校验会话失败: %v", err)
			writeError(w, http.StatusInternalServerError, "登录状态校验失败")
			return
		}

		newExpiry := time.Now().Add(sessionTTL)
		db.Exec(`UPDATE sessions SET expires_at = ? WHERE token = ?`, newExpiry, cookie.Value)

		ctx := context.WithValue(r.Context(), userContextKey, &user)
		next(w, r.WithContext(ctx))
	}
}

// requireAdmin 在 requireAuth 基础上再要求当前用户是超管
func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).IsAdmin {
			writeError(w, http.StatusForbidden, "无权限")
			return
		}
		next(w, r)
	})
}

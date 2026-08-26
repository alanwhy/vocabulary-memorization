package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 30 * 24 * time.Hour

type contextKey string

const userContextKey contextKey = "user"

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse 登录成功响应：token 供前端存本地并在后续请求带 Authorization 头，user 直接给前端展示
type loginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// bootstrapAdmin 确保至少存在一个超管账号；如果没有，用环境变量里的用户名密码创建一个。
// 返回超管的 user id，供历史数据迁移时兜底使用。
func (a *App) bootstrapAdmin() int {
	ctx := context.Background()
	count, err := a.users.CountAdmins(ctx)
	if err != nil {
		log.Fatalf("查询超管账号失败: %v", err)
	}

	if count == 0 {
		username := getEnv("ADMIN_USERNAME", "admin")
		password := os.Getenv("ADMIN_PASSWORD")
		if password == "" {
			password = randomToken()[:16]
			log.Printf("未设置 ADMIN_PASSWORD，随机生成了超管密码，请妥善记录：用户名=%s 密码=%s", username, password)
		}
		if _, err := a.createUser(ctx, username, password, true); err != nil {
			log.Fatalf("创建超管账号失败: %v", err)
		}
		log.Printf("超管账号已创建：%s", username)
	}

	adminID, err := a.users.FirstAdminID(ctx)
	if err != nil {
		log.Fatalf("读取超管账号失败: %v", err)
	}
	return adminID
}

func (a *App) createUser(ctx context.Context, username, password string, isAdmin bool) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	return a.users.Insert(ctx, username, string(hash), isAdmin, time.Now())
}

// passwordCharset 生成随机密码用的字符集，避开容易混淆的字符（0/O、1/l/I）
const passwordCharset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

func generateRandomPassword(length int) string {
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordCharset))))
		if err != nil {
			log.Fatalf("生成随机密码失败: %v", err)
		}
		b[i] = passwordCharset[n.Int64()]
	}
	return string(b)
}

// handleResetUserPassword 管理员重置指定用户的密码：后端自动生成随机密码并返回明文供管理员复制，
// 重置后该用户所有已登录会话立即失效，需要用新密码重新登录
func (a *App) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}
	newPassword := generateRandomPassword(10)
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("生成密码哈希失败: %v", err)
		writeError(w, http.StatusInternalServerError, "重置失败")
		return
	}
	affected, err := a.users.UpdatePasswordHash(r.Context(), id, string(hash))
	if err != nil {
		log.Printf("重置密码失败: %v", err)
		writeError(w, http.StatusInternalServerError, "重置失败")
		return
	}
	if affected == 0 {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err := a.sessions.DeleteByUser(r.Context(), id); err != nil {
		log.Printf("清除会话失败 user_id=%d: %v", id, err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"password": newPassword})
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// handleChangePassword 登录用户自助修改自己的密码，需要正确提供旧密码；当前会话仍然有效，
// 其它已登录的会话会被清除，防止密码泄露后攻击者持有的旧会话继续可用。
func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	limiterKey := "uid:" + strconv.Itoa(user.ID)
	if a.pwLimiter.Locked(limiterKey) {
		writeError(w, http.StatusTooManyRequests, "尝试次数过多，请稍后再试")
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	if len(req.NewPassword) < 6 {
		writeError(w, http.StatusBadRequest, "新密码长度至少 6 位")
		return
	}
	hash, err := a.users.FindPasswordHash(r.Context(), user.ID)
	if err != nil {
		log.Printf("查询密码失败: %v", err)
		writeError(w, http.StatusInternalServerError, "修改失败")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.OldPassword)) != nil {
		a.pwLimiter.RecordFailure(limiterKey)
		writeError(w, http.StatusBadRequest, "原密码错误")
		return
	}
	a.pwLimiter.Reset(limiterKey)
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("生成密码哈希失败: %v", err)
		writeError(w, http.StatusInternalServerError, "修改失败")
		return
	}
	if _, err := a.users.UpdatePasswordHash(r.Context(), user.ID, string(newHash)); err != nil {
		log.Printf("更新密码失败: %v", err)
		writeError(w, http.StatusInternalServerError, "修改失败")
		return
	}
	if token := bearerToken(r); token != "" {
		if err := a.sessions.DeleteByUserExcept(r.Context(), user.ID, token); err != nil {
			log.Printf("清除旧会话失败 user_id=%d: %v", user.ID, err)
		}
	} else if err := a.sessions.DeleteByUser(r.Context(), user.ID); err != nil {
		log.Printf("清除旧会话失败 user_id=%d: %v", user.ID, err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func randomToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("生成随机 token 失败: %v", err)
	}
	return hex.EncodeToString(buf)
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	username := strings.TrimSpace(req.Username)
	ipKey := "ip:" + clientIP(r)
	userKey := "user:" + strings.ToLower(username)
	if a.loginLimiter.Locked(ipKey) || a.loginLimiter.Locked(userKey) {
		writeError(w, http.StatusTooManyRequests, "尝试次数过多，请稍后再试")
		return
	}

	user, hash, err := a.users.FindByUsername(r.Context(), username)

	if err == sql.ErrNoRows {
		a.loginLimiter.RecordFailure(ipKey)
		a.loginLimiter.RecordFailure(userKey)
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if err != nil {
		log.Printf("查询用户失败: %v", err)
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		a.loginLimiter.RecordFailure(ipKey)
		a.loginLimiter.RecordFailure(userKey)
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	a.loginLimiter.Reset(ipKey)
	a.loginLimiter.Reset(userKey)

	token := randomToken()
	now := time.Now()
	expiresAt := now.Add(sessionTTL)
	if err := a.sessions.Create(r.Context(), token, user.ID, expiresAt, now); err != nil {
		log.Printf("创建会话失败: %v", err)
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}

	// 记录最后登录时间；这只是展示用的信息，写失败不该拦住已经通过校验的登录
	if err := a.users.RecordLogin(r.Context(), user.ID, now); err != nil {
		log.Printf("记录最后登录时间失败 user_id=%d: %v", user.ID, err)
	}
	user.LastLoginAt = &now

	writeJSON(w, http.StatusOK, loginResponse{Token: token, User: user})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := bearerToken(r); token != "" {
		if err := a.sessions.DeleteByToken(r.Context(), token); err != nil {
			log.Printf("清除会话失败: %v", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// clientIP 提取客户端 IP 用于登录限流；当前部署无反向代理，直接读 RemoteAddr，
// 如果未来接入反代需要改读 X-Forwarded-For，并需确认该 header 可信（避免伪造绕过限流）。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

// currentUser 从请求上下文里取出 requireAuth 已经校验好的当前用户
func currentUser(r *http.Request) *User {
	u, _ := r.Context().Value(userContextKey).(*User)
	return u
}

// bearerToken 从 Authorization 头里提取 Bearer token；格式必须是 "Bearer <token>"。
// 缺失、大小写不对或 token 为空都返回空串，由 requireAuth 统一按未登录处理。
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return ""
	}
	return token
}

// requireAuth 校验 Authorization 头里的 Bearer token，未登录返回 401；登录成功后把用户信息塞进 context 供后续 handler 使用，
// 同时按滑动过期策略续期会话。
func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}

		user, expiresAt, err := a.sessions.FindWithUser(r.Context(), token)

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
		if err := a.sessions.Touch(r.Context(), token, newExpiry); err != nil {
			log.Printf("续期会话失败: %v", err)
		}

		ctx := context.WithValue(r.Context(), userContextKey, &user)
		next(w, r.WithContext(ctx))
	}
}

// requireAdmin 在 requireAuth 基础上再要求当前用户是超管
func (a *App) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return a.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).IsAdmin {
			writeError(w, http.StatusForbidden, "无权限")
			return
		}
		next(w, r)
	})
}

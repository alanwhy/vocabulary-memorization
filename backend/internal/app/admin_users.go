// 用户详情与编辑集中提供账号信息和实时日额度，不依赖列表中的旧快照。
package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	mysqldriver "github.com/go-sql-driver/mysql"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
	"vocab-backend/internal/billing"
	"vocab-backend/internal/model"
)

type userEditor interface {
	FindByID(context.Context, int) (model.UserWithStats, error)
	UpdateProfile(context.Context, int, string, bool, bool, *int64) error
}

type adminUserDetail struct {
	User   model.UserWithStats `json:"user"`
	Budget billing.Snapshot    `json:"budget"`
}

// userDetail 合并账号和当天账本，返回数据库不存在或服务不可用错误。
func (a *App) userDetail(ctx context.Context, id int) (adminUserDetail, error) {
	editor, ok := a.users.(userEditor)
	if !ok || a.billing == nil {
		return adminUserDetail{}, billing.ErrUnavailable
	}
	user, err := editor.FindByID(ctx, id)
	if err != nil {
		return adminUserDetail{}, err
	}
	budget, err := a.billing.Store.Snapshot(ctx, id, billing.Day(a.billing.Now()))
	return adminUserDetail{user, budget}, err
}

// handleUserDetail 仅由管理员路由调用，返回最新账号和额度。
func (a *App) handleUserDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, 400, "无效的用户 id")
		return
	}
	detail, err := a.userDetail(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "用户不存在")
		return
	}
	if err != nil {
		writeError(w, 503, "用户详情暂时无法读取")
		return
	}
	writeJSON(w, 200, detail)
}

// handleEditUser 校验全量编辑字段后一次提交，自己不能禁用或降级。
func (a *App) handleEditUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, 400, "无效的用户 id")
		return
	}
	var req struct {
		Username *string         `json:"username"`
		IsAdmin  *bool           `json:"is_admin"`
		Disabled *bool           `json:"disabled"`
		Limit    json.RawMessage `json:"daily_limit_cny"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || req.Username == nil || req.IsAdmin == nil || req.Disabled == nil {
		writeError(w, 400, "请完整填写用户名、角色、状态和每日额度")
		return
	}
	username := strings.TrimSpace(*req.Username)
	if username == "" || utf8.RuneCountInString(username) > 64 {
		writeError(w, 400, "用户名须为 1–64 个字符")
		return
	}
	limit, err := billing.ParseLimit(req.Limit)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if id == currentUser(r).ID && (!*req.IsAdmin || *req.Disabled) {
		writeError(w, 400, "不能禁用自己或取消自己的管理员角色")
		return
	}
	editor, ok := a.users.(userEditor)
	if !ok {
		writeError(w, 503, "用户编辑暂不可用")
		return
	}
	err = editor.UpdateProfile(r.Context(), id, username, *req.IsAdmin, *req.Disabled, limit)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "用户不存在")
		return
	}
	if errors.Is(err, billing.ErrLastAdmin) {
		writeError(w, 409, err.Error())
		return
	}
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		writeError(w, 409, "用户名已存在")
		return
	}
	if err != nil {
		writeError(w, 500, "保存用户失败")
		return
	}
	if *req.Disabled {
		_ = a.sessions.DeleteByUser(r.Context(), id)
	}
	detail, err := a.userDetail(r.Context(), id)
	if err != nil {
		writeJSON(w, 200, map[string]any{"saved": true})
		return
	}
	writeJSON(w, 200, detail)
}

// handleAIBudget 只查询当前登录用户额度，不接受客户端指定其他用户。
func (a *App) handleAIBudget(w http.ResponseWriter, r *http.Request) {
	if a.billing == nil {
		writeError(w, 503, "AI 额度服务暂不可用")
		return
	}
	snapshot, err := a.billing.Store.Snapshot(r.Context(), currentUser(r).ID, billing.Day(a.billing.Now()))
	if err != nil {
		writeError(w, 503, "AI 额度暂时无法读取")
		return
	}
	writeJSON(w, 200, snapshot)
}

// writeAIError 把不可重试的额度错误明确返回前端，返回是否已经写响应。
func writeAIError(w http.ResponseWriter, err error) bool {
	if !billing.Terminal(err) {
		return false
	}
	status := http.StatusServiceUnavailable
	code := "ai_billing_unavailable"
	if errors.Is(err, billing.ErrExceeded) {
		status = 429
		code = "ai_budget_exceeded"
	}
	if errors.Is(err, billing.ErrDisabled) {
		status = 403
		code = "account_disabled"
	}
	writeJSON(w, status, map[string]string{"error": err.Error(), "code": code})
	return true
}

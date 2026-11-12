// 每日临时练习接口：只返回当前词，不接受客户端提交答案判定或题库内容。
package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"vocab-backend/internal/training"
)

// decodeTraining 限制请求大小、未知字段和额外 JSON 对象。
func decodeTraining(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return training.Fail(400, "invalid_request", "请求内容无效")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return training.Fail(400, "invalid_request", "请求只能包含一个对象")
	}
	return nil
}

// trainingError 隐藏内部响应，保留可供界面处理的稳定错误码。
func trainingError(w http.ResponseWriter, err error) {
	var p *training.Problem
	if errors.As(err, &p) {
		writeJSON(w, p.Status, map[string]any{"error": p.Message, "code": p.Code})
		return
	}
	writeJSON(w, 500, map[string]any{"error": "读取练习失败，请重试", "code": "training_failed"})
}

// handleTraining 访问内存队列，不创建题库、任务或成绩数据库记录。
func (a *App) handleTraining(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if a.daily == nil {
		trainingError(w, training.Fail(503, "training_unavailable", "练习服务正在启动"))
		return
	}
	user := currentUser(r).ID
	path := strings.TrimPrefix(r.URL.Path, "/api/training/daily")
	var out training.DailySnapshot
	var err error
	switch {
	case r.Method == "GET" && path == "":
		out, err = a.daily.State(r.Context(), user)
	case r.Method == "POST" && path == "":
		out, err = a.daily.Ensure(r.Context(), user, false, "")
	case r.Method == "POST" && path == "/more":
		var input struct {
			RequestID string `json:"request_id"`
		}
		err = decodeTraining(w, r, &input)
		if err == nil && !training.Text(input.RequestID, 64) {
			err = training.Fail(400, "invalid_request_id", "缺少有效请求标识")
		}
		if err == nil {
			out, err = a.daily.Ensure(r.Context(), user, true, input.RequestID)
		}
	case r.Method == "POST" && (path == "/answer" || path == "/advance" || path == "/retry"):
		var input struct {
			GroupID  string  `json:"group_id"`
			WordID   int     `json:"word_id"`
			OptionID *string `json:"option_id"`
		}
		err = decodeTraining(w, r, &input)
		if err == nil && (!training.Text(input.GroupID, 64) || input.WordID <= 0) {
			err = training.Fail(400, "invalid_word", "当前练习或单词无效")
		}
		if err == nil {
			switch path {
			case "/answer":
				out, err = a.daily.Answer(r.Context(), user, input.GroupID, input.WordID, input.OptionID)
			case "/advance":
				out, err = a.daily.Advance(r.Context(), user, input.GroupID, input.WordID)
			case "/retry":
				out, err = a.daily.Retry(r.Context(), user, input.GroupID, input.WordID)
			}
		}
	default:
		err = training.Fail(404, "not_found", "接口不存在")
	}
	if err != nil {
		trainingError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

// registerTrainingRoutes 注册精简接口，沿用已有登录权限和请求超时。
func (a *App) registerTrainingRoutes(mux *http.ServeMux) {
	for _, pattern := range []string{"GET /api/training/daily", "POST /api/training/daily", "POST /api/training/daily/more", "POST /api/training/daily/answer", "POST /api/training/daily/advance", "POST /api/training/daily/retry"} {
		mux.HandleFunc(pattern, withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleTraining)))
	}
}

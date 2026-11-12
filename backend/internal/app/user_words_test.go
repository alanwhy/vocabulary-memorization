package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type syncWordsStore struct {
	WordStore
	userID int
	calls  int
	added  int64
	total  int
	err    error
}

// SyncDictionaryToUser 记录目标用户并返回测试配置的仓储结果，不接触真实数据库。
func (s *syncWordsStore) SyncDictionaryToUser(ctx context.Context, userID int, now time.Time) (int64, int, error) {
	s.userID = userID
	s.calls++
	return s.added, s.total, s.err
}

// TestSyncUserWordsRoute 验证完整管理员路由的鉴权、参数、目标传递和错误契约。
func TestSyncUserWordsRoute(t *testing.T) {
	for _, tc := range []struct {
		name, id, role string
		err            error
		added          int64
		total, status  int
		calls          int
	}{
		{name: "管理员为指定用户同步", id: "42", role: "admin", added: 3, total: 8, status: 200, calls: 1},
		{name: "全部重复仍成功", id: "42", role: "admin", total: 8, status: 200, calls: 1},
		{name: "未登录", id: "42", status: 401},
		{name: "普通用户", id: "42", role: "user", status: 403},
		{name: "非数字ID", id: "abc", role: "admin", status: 400},
		{name: "零ID", id: "0", role: "admin", status: 400},
		{name: "负数ID", id: "-1", role: "admin", status: 400},
		{name: "不存在用户", id: "42", role: "admin", err: sql.ErrNoRows, status: 404, calls: 1},
		{name: "存储失败", id: "42", role: "admin", err: errors.New("database unavailable"), status: 500, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, users, sessions := newTestApp()
			store := &syncWordsStore{added: tc.added, total: tc.total, err: tc.err}
			a.words = store
			req := httptest.NewRequest(http.MethodPost, "/api/admin/users/"+tc.id+"/sync-words", nil)
			if tc.role != "" {
				u := users.addUser(tc.role, "hash", tc.role == "admin")
				sessions.usersByID[u.ID] = u
				sessions.byToken["valid"] = sessionRecord{userID: u.ID, expiresAt: time.Now().Add(time.Hour)}
				req.Header.Set("Authorization", "Bearer valid")
			}
			rec := httptest.NewRecorder()
			a.Handler(t.TempDir()).ServeHTTP(rec, req)
			if rec.Code != tc.status || store.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", rec.Code, store.calls, rec.Body.String())
			}
			if store.calls > 0 && store.userID != 42 {
				t.Fatalf("wrong target: %d", store.userID)
			}
			if rec.Code == http.StatusOK {
				var result struct {
					Added int64 `json:"added_count"`
					Total int   `json:"word_count"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.Added != tc.added || result.Total != tc.total {
					t.Fatalf("unexpected result: %s err=%v", rec.Body.String(), err)
				}
			}
		})
	}
}

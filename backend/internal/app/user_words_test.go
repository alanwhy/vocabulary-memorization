package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type syncWordsStore struct {
	WordStore
	started     chan int
	release     chan struct{}
	err         error
	panicResult bool
}

// SyncDictionaryToUser 通过 release 模拟长任务，started 记录真正的目标账号；ctx 取消时结束。
func (s *syncWordsStore) SyncDictionaryToUser(ctx context.Context, userID int, now time.Time) (int64, int, error) {
	s.started <- userID
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	case <-s.release:
	}
	if s.panicResult {
		panic("test task panic")
	}
	return 3, 8, s.err
}

// wordSyncApp 构造有独立后台生命周期的测试应用，并确保所有后台任务在测试结束时退出。
func wordSyncApp(t *testing.T, store *syncWordsStore) (*App, *fakeUserStore, *fakeSessionStore) {
	t.Helper()
	a, users, sessions := newTestApp()
	a.words = store
	a.bgCtx, a.bgCancel = context.WithCancel(context.Background())
	t.Cleanup(func() {
		a.CancelBackground()
		if !a.WaitBackground(time.Second) {
			t.Error("background sync did not stop")
		}
	})
	return a, users, sessions
}

// wordSyncRequest 通过完整路由发起 method/token 请求，返回响应及本人任务快照。
func wordSyncRequest(t *testing.T, a *App, method, token string) (int, wordSyncStatus) {
	t.Helper()
	req := httptest.NewRequest(method, "/api/me/sync-words?user_id=999", strings.NewReader(`{"user_id":999}`))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	a.Handler(t.TempDir()).ServeHTTP(rec, req)
	var result wordSyncStatus
	if rec.Code < 400 {
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, result
}

// addSyncSession 建立普通/管理员会话，任务目标必须来自该会话而非请求体。
func addSyncSession(users *fakeUserStore, sessions *fakeSessionStore, token string, admin bool) User {
	u := users.addUser(token, "hash", admin)
	sessions.usersByID[u.ID] = u
	sessions.byToken[token] = sessionRecord{userID: u.ID, expiresAt: time.Now().Add(time.Hour)}
	return u
}

// TestUserWordSyncAsyncAndIsolation 检查快速返回、请求取消独立、幂等复用和跨用户状态隔离。
func TestUserWordSyncAsyncAndIsolation(t *testing.T) {
	store := &syncWordsStore{started: make(chan int, 4), release: make(chan struct{})}
	a, users, sessions := wordSyncApp(t, store)
	alice := addSyncSession(users, sessions, "alice", false)
	bob := addSyncSession(users, sessions, "bob", true)
	if code, status := wordSyncRequest(t, a, http.MethodGet, "alice"); code != 200 || status.Status != "idle" {
		t.Fatal("expected idle")
	}

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/me/sync-words", strings.NewReader(`{"user_id":999}`)).WithContext(requestCtx)
	req.Header.Set("Authorization", "Bearer alice")
	rec := httptest.NewRecorder()
	a.Handler(t.TempDir()).ServeHTTP(rec, req)
	cancelRequest()
	var original wordSyncStatus
	if rec.Code != 202 || json.Unmarshal(rec.Body.Bytes(), &original) != nil || original.Status != "running" || original.TaskID == "" {
		t.Fatalf("not async: %s", rec.Body.String())
	}
	select {
	case id := <-store.started:
		if id != alice.ID {
			t.Fatalf("target=%d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("task not started")
	}
	for range 5 {
		code, reused := wordSyncRequest(t, a, http.MethodPost, "alice")
		if code != 200 || reused.TaskID != original.TaskID {
			t.Fatal("running task not reused")
		}
	}
	if _, status := wordSyncRequest(t, a, http.MethodGet, "alice"); status.Status != "running" || status.TaskID != original.TaskID {
		t.Fatal("refresh/request cancel lost task")
	}
	if _, status := wordSyncRequest(t, a, http.MethodGet, "bob"); status.Status != "idle" {
		t.Fatal("leaked alice task")
	}
	code, second := wordSyncRequest(t, a, http.MethodPost, "bob")
	if code != 202 || second.TaskID == original.TaskID {
		t.Fatal("user tasks shared")
	}
	select {
	case id := <-store.started:
		if id != bob.ID {
			t.Fatalf("target=%d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("second user task not started")
	}
	close(store.release)
	if !a.WaitBackground(time.Second) {
		t.Fatal("sync did not finish")
	}
	for _, token := range []string{"alice", "bob"} {
		_, status := wordSyncRequest(t, a, http.MethodGet, token)
		if status.Status != "completed" || status.AddedCount != 3 || status.WordCount != 8 {
			t.Fatalf("bad result %+v", status)
		}
	}
}

// TestUserWordSyncAuthAndRetiredAdminRoute 确认未登录/禁用会话拒绝，旧代用户接口没有写入能力。
func TestUserWordSyncAuthAndRetiredAdminRoute(t *testing.T) {
	store := &syncWordsStore{started: make(chan int, 4), release: make(chan struct{})}
	a, users, sessions := wordSyncApp(t, store)
	admin := addSyncSession(users, sessions, "admin", true)
	disabled := addSyncSession(users, sessions, "disabled", false)
	disabled.Disabled = true
	sessions.usersByID[disabled.ID] = disabled
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		if code, _ := wordSyncRequest(t, a, method, ""); code != 401 {
			t.Fatalf("unauth=%d", code)
		}
		// 首次禁用检查会撤销会话；每个方法单独建立会话再验证拒绝行为。
		sessions.byToken["disabled"] = sessionRecord{userID: disabled.ID, expiresAt: time.Now().Add(time.Hour)}
		if code, _ := wordSyncRequest(t, a, method, "disabled"); code != 401 {
			t.Fatalf("disabled=%d", code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users/999/sync-words", nil)
	req.Header.Set("Authorization", "Bearer admin")
	rec := httptest.NewRecorder()
	a.Handler(t.TempDir()).ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("old endpoint=%d", rec.Code)
	}
	if _, status := wordSyncRequest(t, a, http.MethodGet, "admin"); status.Status != "idle" || admin.ID == 0 {
		t.Fatal("old route started task")
	}
}

// TestUserWordSyncFailureAndShutdown 覆盖存储错误、panic 和关闭取消，结果不能误报成功。
func TestUserWordSyncFailureAndShutdown(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		err                   error
		panicResult, shutdown bool
	}{
		{name: "storage error", err: errors.New("private database error")},
		{name: "panic", panicResult: true},
		{name: "shutdown", shutdown: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &syncWordsStore{started: make(chan int, 4), release: make(chan struct{}), err: tc.err, panicResult: tc.panicResult}
			a, users, sessions := wordSyncApp(t, store)
			addSyncSession(users, sessions, "alice", false)
			if code, _ := wordSyncRequest(t, a, http.MethodPost, "alice"); code != 202 {
				t.Fatal("start failed")
			}
			select {
			case <-store.started:
			case <-time.After(time.Second):
				t.Fatal("not started")
			}
			if tc.shutdown {
				a.CancelBackground()
			} else {
				close(store.release)
			}
			if !a.WaitBackground(time.Second) {
				t.Fatal("task stuck")
			}
			_, status := wordSyncRequest(t, a, http.MethodGet, "alice")
			if status.Status != "failed" || status.Message == "" || status.AddedCount != 0 || status.WordCount != 0 || strings.Contains(status.Message, "private") {
				t.Fatalf("bad failure %+v", status)
			}
			if tc.shutdown {
				if code, _ := wordSyncRequest(t, a, http.MethodPost, "alice"); code != 503 {
					t.Fatal("started during shutdown")
				}
			} else {
				if code, next := wordSyncRequest(t, a, http.MethodPost, "alice"); code != 202 || next.TaskID == status.TaskID {
					t.Fatal("failed task cannot retry")
				}
				if !a.WaitBackground(time.Second) {
					t.Fatal("retry stuck")
				}
			}
		})
	}
}

package app

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"vocab-backend/internal/model"
	"vocab-backend/internal/training"
)

type emptyDailySource struct{ WordStore }

// ListPage 返回空词库，检验空状态不会调用模型。
func (e *emptyDailySource) ListPage(context.Context, int, bool, string, string, string, int, int) ([]model.Word, error) {
	return []model.Word{}, nil
}

// FindByIDs 保持空词库的只读接口。
func (e *emptyDailySource) FindByIDs(context.Context, int, []int) ([]model.Word, error) {
	return []model.Word{}, nil
}

// dailyHTTPApp 使用真实路由与登录中间件组装当日内存练习。
func dailyHTTPApp(t *testing.T) *App {
	t.Helper()
	a, _, sessions := newTestApp()
	a.daily = training.NewDailyManager(context.Background(), &emptyDailySource{}, func(context.Context, model.Word) (model.TrainingBundle, error) {
		t.Error("empty bank called model")
		return model.TrainingBundle{}, nil
	}, time.Now)
	t.Cleanup(a.daily.Close)
	sessions.usersByID[42] = User{ID: 42, Username: "daily-user"}
	sessions.byToken["daily-token"] = sessionRecord{userID: 42, expiresAt: time.Now().Add(time.Hour)}
	return a
}

// TestDailyHTTPEmptyAndPrivateFields 验证自动请求空词库与客户端不能自报正确答案。
func TestDailyHTTPEmptyAndPrivateFields(t *testing.T) {
	a := dailyHTTPApp(t)
	request := httptest.NewRequest("POST", "/api/training/daily", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer daily-token")
	w := httptest.NewRecorder()
	a.Handler(t.TempDir()).ServeHTTP(w, request)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"empty"`) {
		t.Fatalf("empty: %d %s", w.Code, w.Body.String())
	}
	request = httptest.NewRequest("POST", "/api/training/daily/answer", strings.NewReader(`{"group_id":"one","word_id":1,"option_id":"o1","correct":true}`))
	request.Header.Set("Authorization", "Bearer daily-token")
	w = httptest.NewRecorder()
	a.Handler(t.TempDir()).ServeHTTP(w, request)
	if w.Code != 400 {
		t.Fatal("client grade accepted")
	}
}

// TestDailyRoutesRequireAuthentication 验证精简后的接口仍要求本人登录。
func TestDailyRoutesRequireAuthentication(t *testing.T) {
	a, _, _ := newTestApp()
	for _, path := range []string{"/api/training/daily", "/api/training/daily/more", "/api/training/daily/answer", "/api/training/daily/advance", "/api/training/daily/retry"} {
		w := httptest.NewRecorder()
		a.Handler(t.TempDir()).ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		if w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
}

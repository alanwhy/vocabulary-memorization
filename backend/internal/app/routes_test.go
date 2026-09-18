package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHandlerRegistersExistingAPIRoutes 固定原有 32 条 method+path 契约；除登录、退出外，
// 未带凭证的请求都应先被鉴权层拒绝，而不能误落入 SPA fallback。
func TestHandlerRegistersExistingAPIRoutes(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodPost, "/api/login", http.StatusBadRequest},
		{http.MethodPost, "/api/logout", http.StatusNoContent},
		{http.MethodGet, "/api/me", http.StatusUnauthorized},
		{http.MethodPut, "/api/me/password", http.StatusUnauthorized},
		{http.MethodPost, "/api/words", http.StatusUnauthorized},
		{http.MethodGet, "/api/words", http.StatusUnauthorized},
		{http.MethodPost, "/api/words/reset-counts", http.StatusUnauthorized},
		{http.MethodGet, "/api/words/translating", http.StatusUnauthorized},
		{http.MethodGet, "/api/words/lookup", http.StatusUnauthorized},
		{http.MethodGet, "/api/vocabulary", http.StatusUnauthorized},
		{http.MethodGet, "/api/review-colors", http.StatusUnauthorized},
		{http.MethodGet, "/api/stats", http.StatusUnauthorized},
		{http.MethodDelete, "/api/words/1", http.StatusUnauthorized},
		{http.MethodPost, "/api/words/1/archive", http.StatusUnauthorized},
		{http.MethodPost, "/api/words/1/unarchive", http.StatusUnauthorized},
		{http.MethodPost, "/api/words/1/retry", http.StatusUnauthorized},
		{http.MethodPut, "/api/words/1/important", http.StatusUnauthorized},
		{http.MethodGet, "/api/flashcards/queue", http.StatusUnauthorized},
		{http.MethodPost, "/api/flashcards/review", http.StatusUnauthorized},
		{http.MethodGet, "/api/pronounce/example", http.StatusUnauthorized},
		{http.MethodPost, "/api/admin/users", http.StatusUnauthorized},
		{http.MethodGet, "/api/admin/users", http.StatusUnauthorized},
		{http.MethodPost, "/api/admin/users/1/reset-password", http.StatusUnauthorized},
		{http.MethodPost, "/api/admin/users/1/disable", http.StatusUnauthorized},
		{http.MethodDelete, "/api/admin/users/1", http.StatusUnauthorized},
		{http.MethodGet, "/api/admin/settings", http.StatusUnauthorized},
		{http.MethodPut, "/api/admin/settings", http.StatusUnauthorized},
		{http.MethodGet, "/api/admin/dictionary", http.StatusUnauthorized},
		{http.MethodGet, "/api/admin/dictionary/export", http.StatusUnauthorized},
		{http.MethodDelete, "/api/admin/dictionary/example", http.StatusUnauthorized},
		{http.MethodPost, "/api/admin/dictionary/batch-delete", http.StatusUnauthorized},
		{http.MethodPost, "/api/admin/dictionary/retry", http.StatusUnauthorized},
	}

	application, _, _ := newTestApp()
	handler := application.Handler(t.TempDir())
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%q", recorder.Code, tt.want, recorder.Body.String())
			}
			if tt.want == http.StatusUnauthorized && recorder.Body.String() != "{\"error\":\"请先登录\"}\n" {
				t.Fatalf("unauthorized body = %q", recorder.Body.String())
			}
		})
	}
}

// TestHandlerServesAuthenticatedMeThroughFullStack 覆盖完整 HTTP 组装链路：
// Handler 路由必须先用 fake session 完成鉴权与续期，再由 /api/me 返回当前用户。
func TestHandlerServesAuthenticatedMeThroughFullStack(t *testing.T) {
	application, _, sessions := newTestApp()
	user := User{
		ID:        42,
		Username:  "route-user",
		IsAdmin:   true,
		CreatedAt: time.Date(2026, time.September, 18, 8, 0, 0, 0, time.UTC),
	}
	const token = "valid-route-session"
	originalExpiry := time.Now().Add(time.Hour)
	sessions.usersByID[user.ID] = user
	sessions.byToken[token] = sessionRecord{userID: user.ID, expiresAt: originalExpiry}

	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	application.Handler(t.TempDir()).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	expectedBody, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("marshal expected user: %v", err)
	}
	if got, want := recorder.Body.String(), string(expectedBody)+"\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if refreshedExpiry := sessions.byToken[token].expiresAt; !refreshedExpiry.After(originalExpiry) {
		t.Fatalf("session expiry = %v, want after %v", refreshedExpiry, originalExpiry)
	}
}

func TestHandlerFallsBackToSPAIndex(t *testing.T) {
	staticDir := t.TempDir()
	const index = "<!doctype html><title>vocabulary spa</title>"
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte(index), 0o600); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	application, _, _ := newTestApp()
	recorder := httptest.NewRecorder()
	application.Handler(staticDir).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/profile", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "vocabulary spa") {
		t.Fatalf("SPA fallback body = %q", recorder.Body.String())
	}
}

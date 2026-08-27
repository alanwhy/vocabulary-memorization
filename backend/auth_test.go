package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestGenerateRandomPasswordLengthAndCharset(t *testing.T) {
	pw := generateRandomPassword(10)
	if len(pw) != 10 {
		t.Fatalf("expected length 10, got %d", len(pw))
	}
	for _, c := range pw {
		if !strings.ContainsRune(passwordCharset, c) {
			t.Fatalf("unexpected character %q in generated password %q", c, pw)
		}
	}
}

func TestRandomTokenIsHexAndUnique(t *testing.T) {
	a := randomToken()
	b := randomToken()
	if len(a) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(a))
	}
	if a == b {
		t.Fatalf("expected two random tokens to differ")
	}
}

func TestBearerToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", ""},
		{"Bearer abc123", "abc123"},
		{"bearer abc123", ""}, // 前缀大小写必须精确
		{"Bearer ", ""},
		{"Basic abc123", ""},
		{"Bearer  abc  ", "abc"}, // token 前后空白修剪
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/api/me", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		if got := bearerToken(req); got != c.want {
			t.Fatalf("bearerToken(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}

func TestHandleLoginSuccess(t *testing.T) {
	app, users, sessions := newTestApp()
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	u := users.addUser("alice", string(hash), false)
	sessions.usersByID[u.ID] = u

	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"alice","password":"secret123"}`))
	req.RemoteAddr = "1.2.3.4:5555"
	rec := httptest.NewRecorder()

	app.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(sessions.byToken) != 1 {
		t.Fatalf("expected exactly one session to be created, got %d", len(sessions.byToken))
	}
	if setCookie := rec.Header().Get("Set-Cookie"); setCookie != "" {
		t.Fatalf("expected no Set-Cookie header after switching to Bearer, got %q", setCookie)
	}
	if !strings.Contains(rec.Body.String(), `"token":"`) {
		t.Fatalf("expected login response to carry a token, got %s", rec.Body.String())
	}
}

func TestHandleLoginRecordsLastLogin(t *testing.T) {
	app, users, sessions := newTestApp()
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	u := users.addUser("alice", string(hash), false)
	sessions.usersByID[u.ID] = u

	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"alice","password":"secret123"}`))
	req.RemoteAddr = "1.2.3.4:5555"
	rec := httptest.NewRecorder()

	app.handleLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, ok := users.logins[u.ID]; !ok {
		t.Fatalf("expected last login time to be recorded for user %d, got %v", u.ID, users.logins)
	}
	// 响应体要立刻带上本次登录时间，前端不用再多请求一次 /api/me
	if !strings.Contains(rec.Body.String(), `"last_login_at":"`) {
		t.Fatalf("expected login response to carry last_login_at, got %s", rec.Body.String())
	}
}

func TestHandleLoginFailureDoesNotRecordLastLogin(t *testing.T) {
	app, users, _ := newTestApp()
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	u := users.addUser("alice", string(hash), false)

	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"alice","password":"wrong"}`))
	req.RemoteAddr = "1.2.3.4:5555"
	rec := httptest.NewRecorder()

	app.handleLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if _, ok := users.logins[u.ID]; ok {
		t.Fatalf("expected no last login record for a failed login")
	}
}

func TestHandleLoginLockoutAfterThreshold(t *testing.T) {
	app, users, _ := newTestApp()
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	users.addUser("bob", string(hash), false)

	doAttempt := func() int {
		req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"bob","password":"wrong"}`))
		req.RemoteAddr = "9.9.9.9:1111"
		rec := httptest.NewRecorder()
		app.handleLogin(rec, req)
		return rec.Code
	}

	for i := 0; i < 5; i++ {
		if code := doAttempt(); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, code)
		}
	}
	if code := doAttempt(); code != http.StatusTooManyRequests {
		t.Fatalf("6th attempt: expected 429, got %d", code)
	}
}

func TestHandleChangePasswordClearsOtherSessions(t *testing.T) {
	app, users, sessions := newTestApp()
	hash, _ := bcrypt.GenerateFromPassword([]byte("old12345"), bcrypt.DefaultCost)
	u := users.addUser("carol", string(hash), false)
	sessions.usersByID[u.ID] = u
	sessions.byToken["tokenA"] = sessionRecord{userID: u.ID, expiresAt: time.Now().Add(time.Hour)}
	sessions.byToken["tokenB"] = sessionRecord{userID: u.ID, expiresAt: time.Now().Add(time.Hour)}

	req := httptest.NewRequest("PUT", "/api/me/password", strings.NewReader(`{"old_password":"old12345","new_password":"newpass456"}`))
	req.Header.Set("Authorization", "Bearer tokenA")
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, &u))
	rec := httptest.NewRecorder()

	app.handleChangePassword(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, ok := sessions.byToken["tokenB"]; ok {
		t.Fatalf("expected tokenB session to be cleared")
	}
	if _, ok := sessions.byToken["tokenA"]; !ok {
		t.Fatalf("expected tokenA (current) session to survive")
	}
}

func TestHandleChangePasswordWrongOldPassword(t *testing.T) {
	app, users, _ := newTestApp()
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct1"), bcrypt.DefaultCost)
	u := users.addUser("dave", string(hash), false)

	req := httptest.NewRequest("PUT", "/api/me/password", strings.NewReader(`{"old_password":"wrong","new_password":"newpass456"}`))
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, &u))
	rec := httptest.NewRecorder()

	app.handleChangePassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResetUserPasswordNotFound(t *testing.T) {
	app, _, _ := newTestApp()

	req := httptest.NewRequest("POST", "/api/admin/users/999/reset-password", nil)
	req.SetPathValue("id", "999")
	rec := httptest.NewRecorder()

	app.handleResetUserPassword(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResetUserPasswordSuccessClearsSessions(t *testing.T) {
	app, users, sessions := newTestApp()
	hash, _ := bcrypt.GenerateFromPassword([]byte("old"), bcrypt.DefaultCost)
	u := users.addUser("eve", string(hash), false)
	sessions.byToken["tok"] = sessionRecord{userID: u.ID, expiresAt: time.Now().Add(time.Hour)}

	req := httptest.NewRequest("POST", "/api/admin/users/"+strconv.Itoa(u.ID)+"/reset-password", nil)
	req.SetPathValue("id", strconv.Itoa(u.ID))
	rec := httptest.NewRecorder()

	app.handleResetUserPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(sessions.byToken) != 0 {
		t.Fatalf("expected sessions to be cleared after reset, got %d remaining", len(sessions.byToken))
	}
}

func TestRequireAuthNoHeader(t *testing.T) {
	app, _, _ := newTestApp()
	handler := app.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next should not be called")
	})
	req := httptest.NewRequest("GET", "/api/me", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuthExpiredSession(t *testing.T) {
	app, users, sessions := newTestApp()
	u := users.addUser("frank", "hash", false)
	sessions.usersByID[u.ID] = u
	sessions.byToken["expired"] = sessionRecord{userID: u.ID, expiresAt: time.Now().Add(-time.Hour)}

	handler := app.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next should not be called")
	})
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer expired")
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuthValidSessionPassesThrough(t *testing.T) {
	app, users, sessions := newTestApp()
	u := users.addUser("grace", "hash", false)
	sessions.usersByID[u.ID] = u
	sessions.byToken["valid"] = sessionRecord{userID: u.ID, expiresAt: time.Now().Add(time.Hour)}

	called := false
	handler := app.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if currentUser(r).Username != "grace" {
			t.Fatalf("expected current user to be grace, got %q", currentUser(r).Username)
		}
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	handler(rec, req)
	if !called {
		t.Fatalf("expected next handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequireAdminForbidsNonAdmin(t *testing.T) {
	app, users, sessions := newTestApp()
	u := users.addUser("henry", "hash", false)
	sessions.usersByID[u.ID] = u
	sessions.byToken["valid"] = sessionRecord{userID: u.ID, expiresAt: time.Now().Add(time.Hour)}

	handler := app.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next should not be called for non-admin")
	})
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRequireAdminAllowsAdmin(t *testing.T) {
	app, users, sessions := newTestApp()
	u := users.addUser("iris", "hash", true)
	sessions.usersByID[u.ID] = u
	sessions.byToken["valid"] = sessionRecord{userID: u.ID, expiresAt: time.Now().Add(time.Hour)}

	called := false
	handler := app.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	handler(rec, req)
	if !called {
		t.Fatalf("expected next handler to be called for admin")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleLoginRejectsDisabledUser(t *testing.T) {
	app, users, sessions := newTestApp()
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	u := users.addUser("alice", string(hash), false)
	u.Disabled = true
	users.usersByName["alice"] = u
	sessions.usersByID[u.ID] = u

	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"alice","password":"secret123"}`))
	req.RemoteAddr = "1.2.3.4:5555"
	rec := httptest.NewRecorder()

	app.handleLogin(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(sessions.byToken) != 0 {
		t.Fatalf("禁用用户登录不应创建会话")
	}
}

func TestRequireAuthRejectsDisabledSession(t *testing.T) {
	app, users, sessions := newTestApp()
	u := users.addUser("grace", "hash", false)
	u.Disabled = true
	sessions.usersByID[u.ID] = u
	sessions.byToken["valid"] = sessionRecord{userID: u.ID, expiresAt: time.Now().Add(time.Hour)}

	handler := app.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next should not be called for disabled user")
	})
	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer valid")
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if len(sessions.byToken) != 0 {
		t.Fatalf("禁用用户的会话应被清除")
	}
}

func TestHandleDisableUserCannotDisableSelf(t *testing.T) {
	app, users, _ := newTestApp()
	u := users.addUser("admin", "hash", true)

	req := httptest.NewRequest("POST", "/api/admin/users/"+strconv.Itoa(u.ID)+"/disable", strings.NewReader(`{"disabled":true}`))
	req.SetPathValue("id", strconv.Itoa(u.ID))
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, &u))
	rec := httptest.NewRecorder()

	app.handleDisableUser(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteUserCannotDeleteSelf(t *testing.T) {
	app, users, _ := newTestApp()
	u := users.addUser("admin", "hash", true)

	req := httptest.NewRequest("DELETE", "/api/admin/users/"+strconv.Itoa(u.ID), nil)
	req.SetPathValue("id", strconv.Itoa(u.ID))
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, &u))
	rec := httptest.NewRecorder()

	app.handleDeleteUser(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

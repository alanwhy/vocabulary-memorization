package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRecoverMiddlewareRecoversPanic(t *testing.T) {
	handler := recoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestWithTimeoutSetsDeadlineOnContext(t *testing.T) {
	var hasDeadline bool
	h := withTimeout(50 * time.Millisecond)(func(w http.ResponseWriter, r *http.Request) {
		_, hasDeadline = r.Context().Deadline()
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	h(rec, req)

	if !hasDeadline {
		t.Fatalf("expected request context to carry a deadline")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

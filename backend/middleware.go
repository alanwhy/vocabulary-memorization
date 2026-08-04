package main

import (
	"context"
	"log"
	"net/http"
	"time"
)

// withTimeout 给请求的 context 设置一个默认超时，避免下游卡住的调用（数据库、第三方接口）
// 无限期占用连接；handler 内部通过 r.Context() 读到的就是这个带超时的 context。
func withTimeout(d time.Duration) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next(w, r.WithContext(ctx))
		}
	}
}

// recoverMiddleware 包在整个 mux 外层，任何 handler 里的 panic 都会被这里兜住返回 500，
// 而不是让单个请求的 panic 打断整个进程（Go 的 http.Server 默认只会关闭当前连接，
// 但业务上仍希望有明确的错误响应和日志）。
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("请求处理时发生 panic %s %s: %v", r.Method, r.URL.Path, rec)
				writeError(w, http.StatusInternalServerError, "服务器内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

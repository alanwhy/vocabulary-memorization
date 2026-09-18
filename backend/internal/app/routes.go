// 路由只在 Handler 中组装：调用方拿到的 handler 已包含全部 API、SPA fallback 和一层 panic 恢复。
// 每条 API 的 timeout 保持在鉴权外层，确保鉴权查询也受请求超时约束。
package app

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultRequestTimeout  = 10 * time.Second
	exportRequestTimeout   = 60 * time.Second
	dictionaryRetryTimeout = 60 * time.Second
)

// Handler 返回应用完整的 HTTP 入口。staticDir 由组合根传入，以保持生产工作目录语义并方便测试隔离。
func (a *App) Handler(staticDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", withTimeout(defaultRequestTimeout)(a.handleLogin))
	mux.HandleFunc("POST /api/logout", withTimeout(defaultRequestTimeout)(a.handleLogout))
	mux.HandleFunc("GET /api/me", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleMe)))
	mux.HandleFunc("PUT /api/me/password", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleChangePassword)))

	mux.HandleFunc("POST /api/words", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleAddWord)))
	mux.HandleFunc("GET /api/words", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleListWords)))
	mux.HandleFunc("POST /api/words/reset-counts", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleResetReviewCounts)))
	mux.HandleFunc("GET /api/words/translating", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleListTranslatingWords)))
	mux.HandleFunc("GET /api/words/lookup", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleLookupWord)))
	mux.HandleFunc("GET /api/vocabulary", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleVocabularyIndex)))
	mux.HandleFunc("GET /api/review-colors", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleReviewColors)))
	mux.HandleFunc("GET /api/stats", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleWordStats)))
	mux.HandleFunc("DELETE /api/words/{id}", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleDeleteWord)))
	mux.HandleFunc("POST /api/words/{id}/archive", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleArchiveWord)))
	mux.HandleFunc("POST /api/words/{id}/unarchive", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleUnarchiveWord)))
	mux.HandleFunc("POST /api/words/{id}/retry", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleRetryWord)))
	mux.HandleFunc("PUT /api/words/{id}/important", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleSetWordImportant)))
	mux.HandleFunc("GET /api/flashcards/queue", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleFlashcardQueue)))
	mux.HandleFunc("POST /api/flashcards/review", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handleFlashcardReview)))
	mux.HandleFunc("GET /api/pronounce/{wordKey}", withTimeout(defaultRequestTimeout)(a.requireAuth(a.handlePronounce)))

	mux.HandleFunc("POST /api/admin/users", withTimeout(defaultRequestTimeout)(a.requireAdmin(a.handleCreateUser)))
	mux.HandleFunc("GET /api/admin/users", withTimeout(defaultRequestTimeout)(a.requireAdmin(a.handleListUsers)))
	mux.HandleFunc("POST /api/admin/users/{id}/reset-password", withTimeout(defaultRequestTimeout)(a.requireAdmin(a.handleResetUserPassword)))
	mux.HandleFunc("POST /api/admin/users/{id}/disable", withTimeout(defaultRequestTimeout)(a.requireAdmin(a.handleDisableUser)))
	mux.HandleFunc("DELETE /api/admin/users/{id}", withTimeout(defaultRequestTimeout)(a.requireAdmin(a.handleDeleteUser)))
	mux.HandleFunc("GET /api/admin/settings", withTimeout(defaultRequestTimeout)(a.requireAdmin(a.handleGetSettings)))
	mux.HandleFunc("PUT /api/admin/settings", withTimeout(defaultRequestTimeout)(a.requireAdmin(a.handleUpdateSettings)))
	mux.HandleFunc("GET /api/admin/dictionary", withTimeout(defaultRequestTimeout)(a.requireAdmin(a.handleListDictionary)))
	mux.HandleFunc("GET /api/admin/dictionary/export", withTimeout(exportRequestTimeout)(a.requireAdmin(a.handleExportDictionary)))
	mux.HandleFunc("DELETE /api/admin/dictionary/{word_key}", withTimeout(defaultRequestTimeout)(a.requireAdmin(a.handleDeleteDictionaryEntry)))
	mux.HandleFunc("POST /api/admin/dictionary/batch-delete", withTimeout(defaultRequestTimeout)(a.requireAdmin(a.handleDeleteDictionaryBatch)))
	mux.HandleFunc("POST /api/admin/dictionary/retry", withTimeout(dictionaryRetryTimeout)(a.requireAdmin(a.handleRetryDictionary)))

	mux.HandleFunc("/", spaHandler(staticDir))
	return recoverMiddleware(mux)
}

// spaHandler 让 Vue Router history 模式的深链接在文件不存在时回退到 index.html。
func spaHandler(staticDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
	}
}

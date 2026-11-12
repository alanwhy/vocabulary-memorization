// 用户管理的母库同步入口：管理员指定目标用户，由仓储原子补齐缺词并返回数量。
package app

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"
)

// handleSyncUserWords 将请求路径中的用户 id 交给仓储同步母库。
// w 返回新增数量和个人词库总数，r 必须经过管理员鉴权；非法 id、用户缺失和存储失败返回明确错误。
func (a *App) handleSyncUserWords(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "无效的 id")
		return
	}
	added, total, err := a.words.SyncDictionaryToUser(r.Context(), id, time.Now())
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		log.Printf("同步用户单词失败 user_id=%d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "同步失败，请重试")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added_count": added, "word_count": total})
}

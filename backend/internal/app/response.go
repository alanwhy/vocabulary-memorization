package app

import (
	"encoding/json"
	"log"
	"net/http"
)

// pageResult 是分页接口共用的 HTTP 响应信封，不属于领域模型。
type pageResult struct {
	Items interface{} `json:"items"`
	Total int         `json:"total"`
	// TotalAll 不受筛选影响的整表总数（仅词库管理接口填充），用于顶部展示全库单词数。
	TotalAll int  `json:"total_all,omitempty"`
	Page     int  `json:"page"`
	Limit    int  `json:"limit"`
	HasMore  bool `json:"has_more"`
}

func newPageResult(items interface{}, total, page, limit int) pageResult {
	return pageResult{
		Items:   items,
		Total:   total,
		Page:    page,
		Limit:   limit,
		HasMore: page*limit < total,
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("写入响应失败: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

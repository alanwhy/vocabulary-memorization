package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
)

// deepseekConfig DeepSeek 查词的配置，可在后台动态修改
type deepseekConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	Enabled bool   `json:"enabled"`
}

var (
	settingsMu sync.RWMutex
	dsConfig   deepseekConfig
)

const (
	settingKeyAPIKey  = "deepseek_api_key"
	settingKeyBaseURL = "deepseek_base_url"
	settingKeyModel   = "deepseek_model"
	settingKeyEnabled = "deepseek_enabled"
)

// loadSettings 启动时调用：如果 settings 表里还没有 DeepSeek 配置，用环境变量（或内置默认值）种一份进去，
// 之后一律以数据库里的值为准，读到内存缓存里
func loadSettings() {
	seedSettingIfMissing(settingKeyAPIKey, getEnv("DEEPSEEK_API_KEY", "REDACTED"))
	seedSettingIfMissing(settingKeyBaseURL, getEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com"))
	seedSettingIfMissing(settingKeyModel, getEnv("DEEPSEEK_MODEL", "deepseek-v4-flash"))
	seedSettingIfMissing(settingKeyEnabled, "true")
	refreshSettingsCache()
}

func seedSettingIfMissing(name, value string) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE name = ?`, name).Scan(&count); err != nil {
		log.Fatalf("查询配置失败 name=%s: %v", name, err)
	}
	if count > 0 {
		return
	}
	if _, err := db.Exec(`INSERT INTO settings (name, value) VALUES (?, ?)`, name, value); err != nil {
		log.Fatalf("初始化配置失败 name=%s: %v", name, err)
	}
}

func refreshSettingsCache() {
	values := map[string]string{}
	rows, err := db.Query(`SELECT name, value FROM settings WHERE name IN (?, ?, ?, ?)`,
		settingKeyAPIKey, settingKeyBaseURL, settingKeyModel, settingKeyEnabled)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			log.Fatalf("读取配置失败: %v", err)
		}
		values[name] = value
	}

	settingsMu.Lock()
	dsConfig = deepseekConfig{
		APIKey:  values[settingKeyAPIKey],
		BaseURL: strings.TrimRight(values[settingKeyBaseURL], "/"),
		Model:   values[settingKeyModel],
		Enabled: values[settingKeyEnabled] == "true",
	}
	settingsMu.Unlock()
}

func getDeepSeekConfig() deepseekConfig {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return dsConfig
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := getDeepSeekConfig()
	cfg.APIKey = maskAPIKey(cfg.APIKey)
	writeJSON(w, http.StatusOK, cfg)
}

type updateSettingsRequest struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	Enabled bool   `json:"enabled"`
}

func handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.Model = strings.TrimSpace(req.Model)
	if req.BaseURL == "" || req.Model == "" {
		writeError(w, http.StatusBadRequest, "base_url 和 model 不能为空")
		return
	}

	current := getDeepSeekConfig()
	apiKey := strings.TrimSpace(req.APIKey)
	// 前端展示的是打了掩码的 key，如果用户没有改动这一项就原样提交回来了，不要用掩码覆盖真实的 key
	if apiKey == "" || apiKey == maskAPIKey(current.APIKey) {
		apiKey = current.APIKey
	}

	enabledStr := "false"
	if req.Enabled {
		enabledStr = "true"
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("更新配置失败: %v", err)
		writeError(w, http.StatusInternalServerError, "保存失败")
		return
	}
	updates := map[string]string{
		settingKeyAPIKey:  apiKey,
		settingKeyBaseURL: req.BaseURL,
		settingKeyModel:   req.Model,
		settingKeyEnabled: enabledStr,
	}
	for name, value := range updates {
		if _, err := tx.Exec(`INSERT INTO settings (name, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = ?`, name, value, value); err != nil {
			tx.Rollback()
			log.Printf("更新配置失败 name=%s: %v", name, err)
			writeError(w, http.StatusInternalServerError, "保存失败")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("提交配置更新失败: %v", err)
		writeError(w, http.StatusInternalServerError, "保存失败")
		return
	}

	refreshSettingsCache()
	cfg := getDeepSeekConfig()
	cfg.APIKey = maskAPIKey(cfg.APIKey)
	writeJSON(w, http.StatusOK, cfg)
}

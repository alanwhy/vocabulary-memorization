package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// deepseekConfig DeepSeek 查词的配置，可在后台动态修改
type deepseekConfig struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	Enabled bool   `json:"enabled"`
}

const (
	settingKeyAPIKey  = "deepseek_api_key"
	settingKeyBaseURL = "deepseek_base_url"
	settingKeyModel   = "deepseek_model"
	settingKeyEnabled = "deepseek_enabled"
)

// loadSettings 启动时调用：如果 settings 表里还没有 DeepSeek 配置，用环境变量（或内置默认值）种一份进去，
// 之后一律以数据库里的值为准，读到内存缓存里
func (a *App) loadSettings() {
	ctx := context.Background()
	a.seedSettingIfMissing(ctx, settingKeyAPIKey, getEnv("DEEPSEEK_API_KEY", ""))
	a.seedSettingIfMissing(ctx, settingKeyBaseURL, getEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com"))
	a.seedSettingIfMissing(ctx, settingKeyModel, getEnv("DEEPSEEK_MODEL", "deepseek-v4-flash"))
	a.seedSettingIfMissing(ctx, settingKeyEnabled, "true")
	a.refreshSettingsCache(ctx)
}

func (a *App) seedSettingIfMissing(ctx context.Context, name, value string) {
	if err := a.settings.SeedIfMissing(ctx, name, value); err != nil {
		log.Fatalf("初始化配置失败 name=%s: %v", name, err)
	}
}

func (a *App) refreshSettingsCache(ctx context.Context) {
	values, err := a.settings.LoadValues(ctx, []string{settingKeyAPIKey, settingKeyBaseURL, settingKeyModel, settingKeyEnabled})
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	a.settingsMu.Lock()
	a.dsConfig = deepseekConfig{
		APIKey:  values[settingKeyAPIKey],
		BaseURL: strings.TrimRight(values[settingKeyBaseURL], "/"),
		Model:   values[settingKeyModel],
		Enabled: values[settingKeyEnabled] == "true",
	}
	a.settingsMu.Unlock()
}

func (a *App) getDeepSeekConfig() deepseekConfig {
	a.settingsMu.RLock()
	defer a.settingsMu.RUnlock()
	return a.dsConfig
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func (a *App) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := a.getDeepSeekConfig()
	cfg.APIKey = maskAPIKey(cfg.APIKey)
	writeJSON(w, http.StatusOK, cfg)
}

type updateSettingsRequest struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	Enabled bool   `json:"enabled"`
}

func (a *App) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
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

	current := a.getDeepSeekConfig()
	apiKey := strings.TrimSpace(req.APIKey)
	// 前端展示的是打了掩码的 key，如果用户没有改动这一项就原样提交回来了，不要用掩码覆盖真实的 key
	if apiKey == "" || apiKey == maskAPIKey(current.APIKey) {
		apiKey = current.APIKey
	}

	enabledStr := "false"
	if req.Enabled {
		enabledStr = "true"
	}

	updates := map[string]string{
		settingKeyAPIKey:  apiKey,
		settingKeyBaseURL: req.BaseURL,
		settingKeyModel:   req.Model,
		settingKeyEnabled: enabledStr,
	}
	if err := a.settings.UpsertMany(r.Context(), updates); err != nil {
		log.Printf("更新配置失败: %v", err)
		writeError(w, http.StatusInternalServerError, "保存失败")
		return
	}

	a.refreshSettingsCache(r.Context())
	cfg := a.getDeepSeekConfig()
	cfg.APIKey = maskAPIKey(cfg.APIKey)
	writeJSON(w, http.StatusOK, cfg)
}

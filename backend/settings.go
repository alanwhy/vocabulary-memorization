package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// deepseekConfig DeepSeek 查词的配置，可在后台动态修改。
// FallbackModel 是当前实际用于查词的模型；ThinkingModel 本期仅做配置存储，尚未接入查词链路。
type deepseekConfig struct {
	APIKey        string `json:"api_key"`
	BaseURL       string `json:"base_url"`
	FallbackModel string `json:"fallback_model"`
	ThinkingModel string `json:"thinking_model"`
	Enabled       bool   `json:"enabled"`
}

const (
	settingKeyAPIKey        = "deepseek_api_key"
	settingKeyBaseURL       = "deepseek_base_url"
	settingKeyFallbackModel = "deepseek_fallback_model"
	settingKeyThinkingModel = "deepseek_thinking_model"
	settingKeyEnabled       = "deepseek_enabled"

	// legacySettingKeyModel 旧版单一「模型」配置 key，仅用于启动时迁移到兜底模型，之后不再读写。
	legacySettingKeyModel = "deepseek_model"
)

// loadSettings 启动时调用：如果 settings 表里还没有 DeepSeek 配置，用环境变量（或内置默认值）种一份进去，
// 之后一律以数据库里的值为准，读到内存缓存里
func (a *App) loadSettings() {
	ctx := context.Background()
	a.seedSettingIfMissing(ctx, settingKeyAPIKey, getEnv("DEEPSEEK_API_KEY", ""))
	a.seedSettingIfMissing(ctx, settingKeyBaseURL, getEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com"))
	a.seedSettingIfMissing(ctx, settingKeyEnabled, "true")

	// 兜底模型播种优先级：旧 deepseek_model（迁移，避免覆盖已自定义的值）> DEEPSEEK_FALLBACK_MODEL > 内置默认值。
	fallbackDefault := getEnv("DEEPSEEK_FALLBACK_MODEL", "deepseek-v4-flash")
	if existing, err := a.settings.LoadValues(ctx, []string{legacySettingKeyModel, settingKeyFallbackModel}); err == nil {
		if existing[settingKeyFallbackModel] == "" && existing[legacySettingKeyModel] != "" {
			fallbackDefault = existing[legacySettingKeyModel]
		}
	}
	a.seedSettingIfMissing(ctx, settingKeyFallbackModel, fallbackDefault)
	a.seedSettingIfMissing(ctx, settingKeyThinkingModel, getEnv("DEEPSEEK_THINKING_MODEL", ""))
	a.refreshSettingsCache(ctx)
}

func (a *App) seedSettingIfMissing(ctx context.Context, name, value string) {
	if err := a.settings.SeedIfMissing(ctx, name, value); err != nil {
		log.Fatalf("初始化配置失败 name=%s: %v", name, err)
	}
}

func (a *App) refreshSettingsCache(ctx context.Context) {
	values, err := a.settings.LoadValues(ctx, []string{
		settingKeyAPIKey, settingKeyBaseURL, settingKeyFallbackModel, settingKeyThinkingModel, settingKeyEnabled,
	})
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	a.settingsMu.Lock()
	a.dsConfig = deepseekConfig{
		APIKey:        values[settingKeyAPIKey],
		BaseURL:       strings.TrimRight(values[settingKeyBaseURL], "/"),
		FallbackModel: values[settingKeyFallbackModel],
		ThinkingModel: values[settingKeyThinkingModel],
		Enabled:       values[settingKeyEnabled] == "true",
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
	APIKey        string `json:"api_key"`
	BaseURL       string `json:"base_url"`
	FallbackModel string `json:"fallback_model"`
	ThinkingModel string `json:"thinking_model"`
	Enabled       bool   `json:"enabled"`
}

func (a *App) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式不正确")
		return
	}
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	req.FallbackModel = strings.TrimSpace(req.FallbackModel)
	req.ThinkingModel = strings.TrimSpace(req.ThinkingModel)
	if req.BaseURL == "" || req.FallbackModel == "" {
		writeError(w, http.StatusBadRequest, "base_url 和兜底模型不能为空")
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
		settingKeyAPIKey:        apiKey,
		settingKeyBaseURL:       req.BaseURL,
		settingKeyFallbackModel: req.FallbackModel,
		settingKeyThinkingModel: req.ThinkingModel,
		settingKeyEnabled:       enabledStr,
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
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

// ttsConfig 豆包（火山引擎）语音合成的配置，可在后台动态修改。
// APIKey 是 BytePlus Seed Speech 的 API Key，放到请求的 X-Api-Key 头里（见 doubao.go）。
type ttsConfig struct {
	APIKey    string `json:"tts_api_key"`
	Cluster   string `json:"tts_cluster"`
	VoiceType string `json:"tts_voice_type"`
}

// reviewColorConfig 是管理员配置的次数渐变。它和 DeepSeek/TTS 配置共用 settings 表，
// 读取后缓存到 App，避免每次渲染颜色都访问数据库。
type reviewColorConfig struct {
	StartColor string `json:"start_color"`
	EndColor   string `json:"end_color"`
	Segments   int    `json:"segments"`
}

// complete 判断 TTS 配置是否齐全——三项都非空才认为已配置，缺任一项都视为「未配置」。
func (c ttsConfig) complete() bool {
	return c.APIKey != "" && c.Cluster != "" && c.VoiceType != ""
}

const (
	settingKeyAPIKey        = "deepseek_api_key"
	settingKeyBaseURL       = "deepseek_base_url"
	settingKeyFallbackModel = "deepseek_fallback_model"
	settingKeyThinkingModel = "deepseek_thinking_model"
	settingKeyEnabled       = "deepseek_enabled"

	settingKeyTTSAPIKey    = "tts_api_key"
	settingKeyTTSCluster   = "tts_cluster"
	settingKeyTTSVoiceType = "tts_voice_type"

	settingKeyReviewColorStart    = "review_color_start"
	settingKeyReviewColorEnd      = "review_color_end"
	settingKeyReviewColorSegments = "review_color_segments"

	// legacySettingKeyTTSAppID 旧版误把 API Key 存成了 tts_appid，启动时迁移到 tts_api_key，之后不再读写。
	legacySettingKeyTTSAppID = "tts_appid"

	// legacySettingKeyModel 旧版单一「模型」配置 key，仅用于启动时迁移到兜底模型，之后不再读写。
	legacySettingKeyModel = "deepseek_model"
)

const (
	defaultReviewColorStart = "#e4f7e9"
	defaultReviewColorEnd   = "#a11d1d"
	defaultReviewSegments   = 6
	minReviewSegments       = 2
	maxReviewSegments       = 12
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

	// 豆包语音合成配置：api_key 默认空（未配置），cluster/voice_type 给内置默认值。
	// 旧版误把 API Key 存成 tts_appid，这里优先迁移它，避免用户重填。
	apiKeyDefault := getEnv("TTS_API_KEY", "")
	if existing, err := a.settings.LoadValues(ctx, []string{legacySettingKeyTTSAppID, settingKeyTTSAPIKey}); err == nil {
		if existing[settingKeyTTSAPIKey] == "" && existing[legacySettingKeyTTSAppID] != "" {
			apiKeyDefault = existing[legacySettingKeyTTSAppID]
		}
	}
	a.seedSettingIfMissing(ctx, settingKeyTTSAPIKey, apiKeyDefault)
	a.seedSettingIfMissing(ctx, settingKeyTTSCluster, getEnv("TTS_CLUSTER", "volcano_tts"))
	a.seedSettingIfMissing(ctx, settingKeyTTSVoiceType, getEnv("TTS_VOICE_TYPE", "BV001"))
	a.seedSettingIfMissing(ctx, settingKeyReviewColorStart, defaultReviewColorStart)
	a.seedSettingIfMissing(ctx, settingKeyReviewColorEnd, defaultReviewColorEnd)
	a.seedSettingIfMissing(ctx, settingKeyReviewColorSegments, strconv.Itoa(defaultReviewSegments))
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
		settingKeyTTSAPIKey, settingKeyTTSCluster, settingKeyTTSVoiceType,
		settingKeyReviewColorStart, settingKeyReviewColorEnd, settingKeyReviewColorSegments,
	})
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	segments, err := strconv.Atoi(values[settingKeyReviewColorSegments])
	if err != nil {
		segments = 0
	}
	reviewColors, err := validatedReviewColorConfig(values[settingKeyReviewColorStart], values[settingKeyReviewColorEnd], segments)
	if err != nil {
		// 手工改库或旧部署的脏配置不能阻断服务启动；界面先安全回退到默认值，
		// 管理员下次保存会把三项配置一并修正。
		reviewColors = defaultReviewColorConfig()
	}

	a.settingsMu.Lock()
	a.dsConfig = deepseekConfig{
		APIKey:        values[settingKeyAPIKey],
		BaseURL:       strings.TrimRight(values[settingKeyBaseURL], "/"),
		FallbackModel: values[settingKeyFallbackModel],
		ThinkingModel: values[settingKeyThinkingModel],
		Enabled:       values[settingKeyEnabled] == "true",
	}
	a.ttsConfig = ttsConfig{
		APIKey:    values[settingKeyTTSAPIKey],
		Cluster:   values[settingKeyTTSCluster],
		VoiceType: values[settingKeyTTSVoiceType],
	}
	a.reviewColors = reviewColors
	a.settingsMu.Unlock()
}

func (a *App) getDeepSeekConfig() deepseekConfig {
	a.settingsMu.RLock()
	defer a.settingsMu.RUnlock()
	return a.dsConfig
}

func (a *App) getTTSConfig() ttsConfig {
	a.settingsMu.RLock()
	defer a.settingsMu.RUnlock()
	return a.ttsConfig
}

func (a *App) getReviewColorConfig() reviewColorConfig {
	a.settingsMu.RLock()
	defer a.settingsMu.RUnlock()
	return a.reviewColors
}

func defaultReviewColorConfig() reviewColorConfig {
	return reviewColorConfig{
		StartColor: defaultReviewColorStart,
		EndColor:   defaultReviewColorEnd,
		Segments:   defaultReviewSegments,
	}
}

// validatedReviewColorConfig 只接受完整的 #RRGGBB 和可展示的分段范围，
// 让 API 和启动期缓存使用同一套规则，避免前端收到无法插值的颜色。
func validatedReviewColorConfig(startColor, endColor string, segments int) (reviewColorConfig, error) {
	startColor = strings.ToLower(strings.TrimSpace(startColor))
	endColor = strings.ToLower(strings.TrimSpace(endColor))
	if !isHexColor(startColor) || !isHexColor(endColor) {
		return reviewColorConfig{}, fmt.Errorf("起始色和终止色必须是 #RRGGBB 格式")
	}
	if segments < minReviewSegments || segments > maxReviewSegments {
		return reviewColorConfig{}, fmt.Errorf("分段数必须在 %d 到 %d 之间", minReviewSegments, maxReviewSegments)
	}
	return reviewColorConfig{StartColor: startColor, EndColor: endColor, Segments: segments}, nil
}

func isHexColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	for _, c := range color[1:] {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') && !(c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// settingsView 后台管理页展示/保存的配置视图：DeepSeek 查词 + 豆包 TTS 平铺成一张表单。
type settingsView struct {
	Enabled       bool   `json:"enabled"`
	APIKey        string `json:"api_key"`
	BaseURL       string `json:"base_url"`
	FallbackModel string `json:"fallback_model"`
	ThinkingModel string `json:"thinking_model"`
	TTSAPIKey     string `json:"tts_api_key"`
	TTSCluster    string `json:"tts_cluster"`
	TTSVoiceType  string `json:"tts_voice_type"`
	StartColor    string `json:"start_color"`
	EndColor      string `json:"end_color"`
	Segments      int    `json:"segments"`
}

// maskedSettingsView 构造带掩码的配置视图（APIKey 打码），供 GET 和 PUT 两个接口复用。
func (a *App) maskedSettingsView() settingsView {
	ds := a.getDeepSeekConfig()
	ds.APIKey = maskAPIKey(ds.APIKey)
	tts := a.getTTSConfig()
	tts.APIKey = maskAPIKey(tts.APIKey)
	reviewColors := a.getReviewColorConfig()
	return settingsView{
		Enabled:       ds.Enabled,
		APIKey:        ds.APIKey,
		BaseURL:       ds.BaseURL,
		FallbackModel: ds.FallbackModel,
		ThinkingModel: ds.ThinkingModel,
		TTSAPIKey:     tts.APIKey,
		TTSCluster:    tts.Cluster,
		TTSVoiceType:  tts.VoiceType,
		StartColor:    reviewColors.StartColor,
		EndColor:      reviewColors.EndColor,
		Segments:      reviewColors.Segments,
	}
}

func (a *App) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.maskedSettingsView())
}

type updateSettingsRequest struct {
	APIKey        string `json:"api_key"`
	BaseURL       string `json:"base_url"`
	FallbackModel string `json:"fallback_model"`
	ThinkingModel string `json:"thinking_model"`
	Enabled       bool   `json:"enabled"`
	TTSAPIKey     string `json:"tts_api_key"`
	TTSCluster    string `json:"tts_cluster"`
	TTSVoiceType  string `json:"tts_voice_type"`
	StartColor    string `json:"start_color"`
	EndColor      string `json:"end_color"`
	Segments      int    `json:"segments"`
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
	req.TTSAPIKey = strings.TrimSpace(req.TTSAPIKey)
	req.TTSCluster = strings.TrimSpace(req.TTSCluster)
	req.TTSVoiceType = strings.TrimSpace(req.TTSVoiceType)
	reviewColors, err := validatedReviewColorConfig(req.StartColor, req.EndColor, req.Segments)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.BaseURL == "" || req.FallbackModel == "" {
		writeError(w, http.StatusBadRequest, "base_url 和兜底模型不能为空")
		return
	}

	currentDS := a.getDeepSeekConfig()
	apiKey := strings.TrimSpace(req.APIKey)
	// 前端展示的是打了掩码的 key，如果用户没有改动这一项就原样提交回来了，不要用掩码覆盖真实的 key
	if apiKey == "" || apiKey == maskAPIKey(currentDS.APIKey) {
		apiKey = currentDS.APIKey
	}

	currentTTS := a.getTTSConfig()
	ttsAPIKey := strings.TrimSpace(req.TTSAPIKey)
	if ttsAPIKey == "" || ttsAPIKey == maskAPIKey(currentTTS.APIKey) {
		ttsAPIKey = currentTTS.APIKey
	}

	enabledStr := "false"
	if req.Enabled {
		enabledStr = "true"
	}

	updates := map[string]string{
		settingKeyAPIKey:              apiKey,
		settingKeyBaseURL:             req.BaseURL,
		settingKeyFallbackModel:       req.FallbackModel,
		settingKeyThinkingModel:       req.ThinkingModel,
		settingKeyEnabled:             enabledStr,
		settingKeyTTSAPIKey:           ttsAPIKey,
		settingKeyTTSCluster:          req.TTSCluster,
		settingKeyTTSVoiceType:        req.TTSVoiceType,
		settingKeyReviewColorStart:    reviewColors.StartColor,
		settingKeyReviewColorEnd:      reviewColors.EndColor,
		settingKeyReviewColorSegments: strconv.Itoa(reviewColors.Segments),
	}
	if err := a.settings.UpsertMany(r.Context(), updates); err != nil {
		log.Printf("更新配置失败: %v", err)
		writeError(w, http.StatusInternalServerError, "保存失败")
		return
	}

	a.refreshSettingsCache(r.Context())
	writeJSON(w, http.StatusOK, a.maskedSettingsView())
}

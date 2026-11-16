package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"vocab-backend/internal/billing"
)

// deepseekConfig DeepSeek 查词的配置，可在后台动态修改。
// FallbackModel 是当前实际用于查词的模型；ThinkingModel 本期仅做配置存储，尚未接入查词链路。
type deepseekConfig struct {
	APIKey        string           `json:"api_key"`
	BaseURL       string           `json:"base_url"`
	FallbackModel string           `json:"fallback_model"`
	ThinkingModel string           `json:"thinking_model"`
	Enabled       bool             `json:"enabled"`
	Accounting    *billing.Service `json:"-"`
}

// ttsConfig uses the shared DeepSeek credentials and an independent voice choice.
type ttsConfig struct {
	APIKey  string
	BaseURL string
	VoiceID string
}

// reviewColorConfig 是管理员配置的次数渐变。它和 DeepSeek/TTS 配置共用 settings 表，
// 读取后缓存到 App，避免每次渲染颜色都访问数据库。
type reviewColorConfig struct {
	StartColor string `json:"start_color"`
	EndColor   string `json:"end_color"`
	Segments   int    `json:"segments"`
}

// complete validates credentials and the two supported voices; lookup's Enabled flag does not apply.
func (c ttsConfig) complete() bool {
	return c.APIKey != "" && c.BaseURL != "" && validTTSVoice(c.VoiceID)
}

// validTTSVoice constrains voice IDs to those supported by the official API.
func validTTSVoice(voice string) bool { return voice == "stella" || voice == "tide" }

const (
	settingKeyAPIKey        = "deepseek_api_key"
	settingKeyBaseURL       = "deepseek_base_url"
	settingKeyFallbackModel = "deepseek_fallback_model"
	settingKeyThinkingModel = "deepseek_thinking_model"
	settingKeyEnabled       = "deepseek_enabled"

	settingKeyTTSVoiceID = "deepseek_tts_voice_id"

	settingKeyReviewColorStart    = "review_color_start"
	settingKeyReviewColorEnd      = "review_color_end"
	settingKeyReviewColorSegments = "review_color_segments"

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

	voice := getEnv("DEEPSEEK_TTS_VOICE_ID", "stella")
	if !validTTSVoice(voice) {
		voice = "stella"
	}
	a.seedSettingIfMissing(ctx, settingKeyTTSVoiceID, voice)
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
		settingKeyTTSVoiceID,
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
	voice := values[settingKeyTTSVoiceID]
	if !validTTSVoice(voice) {
		voice = "stella"
	}
	a.ttsConfig = ttsConfig{
		APIKey:  values[settingKeyAPIKey],
		BaseURL: strings.TrimRight(values[settingKeyBaseURL], "/"),
		VoiceID: voice,
	}
	a.reviewColors = reviewColors
	a.settingsMu.Unlock()
}

func (a *App) getDeepSeekConfig() deepseekConfig {
	a.settingsMu.RLock()
	defer a.settingsMu.RUnlock()
	cfg := a.dsConfig
	cfg.Accounting = a.billing
	return cfg
}

// getTTSConfig returns a single shared-credential snapshot without chat billing settings.
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

// settingsView combines shared DeepSeek credentials, TTS voice and review colors; keys are masked.
type settingsView struct {
	Enabled       bool   `json:"enabled"`
	APIKey        string `json:"api_key"`
	BaseURL       string `json:"base_url"`
	FallbackModel string `json:"fallback_model"`
	ThinkingModel string `json:"thinking_model"`
	TTSVoiceID    string `json:"tts_voice_id"`
	StartColor    string `json:"start_color"`
	EndColor      string `json:"end_color"`
	Segments      int    `json:"segments"`
}

// maskedSettingsView 构造带掩码的配置视图（APIKey 打码），供 GET 和 PUT 两个接口复用。
func (a *App) maskedSettingsView() settingsView {
	ds := a.getDeepSeekConfig()
	ds.APIKey = maskAPIKey(ds.APIKey)
	tts := a.getTTSConfig()
	reviewColors := a.getReviewColorConfig()
	return settingsView{
		Enabled:       ds.Enabled,
		APIKey:        ds.APIKey,
		BaseURL:       ds.BaseURL,
		FallbackModel: ds.FallbackModel,
		ThinkingModel: ds.ThinkingModel,
		TTSVoiceID:    tts.VoiceID,
		StartColor:    reviewColors.StartColor,
		EndColor:      reviewColors.EndColor,
		Segments:      reviewColors.Segments,
	}
}

func (a *App) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.maskedSettingsView())
}

type updateSettingsRequest struct {
	APIKey        string  `json:"api_key"`
	BaseURL       string  `json:"base_url"`
	FallbackModel string  `json:"fallback_model"`
	ThinkingModel string  `json:"thinking_model"`
	Enabled       bool    `json:"enabled"`
	TTSVoiceID    *string `json:"tts_voice_id"`
	StartColor    string  `json:"start_color"`
	EndColor      string  `json:"end_color"`
	Segments      int     `json:"segments"`
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

	voice := a.getTTSConfig().VoiceID
	if req.TTSVoiceID != nil {
		voice = strings.TrimSpace(*req.TTSVoiceID)
		if !validTTSVoice(voice) {
			writeError(w, http.StatusBadRequest, "请选择有效的语音音色")
			return
		}
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
		settingKeyTTSVoiceID:          voice,
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

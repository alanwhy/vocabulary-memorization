package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaskAPIKey(t *testing.T) {
	cases := map[string]string{
		"":           "****",
		"short":      "****",
		"abcdefgh":   "****",
		"abcdefghij": "abcd****ghij",
	}
	for in, want := range cases {
		if got := maskAPIKey(in); got != want {
			t.Errorf("maskAPIKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// speechSettingsFake records only requested keys, keeping obsolete supplier credentials inert.
type speechSettingsFake struct{ values map[string]string }

func (s *speechSettingsFake) SeedIfMissing(ctx context.Context, name, value string) error {
	if _, ok := s.values[name]; !ok {
		s.values[name] = value
	}
	return nil
}
func (s *speechSettingsFake) LoadValues(ctx context.Context, names []string) (map[string]string, error) {
	values := map[string]string{}
	for _, name := range names {
		values[name] = s.values[name]
	}
	return values, nil
}
func (s *speechSettingsFake) UpsertMany(ctx context.Context, values map[string]string) error {
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}

func TestSettingsTTSUsesSharedCredentialsAndIgnoresLegacyKeys(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("DEEPSEEK_TTS_VOICE_ID", "invalid")
	store := &speechSettingsFake{values: map[string]string{"tts_api_key": "legacy-key", "tts_appid": "legacy-appid"}}
	a := New(Dependencies{Settings: store}, Config{})
	defer a.CancelBackground()
	a.loadSettings()
	if cfg := a.getTTSConfig(); cfg.APIKey != "" || cfg.VoiceID != "stella" {
		t.Fatalf("cfg=%+v", cfg)
	}
	store.values[settingKeyAPIKey] = "shared-deepseek-key"
	store.values[settingKeyEnabled] = "false"
	a.refreshSettingsCache(context.Background())
	if !a.getTTSConfig().complete() || a.getDeepSeekConfig().Enabled {
		t.Fatal("lookup switch disabled speech")
	}
	view, _ := json.Marshal(a.maskedSettingsView())
	for _, legacy := range []string{"tts_api_key", "tts_cluster", "tts_voice_type", "shared-deepseek-key", "legacy-key"} {
		if strings.Contains(string(view), legacy) {
			t.Fatalf("view leaked old configuration or key: %s", view)
		}
	}
}

func TestSettingsTTSVoiceValidationAndMaskedKeyPreservation(t *testing.T) {
	store := &speechSettingsFake{values: map[string]string{settingKeyAPIKey: "shared-deepseek-key", settingKeyTTSVoiceID: "stella"}}
	a := New(Dependencies{Settings: store}, Config{})
	defer a.CancelBackground()
	a.loadSettings()
	for _, tc := range []struct {
		voice any
		want  int
	}{{"tide", 200}, {"invalid", 400}, {nil, 200}} {
		payload := map[string]any{"api_key": maskAPIKey("shared-deepseek-key"), "base_url": "https://api.deepseek.com", "fallback_model": "existing-model", "start_color": defaultReviewColorStart, "end_color": defaultReviewColorEnd, "segments": 6}
		if tc.voice != nil {
			payload["tts_voice_id"] = tc.voice
		}
		body, _ := json.Marshal(payload)
		w := httptest.NewRecorder()
		a.handleUpdateSettings(w, httptest.NewRequest("PUT", "/api/admin/settings", strings.NewReader(string(body))))
		if w.Code != tc.want {
			t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
		}
		if a.getTTSConfig().APIKey != "shared-deepseek-key" {
			t.Fatal("mask overwrote shared key")
		}
		if a.getTTSConfig().VoiceID != "tide" {
			t.Fatal("invalid or omitted voice overwrote current voice")
		}
	}
}

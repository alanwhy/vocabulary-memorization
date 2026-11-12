package app

import (
	"context"
	"os"
	"testing"
	"vocab-backend/internal/model"
	"vocab-backend/internal/training"
)

// TestDailyDisabledProviderReturnsClearFailure 区分有词但模型不可用与空词库。
func TestDailyDisabledProviderReturnsClearFailure(t *testing.T) {
	a := &App{}
	_, err := a.generateDailyWord(context.Background(), model.Word{WordKey: "word"})
	p, ok := err.(*training.Problem)
	if !ok || p.Code != "generation_disabled" {
		t.Fatal("unconfigured provider did not report clear failure")
	}
}

// TestLiveDailyGenerator 显式启用时验证真实搜索、生成和答案复核，不写数据库。
func TestLiveDailyGenerator(t *testing.T) {
	key := os.Getenv("TRAINING_LIVE_API_KEY")
	if key == "" {
		t.Skip("live check not enabled")
	}
	a := &App{}
	a.dsConfig = deepseekConfig{Enabled: true, APIKey: key, BaseURL: os.Getenv("TRAINING_LIVE_BASE_URL"), FallbackModel: os.Getenv("TRAINING_LIVE_MODEL")}
	if a.dsConfig.BaseURL == "" {
		a.dsConfig.BaseURL = "https://api.deepseek.com"
	}
	if a.dsConfig.FallbackModel == "" {
		a.dsConfig.FallbackModel = "deepseek-v4-flash"
	}
	b, err := a.generateDailyWord(context.Background(), model.Word{WordKey: "report", DisplayWord: "report", ReviewCount: 9})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Questions) != 1 {
		t.Fatal("expected one exercise")
	}
	t.Log("daily search/generation/review passed; source status:", b.SearchStatus)
}

package training

import (
	"context"
	"os"
	"testing"
	"time"
	"vocab-backend/internal/model"
)

// TestTrainingLiveProvider is opt-in and performs a small real generation plus blind review without writing user data.
func TestTrainingLiveProvider(t *testing.T) {
	key := os.Getenv("TRAINING_LIVE_API_KEY")
	if key == "" {
		t.Skip("live DeepSeek check not enabled")
	}
	base, name := os.Getenv("TRAINING_LIVE_BASE_URL"), os.Getenv("TRAINING_LIVE_MODEL")
	if base == "" {
		base = "https://api.deepseek.com"
	}
	if name == "" {
		name = "deepseek-v4-flash"
	}
	p := NewProvider(base, key, name, 60*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	text, _, err := p.Call(ctx, GeneratePrompt, GenerationData(model.TrainingSeed{Word: "postpone"}, "general", "basic"))
	if err != nil {
		t.Fatal(err)
	}
	var b model.TrainingBundle
	if err = DecodeModel(text, &b); err != nil {
		t.Fatal(err)
	}
	if err = ValidateBundle(b); err != nil {
		t.Fatal(err)
	}
	if len(b.Questions) != 1 {
		t.Fatal("expected one daily exercise")
	}
	for _, q := range b.Questions {
		if err = ValidateQuestion(q, "basic"); err != nil {
			t.Fatal(err)
		}
		text, _, err = p.Call(ctx, ReviewPrompt, ReviewData(q))
		if err != nil {
			t.Fatal(err)
		}
		var review model.TrainingReview
		if err = DecodeModel(text, &review); err != nil {
			t.Fatal(err)
		}
		if err = AcceptReview(q, review); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("real provider generated a daily exercise and passed structural and blind-answer checks")
}

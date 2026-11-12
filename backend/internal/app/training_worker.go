package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"vocab-backend/internal/billing"
	"vocab-backend/internal/model"
	"vocab-backend/internal/training"
)

// startDailyTraining 启动当日内存队列，生成不依赖旧训练数据库表。
func (a *App) startDailyTraining() {
	if a.words != nil {
		a.daily = training.NewDailyManager(a.bgCtx, a.words, a.generateDailyWord, time.Now)
	}
}

// generateDailyWord 自动检索真题来源并生成一道短练习，复核后仅返回给内存队列。
func (a *App) generateDailyWord(parent context.Context, word model.Word) (model.TrainingBundle, error) {
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	cfg := a.getDeepSeekConfig()
	if !cfg.Enabled || cfg.APIKey == "" || cfg.BaseURL == "" || cfg.FallbackModel == "" {
		return model.TrainingBundle{}, training.Fail(503, "generation_disabled", "请先在后台启用并配置 DeepSeek。")
	}
	workflowCtx, release, budgetErr := a.billing.BeginTraining(ctx, cfg.BaseURL, cfg.FallbackModel)
	if budgetErr != nil {
		return model.TrainingBundle{}, trainingBudgetError(budgetErr)
	}
	defer release()
	ctx = workflowCtx
	provider := training.NewProvider(cfg.BaseURL, cfg.APIKey, cfg.FallbackModel, 45*time.Second)
	provider.Billing = a.billing
	searchCtx, searchCancel := context.WithTimeout(ctx, 15*time.Second)
	sources, searchErr := provider.SearchExam(searchCtx, word.WordKey)
	searchCancel()
	if billing.Terminal(searchErr) {
		return model.TrainingBundle{}, trainingBudgetError(searchErr)
	}
	senses := []model.Sense{}
	for _, s := range word.Senses {
		if s.Pos != "error" && s.Pos != "系统提示" {
			senses = append(senses, s)
		}
	}
	seed := model.TrainingSeed{Word: word.WordKey, Senses: senses, ImportantGlosses: word.ImportantGlosses}
	data, _ := json.Marshal(map[string]any{"seed": seed, "exam_sources": training.SourceContext(sources, word.WordKey)})
	text, _, err := provider.Call(ctx, training.GeneratePrompt, string(data))
	if err != nil {
		if billing.Terminal(err) {
			return model.TrainingBundle{}, trainingBudgetError(err)
		}
		return model.TrainingBundle{}, training.Fail(502, "generation_failed", "DeepSeek 暂未生成成功，请重试本词。")
	}
	var b model.TrainingBundle
	if err = training.DecodeModel(text, &b); err != nil {
		return b, err
	}
	if err = training.ValidateBundle(b); err != nil {
		return b, err
	}
	if !strings.EqualFold(strings.TrimSpace(b.Relation.Anchor), strings.TrimSpace(word.WordKey)) {
		return b, training.Fail(422, "changed_word", "生成内容偏离了当前单词，请重试。")
	}
	q := &b.Questions[0]
	reviewText, _, err := provider.Call(ctx, training.ReviewPrompt, training.ReviewData(*q))
	if err != nil {
		if billing.Terminal(err) {
			return b, trainingBudgetError(err)
		}
		return b, training.Fail(502, "review_failed", "题目复核暂未完成，请重试本词。")
	}
	var review model.TrainingReview
	if err = training.DecodeModel(reviewText, &review); err != nil {
		return b, err
	}
	if err = training.AcceptReview(*q, review); err != nil {
		return b, err
	}
	training.ApplyExamCitations(q, sources)
	b.SearchStatus = "no_verified_source"
	if searchErr != nil {
		b.SearchStatus = "unavailable"
	} else if len(sources) > 0 {
		b.SearchStatus = "found"
	}
	return b, nil
}

// trainingBudgetError 让内存练习队列保留明确额度提示，不把配额不足伪装成出题失败。
func trainingBudgetError(err error) error {
	if err == billing.ErrExceeded {
		return training.Fail(429, "ai_budget_exceeded", err.Error())
	}
	return training.Fail(503, "ai_billing_unavailable", billing.ErrUnavailable.Error())
}

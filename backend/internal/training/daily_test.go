package training

import (
	"context"
	"sync"
	"testing"
	"time"
	"vocab-backend/internal/model"
)

type fakeDailyWords struct {
	mu    sync.Mutex
	items []model.Word
}

// ListPage 模拟只读取本人未归档词的稳定分页。
func (f *fakeDailyWords) ListPage(ctx context.Context, user int, archived bool, keyword, status, sort string, limit, offset int) ([]model.Word, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := []model.Word{}
	for _, w := range f.items {
		if !w.Archived {
			all = append(all, w)
		}
	}
	orderWords(all)
	if offset >= len(all) {
		return []model.Word{}, nil
	}
	return append([]model.Word{}, all[offset:min(len(all), offset+limit)]...), nil
}

// FindByIDs 模拟生成后归档发生时的即时过滤。
func (f *fakeDailyWords) FindByIDs(ctx context.Context, user int, ids []int) ([]model.Word, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.Word{}
	for _, w := range f.items {
		for _, id := range ids {
			if w.ID == id {
				out = append(out, w)
			}
		}
	}
	return out, nil
}

// TestDailyEmptyDoesNotCallModel 确保空词库不会创建 AI 请求。
func TestDailyEmptyDoesNotCallModel(t *testing.T) {
	calls := 0
	m := NewDailyManager(context.Background(), &fakeDailyWords{}, func(ctx context.Context, w model.Word) (model.TrainingBundle, error) {
		calls++
		return model.TrainingBundle{}, nil
	}, time.Now)
	defer m.Close()
	s, err := m.Ensure(context.Background(), 1, false, "")
	if err != nil || s.Status != "empty" || calls != 0 {
		t.Fatalf("empty: %v %v %d", s, err, calls)
	}
}

// TestDailySelectionContinuesAndResets 验证排序、下一批、归档排除和日期清空。
func TestDailySelectionContinuesAndResets(t *testing.T) {
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))
	var clockMu sync.Mutex
	clock := func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }
	words := &fakeDailyWords{}
	for i := 1; i <= 13; i++ {
		words.items = append(words.items, model.Word{ID: i, WordKey: "word", DisplayWord: "word", ReviewCount: i, Archived: i == 13})
	}
	gate := make(chan struct{})
	m := NewDailyManager(context.Background(), words, func(ctx context.Context, w model.Word) (model.TrainingBundle, error) {
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return model.TrainingBundle{}, ctx.Err()
	}, clock)
	defer m.Close()
	s, err := m.Ensure(context.Background(), 1, false, "")
	if err != nil || s.Total != 10 || s.Card.WordID != 12 {
		t.Fatalf("first: %+v %v", s, err)
	}
	same, _ := m.Ensure(context.Background(), 1, false, "")
	if same.GroupID != s.GroupID {
		t.Fatal("automatic request duplicated group")
	}
	close(gate)
	m.WaitIdle()
	next, err := m.Ensure(context.Background(), 1, true, "next1")
	if err != nil || next.Total != 2 || next.Card.WordID != 2 {
		t.Fatalf("next: %+v %v", next, err)
	}
	m.WaitIdle()
	duplicate, _ := m.Ensure(context.Background(), 1, true, "next1")
	if duplicate.GroupID != next.GroupID {
		t.Fatal("repeat more request duplicated")
	}
	clockMu.Lock()
	now = now.Add(24 * time.Hour)
	clockMu.Unlock()
	reset, err := m.Ensure(context.Background(), 1, false, "")
	if err != nil || reset.GroupID == next.GroupID || reset.Total != 10 || reset.Card.WordID != 12 {
		t.Fatalf("next day: %+v %v", reset, err)
	}
}

// TestTemporaryAnswersAndArchival 验证临时评分不改变次数，归档后不再返回该词。
func TestTemporaryAnswersAndArchival(t *testing.T) {
	words := &fakeDailyWords{items: []model.Word{{ID: 1, WordKey: "report", DisplayWord: "report", ReviewCount: 9}}}
	m := NewDailyManager(context.Background(), words, func(context.Context, model.Word) (model.TrainingBundle, error) {
		return model.TrainingBundle{Relation: model.TrainingContent{Meaning: "解释", Replacement: "explain"}, Questions: []model.TrainingQuestion{sampleQuestion()}}, nil
	}, time.Now)
	defer m.Close()
	s, _ := m.Ensure(context.Background(), 1, false, "")
	m.WaitIdle()
	choice := "o1"
	s, err := m.Answer(context.Background(), 1, s.GroupID, 1, &choice)
	if err != nil || s.Feedback == nil || !s.Feedback.Correct {
		t.Fatal("temporary answer failed", err)
	}
	s, err = m.Answer(context.Background(), 1, s.GroupID, 1, &choice)
	if err != nil {
		t.Fatal("same answer not idempotent")
	}
	words.mu.Lock()
	if words.items[0].ReviewCount != 9 {
		t.Fatal("practice changed word count")
	}
	words.items[0].Archived = true
	words.mu.Unlock()
	s, err = m.State(context.Background(), 1)
	if err != nil || s.Card != nil || s.Total != 0 {
		t.Fatal("archived word remained visible")
	}
}

// TestConcurrentAutomaticRequestsShareGroup 验证并发进入页面不会重复调用模型。
func TestConcurrentAutomaticRequestsShareGroup(t *testing.T) {
	words := &fakeDailyWords{items: []model.Word{{ID: 1, WordKey: "report", DisplayWord: "report"}}}
	var mu sync.Mutex
	calls := 0
	m := NewDailyManager(context.Background(), words, func(context.Context, model.Word) (model.TrainingBundle, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return model.TrainingBundle{Questions: []model.TrainingQuestion{sampleQuestion()}}, nil
	}, time.Now)
	defer m.Close()
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, e := m.Ensure(context.Background(), 1, false, "")
			if e != nil {
				t.Error(e)
			}
			ids <- s.GroupID
		}()
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal("duplicate automatic group")
		}
	}
	m.WaitIdle()
	if calls != 1 {
		t.Fatalf("model called %d times", calls)
	}
}

package training

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"vocab-backend/internal/model"
)

// examFixture 提供带考试结构的公开来源测试文本，正文来源与生成答案分开。
func examFixture() ExamSource {
	return ExamSource{ID: "exam1", Title: "2020考研英语一真题阅读原文", URL: "https://www.eol.cn/kaoyan/example.html", Year: "2020", Content: "Section II Reading Comprehension Text 1. The report explains why household spending declined. Many readers examine different sources before reaching a conclusion, and these sources help them understand the reasons for changes in consumer behaviour over several years."}
}

// TestExamTagsRequireActualSource 拒绝不存在的引用、改写句、自报标签和复习规划。
func TestExamTagsRequireActualSource(t *testing.T) {
	source := examFixture()
	sentence := "The report explains why household spending declined."
	q := sampleQuestion()
	q.Stem = sentence
	q.ExamSourceID = source.ID
	q.ExamQuote = sentence
	ApplyExamCitations(&q, []ExamSource{source})
	if q.Exam == nil {
		t.Fatal("verified source not tagged")
	}
	q.ExamSourceID = source.ID
	q.ExamQuote = "The report causes household spending to decline."
	ApplyExamCitations(&q, []ExamSource{source})
	if q.Exam != nil {
		t.Fatal("fabricated quote tagged")
	}
	q.ExamSourceID = "unknown"
	q.ExamQuote = sentence
	ApplyExamCitations(&q, []ExamSource{source})
	if q.Exam != nil {
		t.Fatal("unknown source tagged")
	}
	source.Title = "2020考研英语复习规划真题"
	q.ExamSourceID = source.ID
	ApplyExamCitations(&q, []ExamSource{source})
	if q.Exam != nil {
		t.Fatal("study plan tagged")
	}
}

// TestNativeSearchUsesContentAndCurrentKey 验证搜索真正调用原生接口且不会把摘要用作原文。
func TestNativeSearchUsesContentAndCurrentKey(t *testing.T) {
	source := examFixture()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.URL.Path != "/v1/tools/search" && r.URL.Path != "/v1/tools/fetch") || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("native search protocol mismatch")
		}
		var data map[string]any
		json.NewDecoder(r.Body).Decode(&data)
		if r.URL.Path == "/v1/tools/search" && data["need_content"] != true {
			t.Error("search did not request source content")
		}
		if r.URL.Path == "/v1/tools/fetch" {
			json.NewEncoder(w).Encode(map[string]string{"title": source.Title, "content": ""})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]string{{"title": source.Title, "url": source.URL, "content": source.Content}, {"title": source.Title, "url": source.URL, "summary": source.Content}}})
	}))
	defer server.Close()
	p := NewProvider(server.URL, "test-key", "model", 0)
	sources, err := p.SearchExam(context.Background(), "report")
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources=%d err=%v", len(sources), err)
	}
}

// TestForgedTagObjectIsRemoved 确保模型不能直接提供可点击的假真题标签。
func TestForgedTagObjectIsRemoved(t *testing.T) {
	q := model.TrainingQuestion{Exam: &model.TrainingCitation{URL: "https://example.com/fake"}, Options: []model.TrainingOption{{Exam: &model.TrainingCitation{URL: "https://example.com/fake"}}}}
	ApplyExamCitations(&q, nil)
	if q.Exam != nil || q.Options[0].Exam != nil {
		t.Fatal("model tag object trusted")
	}
}

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 嵌入接口让未使用方法仍保持可编译；本测试只实现实际调用的存储操作。
type enrichmentWords struct {
	WordStore
	mu       sync.Mutex
	word     Word
	userID   int
	marks    int
	failSave bool
}

func (f *enrichmentWords) FindByIDs(ctx context.Context, userID int, ids []int) ([]Word, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if userID != f.userID || len(ids) == 0 || ids[0] != f.word.ID {
		return []Word{}, nil
	}
	return []Word{f.word}, nil
}
func (f *enrichmentWords) FindByUserAndKey(ctx context.Context, userID int, key string) (Word, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if userID != f.userID || key != f.word.WordKey {
		return Word{}, nil, sql.ErrNoRows
	}
	raw, _ := json.Marshal(f.word.Senses)
	return f.word, raw, nil
}
func (f *enrichmentWords) IncrementReview(ctx context.Context, id, count int, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.word.ReviewCount = count
	f.word.LastReviewedAt = now
	return nil
}
func (f *enrichmentWords) DueFlashcards(ctx context.Context, userID, limit int, now time.Time) ([]Word, error) {
	return f.FindByIDs(ctx, userID, []int{f.word.ID})
}
func (f *enrichmentWords) TryMarkTranslating(ctx context.Context, id int, now time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.word.Translating {
		return false, nil
	}
	f.word.Translating = true
	f.marks++
	return true, nil
}
func (f *enrichmentWords) ClearTranslating(ctx context.Context, id int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.word.Translating = false
	return nil
}
func (f *enrichmentWords) MarkTranslationStarted(ctx context.Context, id int, now time.Time) error {
	return nil
}
func (f *enrichmentWords) TranslationSenses(ctx context.Context, id int) ([]Sense, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.word.Senses, nil
}
func (f *enrichmentWords) UpdateSenses(ctx context.Context, id int, raw []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSave {
		f.failSave = false
		return errors.New("write failed")
	}
	var senses []Sense
	if err := json.Unmarshal(raw, &senses); err != nil {
		return err
	}
	f.word.Senses = senses
	f.word.Translating = false
	return nil
}

type enrichmentDictionary struct {
	DictionaryStore
	mu    sync.Mutex
	raw   []byte
	saves int
	fail  bool
}

func (f *enrichmentDictionary) LookupSenses(ctx context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.raw, nil
}
func (f *enrichmentDictionary) SaveSenses(ctx context.Context, key string, raw []byte) error {
	return f.ReplaceSenses(ctx, key, raw)
}
func (f *enrichmentDictionary) ReplaceSenses(ctx context.Context, key string, raw []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("cache failed")
	}
	f.raw = raw
	f.saves++
	return nil
}
func (f *enrichmentDictionary) UpsertOccurrence(ctx context.Context, key, display string, now time.Time) error {
	return nil
}

const fullLookup = `{"valid":true,"senses":[{"pos":"v.","translation":"工作；运作","phonetic":"/wɜːk/","example":"They work here.","example_translation":"他们在这里工作。","root":"work","affix":"","synonyms":["labor（劳动）"],"antonyms":[],"lookalikes":["worked（工作过）","word（词）"],"derivatives":["worked（过去式/过去分词：工作过）","worker（名词：工人）"]},{"pos":"n.","translation":"工作","phonetic":"/wɜːk/","example":"The work is done.","example_translation":"工作完成了。","root":"work","affix":"","synonyms":["labor（劳动）"],"antonyms":[],"lookalikes":[],"derivatives":[]}]}`

func enrichmentApp(t *testing.T, lookupHandler http.HandlerFunc) (*App, *enrichmentWords, *enrichmentDictionary, http.Handler) {
	t.Helper()
	app, _, sessions := newTestApp()
	due := time.Now().Add(time.Hour)
	words := &enrichmentWords{userID: 42, word: Word{ID: 9, WordKey: "work", DisplayWord: "work", Senses: []Sense{{Pos: "v.", Translation: "旧释义", Lookalikes: []string{}}}, ReviewCount: 7, IntervalDays: 5, EaseFactor: 2.2, DueAt: &due, ImportantGlosses: []string{"旧释义"}}}
	dict := &enrichmentDictionary{}
	app.words = words
	app.dict = dict
	app.bgCtx, app.bgCancel = context.WithCancel(context.Background())
	app.translateSem = make(chan struct{}, 2)
	server := httptest.NewServer(lookupHandler)
	t.Cleanup(server.Close)
	t.Cleanup(app.bgCancel)
	app.dsConfig = deepseekConfig{APIKey: "test-key", BaseURL: server.URL, FallbackModel: "test-model", Enabled: true}
	sessions.usersByID[42] = User{ID: 42, Username: "test"}
	sessions.byToken["test-token"] = sessionRecord{userID: 42, expiresAt: time.Now().Add(time.Hour)}
	return app, words, dict, app.Handler(t.TempDir())
}
func apiRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}
func modelResponse(w http.ResponseWriter, content string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
}

func TestCorrectionFullHTTP(t *testing.T) {
	var calls atomic.Int32
	var prompt string
	_, words, dict, handler := enrichmentApp(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req deepseekChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		prompt = req.Messages[0].Content
		modelResponse(w, fullLookup)
	})
	before := words.word
	res := apiRequest(handler, "POST", "/api/words/9/correct", `{"reason":"过去式混入形近词"}`)
	if res.Code != 200 {
		t.Fatalf("%d %s", res.Code, res.Body.String())
	}
	var got Word
	_ = json.Unmarshal(res.Body.Bytes(), &got)
	if len(got.Senses) != 2 || got.Senses[0].Translation != "工作；运作" || len(got.Senses[0].Derivatives) != 2 || !reflect.DeepEqual(got.Senses[0].Lookalikes, []string{"word（词）"}) {
		t.Fatalf("完整新结果与去交叉: %+v", got)
	}
	if got.ReviewCount != before.ReviewCount || got.IntervalDays != before.IntervalDays || got.EaseFactor != before.EaseFactor || !got.DueAt.Equal(*before.DueAt) || got.Archived != before.Archived || !reflect.DeepEqual(got.ImportantGlosses, before.ImportantGlosses) {
		t.Fatalf("纠错不应改变复习状态: %+v", got)
	}
	if !strings.Contains(prompt, "过去式混入形近词") || !strings.Contains(prompt, "重新生成所有词性释义") || !strings.Contains(prompt, "不能包含上述时态") {
		t.Fatalf("提示不完整: %s", prompt)
	}
	if dict.saves != 1 || calls.Load() != 1 || words.word.Translating {
		t.Fatal("未保存或未释放预约")
	}
	// 已完整词也必须强制纠错，不能被缓存拦截。
	res = apiRequest(handler, "POST", "/api/words/9/correct", `{"reason":"再核对例句"}`)
	if res.Code != 200 || calls.Load() != 2 || dict.saves != 2 {
		t.Fatal("纠错必须强制绕过缓存")
	}
}
func TestCorrectionErrorsPreserveContent(t *testing.T) {
	for _, tc := range []struct {
		name, body, content       string
		status                    int
		busy, cacheFail, saveFail bool
		id                        string
	}{
		{name: "空原因", body: `{"reason":"  "}`, status: 400},
		{name: "超长原因", body: `{"reason":"` + strings.Repeat("错", 501) + `"}`, status: 400},
		{name: "格式非法", body: `{`, status: 400},
		{name: "非本人词条", body: `{"reason":"错"}`, status: 404, id: "99"},
		{name: "已经查询", body: `{"reason":"错"}`, status: 409, busy: true},
		{name: "模型故障", body: `{"reason":"错"}`, status: 503, content: "bad json"},
		{name: "只有释义和关系字段", body: `{"reason":"错"}`, status: 503, content: `{"valid":true,"senses":[{"pos":"v.","translation":"工作","lookalikes":[],"derivatives":[]}]}`},
		{name: "缺派生字段", body: `{"reason":"错"}`, status: 503, content: `{"valid":true,"senses":[{"pos":"v.","translation":"工作","lookalikes":[]}]}`},
		{name: "拼写错误", body: `{"reason":"错"}`, status: 422, content: `{"valid":false}`},
		{name: "缓存保存失败", body: `{"reason":"错"}`, status: 500, content: fullLookup, cacheFail: true},
		{name: "个人保存失败", body: `{"reason":"错"}`, status: 500, content: fullLookup, saveFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, words, dict, handler := enrichmentApp(t, func(w http.ResponseWriter, r *http.Request) { modelResponse(w, tc.content) })
			words.word.Translating = tc.busy
			dict.fail = tc.cacheFail
			words.failSave = tc.saveFail
			before := words.word.Senses
			id := tc.id
			if id == "" {
				id = "9"
			}
			res := apiRequest(handler, "POST", "/api/words/"+id+"/correct", tc.body)
			if res.Code != tc.status {
				t.Fatalf("%d want %d: %s", res.Code, tc.status, res.Body.String())
			}
			if !reflect.DeepEqual(words.word.Senses, before) || words.word.Translating != tc.busy || words.word.ReviewCount != 7 {
				t.Fatalf("失败改变内容/状态: %+v", words.word)
			}
			if tc.saveFail && dict.saves != 1 {
				t.Fatal("个人保存失败时完整生成的全局新缓存应保留")
			}

		})
	}
}
func TestCorrectionConcurrentSubmission(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	_, words, _, handler := enrichmentApp(t, func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; modelResponse(w, fullLookup) })
	finished := make(chan *httptest.ResponseRecorder)
	go func() { finished <- apiRequest(handler, "POST", "/api/words/9/correct", `{"reason":"纠错"}`) }()
	<-entered
	second := apiRequest(handler, "POST", "/api/words/9/correct", `{"reason":"重复"}`)
	close(release)
	first := <-finished
	if second.Code != 409 || first.Code != 200 || words.marks != 1 {
		t.Fatalf("重复请求: first=%d second=%d marks=%d", first.Code, second.Code, words.marks)
	}
}
func TestFlashcardEnrichmentCacheAndNoRepeatedLookup(t *testing.T) {
	var calls atomic.Int32
	app, words, dict, handler := enrichmentApp(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); modelResponse(w, fullLookup) })
	// 首次旧卡补齐；队列请求不阻塞模型。
	res := apiRequest(handler, "GET", "/api/flashcards/queue", "")
	if res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	app.translateWG.Wait()
	if calls.Load() != 1 || words.word.Translating || !sensesEnriched(words.word.Senses) {
		t.Fatalf("旧卡未补齐: %+v calls=%d", words.word, calls.Load())
	}
	res = apiRequest(handler, "GET", "/api/flashcards/queue", "")
	app.translateWG.Wait()
	if calls.Load() != 1 {
		t.Fatal("已完整卡片不能再次请求模型")
	}
	// 当前用户仍为旧内容而全局缓存新：直接复用，不增加查询次数。
	words.word.Senses = []Sense{{Pos: "v.", Translation: "旧内容", Lookalikes: []string{}}}
	res = apiRequest(handler, "GET", "/api/flashcards/queue", "")
	if res.Code != 200 || calls.Load() != 1 || !sensesEnriched(words.word.Senses) || dict.saves != 1 {
		t.Fatal("新缓存未复用")
	}
	// [] 是完成标记。
	words.word.Senses = []Sense{{Pos: "v.", Translation: "工作", Lookalikes: []string{}, Derivatives: []string{}}}
	_ = apiRequest(handler, "GET", "/api/flashcards/queue", "")
	app.translateWG.Wait()
	if calls.Load() != 1 {
		t.Fatal("空派生列表不应重复查")
	}
}
func TestRepeatedInputReusesEnrichedCache(t *testing.T) {
	var calls atomic.Int32
	app, words, dict, handler := enrichmentApp(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); modelResponse(w, fullLookup) })
	dict.raw = []byte(`[{"pos":"v.","translation":"工作","lookalikes":[],"derivatives":[]}]`)
	res := apiRequest(handler, "POST", "/api/words", `{"word":"WORK"}`)
	app.translateWG.Wait()
	if res.Code != 200 || calls.Load() != 0 || words.word.ReviewCount != 8 || !sensesEnriched(words.word.Senses) {
		t.Fatalf("输入旧词复用缓存: %d %+v", res.Code, words.word)
	}
}

// staleSnapshotWords 模拟另一个纠错在当前请求预约前已完成，首次读仍拿到旧快照。
type staleSnapshotWords struct {
	*enrichmentWords
	reads      int
	old        Word
	failLatest bool
}

func (f *staleSnapshotWords) FindByIDs(ctx context.Context, userID int, ids []int) ([]Word, error) {
	f.reads++
	if f.reads == 1 {
		return []Word{f.old}, nil
	}
	if f.failLatest {
		return nil, errors.New("read failed")
	}
	return f.enrichmentWords.FindByIDs(ctx, userID, ids)
}
func TestCorrectionFailureDoesNotRestoreStaleSnapshot(t *testing.T) {
	for _, failRead := range []bool{false, true} {
		t.Run(fmt.Sprintf("latestReadFail=%v", failRead), func(t *testing.T) {
			app, words, _, handler := enrichmentApp(t, func(w http.ResponseWriter, r *http.Request) { modelResponse(w, "bad json") })
			old := words.word
			words.word.Senses = []Sense{{Pos: "v.", Translation: "另一次纠错已成功", Lookalikes: []string{}, Derivatives: []string{}}}
			app.words = &staleSnapshotWords{enrichmentWords: words, old: old, failLatest: failRead}
			res := apiRequest(handler, "POST", "/api/words/9/correct", `{"reason":"重查"}`)
			if res.Code != 503 && res.Code != 500 {
				t.Fatalf("%d %s", res.Code, res.Body.String())
			}
			if words.word.Translating || words.word.Senses[0].Translation != "另一次纠错已成功" {
				t.Fatalf("失败不应撤销已完成纠错: %+v", words.word)
			}
		})
	}
}

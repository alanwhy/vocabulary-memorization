package billing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryLedger struct {
	mu              sync.Mutex
	limit           *int64
	spent, reserved int64
	disabled        bool
	records         map[string]Reservation
	states          map[string]string
	dates           map[string][2]int64
}

// testLedger 是 HTTP 记账测试用持久化接口替身，不替代真实 MySQL 事务测试。
func testLedger(limit *int64) *memoryLedger {
	return &memoryLedger{limit: limit, records: map[string]Reservation{}, states: map[string]string{}, dates: map[string][2]int64{}}
}

// Snapshot 返回测试账本的当日精确金额。
func (m *memoryLedger) Snapshot(_ context.Context, _ int, date string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.dates[date]
	return Snapshot{Date: date, Limit: m.limit, Spent: v[0], Reserved: v[1]}, nil
}

// WordOwner 模拟恢复任务从数据库取得已确认所有者。
func (m *memoryLedger) WordOwner(_ context.Context, _ int) (int, error) { return 1, nil }

// Reserve 模拟原子账本供服务流程测试，保留记录便于核对预留与结算。
func (m *memoryLedger) Reserve(_ context.Context, b Reservation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled {
		return ErrDisabled
	}
	if b.Unpriced && m.limit != nil {
		return ErrUnavailable
	}
	v := m.dates[b.Date]
	extra := b.Amount
	if b.ParentID != "" {
		p := m.records[b.ParentID]
		if p.Amount < b.Amount || m.states[b.ParentID] != "reserved" {
			return ErrExceeded
		}
		extra = 0
	}
	if m.limit != nil && (*m.limit == 0 || v[0]+v[1]+extra > *m.limit) {
		return ErrExceeded
	}
	if b.ParentID != "" {
		p := m.records[b.ParentID]
		p.Amount -= b.Amount
		m.records[b.ParentID] = p
	}
	v[1] += extra
	m.dates[b.Date] = v
	m.records[b.ID] = b
	m.states[b.ID] = "reserved"
	m.reserved += extra
	return nil
}

// Settle 记录结算状态，未知费用保留预留，并允许幂等重复完成。
func (m *memoryLedger) Settle(_ context.Context, id string, n int64, state string, _ []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.records[id]
	if m.states[id] != "reserved" && m.states[id] != "uncertain" {
		return nil
	}
	if state != "uncertain" {
		v := m.dates[b.Date]
		v[0] += n
		v[1] -= b.Amount
		m.dates[b.Date] = v
		m.spent += n
		m.reserved -= b.Amount
	}
	m.states[id] = state
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip 将模拟响应交给受控请求服务，不发出网络请求。
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// request 构建含用户身份、有界输出的模型请求。
func request(ctx context.Context) *http.Request {
	r, _ := http.NewRequestWithContext(WithOwner(ctx, 1, "test"), "POST", "https://api.deepseek.com/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}],"max_tokens":4096}`))
	return r
}

// service 构造固定上海日期、空闲时段的受控请求测试对象。
func service(m *memoryLedger) *Service {
	return &Service{Store: m, Now: func() time.Time { return time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC) }}
}

// responseClient 返回指定模拟状态和正文，参数 calls 用于确认是否真正发出请求。
func responseClient(status int, body string, calls *int) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		*calls++
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

// TestMoneyAndLimits 验证零、不限、分精度以及拒绝非法输入，不使用浮点数。
func TestMoneyAndLimits(t *testing.T) {
	for raw, want := range map[string]int64{`"5.00"`: 5 * NanoPerYuan, `0`: 0, `"0.01"`: 10_000_000, `"1000000"`: MaxLimit} {
		n, err := ParseLimit([]byte(raw))
		if err != nil || n == nil || *n != want {
			t.Fatalf("%s: %v %v", raw, n, err)
		}
	}
	for _, raw := range []string{"", `-1`, `"5.001"`, `"1e3"`, `true`, `"1000000.01"`} {
		if _, err := ParseLimit([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if n, err := ParseLimit([]byte("null")); n != nil || err != nil {
		t.Fatal(n, err)
	}
	if Money(40) != "0.00000004" || Money(DefaultLimit) != "5.00" {
		t.Fatal("amount lost precision")
	}
	n := DefaultLimit
	raw, _ := json.Marshal(Snapshot{Date: "2026-11-12", Limit: &n, Spent: 1, Reserved: 2})
	if !strings.Contains(string(raw), `"remaining_cny":"4.999999997"`) {
		t.Fatal(string(raw))
	}
}

// TestUsagePrices 验证缓存/输出分别计费，推理 token 不重复计费及不可用用量拒绝。
func TestUsagePrices(t *testing.T) {
	p, _ := PriceFor("https://api.deepseek.com", "deepseek-flash", time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))
	var usage Usage
	json.Unmarshal([]byte(`{"prompt_tokens":100,"prompt_cache_hit_tokens":80,"prompt_cache_miss_tokens":20,"completion_tokens":50,"completion_tokens_details":{"reasoning_tokens":30}}`), &usage)
	n, err := p.Cost(usage, 1000, 4096)
	if err != nil || n != 221600 {
		t.Fatal(n, err)
	}
	usage.Hit = nil
	usage.Miss = nil
	n, err = p.Cost(usage, 1000, 4096)
	if err != nil || n != 300000 {
		t.Fatal(n, err)
	}
	if _, err = p.Cost(Usage{}, 1000, 4096); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, ok := PriceFor("https://api.deepseek.com.evil.test", "deepseek-flash", time.Now()); ok {
		t.Fatal("accepted foreign provider")
	}
	if _, ok := PriceFor("https://api.deepseek.com", "unknown-model", time.Now()); ok {
		t.Fatal("unknown price")
	}
	peak, _ := PriceFor("https://api.deepseek.com", "deepseek-flash", time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC))
	if !peak.Peak || peak.Miss != 2000 {
		t.Fatal(peak)
	}
}

// TestModelUsageSettlesBeforeBusinessParsing 不合法业务内容有 usage 时仍计费且响应可再次读取。
func TestModelUsageSettlesBeforeBusinessParsing(t *testing.T) {
	limit := DefaultLimit
	m := testLedger(&limit)
	calls := 0
	body := `{"choices":[{"message":{"content":"invalid business JSON"}}],"usage":{"prompt_tokens":100,"completion_tokens":50,"prompt_cache_hit_tokens":80,"prompt_cache_miss_tokens":20}}`
	res, err := service(m).Do(responseClient(200, body, &calls), request(context.Background()), "https://api.deepseek.com", "deepseek-flash")
	if err != nil || calls != 1 || m.spent != 221600 || m.reserved != 0 {
		t.Fatal(err, calls, m.spent, m.reserved)
	}
	raw, _ := io.ReadAll(res.Body)
	if string(raw) != body {
		t.Fatal("response consumed")
	}
}

// TestNoBudgetNoNetwork 额度不足、未知模型及无用户不能发起供应商请求。
func TestNoBudgetNoNetwork(t *testing.T) {
	zero := int64(0)
	m := testLedger(&zero)
	calls := 0
	s := service(m)
	if _, err := s.Do(responseClient(200, "{}", &calls), request(context.Background()), "https://api.deepseek.com", "deepseek-flash"); !errors.Is(err, ErrExceeded) {
		t.Fatal(err)
	}
	if _, err := s.Do(responseClient(200, "{}", &calls), request(context.Background()), "https://custom.example", "unknown"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	req := request(context.Background()).WithContext(context.Background())
	if _, err := s.Do(responseClient(200, "{}", &calls), req, "https://api.deepseek.com", "deepseek-flash"); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("blocked request reached provider")
	}
}

// TestUnknownAndHTTPFailures 未知 usage/网络错误不退还预留，明确拒绝请求释放。
func TestUnknownAndHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		uncertain bool
	}{{"missing usage", 200, `{"choices":[]}`, true}, {"rate limit", 429, `{}`, false}, {"server error", 500, `{}`, true}, {"redirect", 307, `{}`, true}} {
		t.Run(tc.name, func(t *testing.T) {
			limit := DefaultLimit
			m := testLedger(&limit)
			calls := 0
			_, err := service(m).Do(responseClient(tc.status, tc.body, &calls), request(context.Background()), "https://api.deepseek.com", "deepseek-flash")
			if tc.uncertain && (err == nil || m.reserved == 0) {
				t.Fatal(err, m.reserved)
			}
			if !tc.uncertain && (err != nil || m.reserved != 0) {
				t.Fatal(err, m.reserved)
			}
		})
	}
	limit := DefaultLimit
	m := testLedger(&limit)
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}
	if _, err := service(m).Do(client, request(context.Background()), "https://api.deepseek.com", "deepseek-flash"); !errors.Is(err, ErrUnavailable) || m.reserved == 0 {
		t.Fatal(err, m.reserved)
	}
}

// TestToolBilling 只对有搜索结果或网页正文的成功工具请求计固定费用。
func TestToolBilling(t *testing.T) {
	for _, tc := range []struct {
		path, body string
		want       int64
	}{{"search", `{"results":[]}`, 0}, {"search", `{"results":[{"title":"irrelevant"}]}`, ToolCost}, {"fetch", `{"content":""}`, 0}, {"fetch", `{"content":"page"}`, ToolCost}} {
		limit := DefaultLimit
		m := testLedger(&limit)
		calls := 0
		req, _ := http.NewRequestWithContext(WithOwner(context.Background(), 1, "tool"), "POST", "https://api.deepseek.com/v1/tools/"+tc.path, strings.NewReader(`{}`))
		_, err := service(m).Do(responseClient(200, tc.body, &calls), req, "https://api.deepseek.com", "deepseek-flash")
		if err != nil || m.spent != tc.want || m.reserved != 0 {
			t.Fatal(tc, err, m.spent, m.reserved)
		}
	}
}

// TestWorkflowReserveAndDayChange 两步工作流不重复占用，午夜新调用在新日期预留。
func TestWorkflowReserveAndDayChange(t *testing.T) {
	limit := DefaultLimit
	m := testLedger(&limit)
	s := service(m)
	ctx, release, err := s.BeginTraining(WithOwner(context.Background(), 1, "training"), "https://api.deepseek.com", "deepseek-flash")
	if err != nil {
		t.Fatal(err)
	}
	before := m.reserved
	calls := 0
	body := `{"usage":{"prompt_tokens":100,"completion_tokens":50}}`
	if _, err = s.Do(responseClient(200, body, &calls), request(ctx), "https://api.deepseek.com", "deepseek-flash"); err != nil {
		t.Fatal(err)
	}
	if m.reserved >= before || m.spent != 300000 {
		t.Fatal("workflow double reserved", m.reserved, before, m.spent)
	}
	s.Now = func() time.Time { return time.Date(2026, 11, 13, 0, 0, 0, 0, time.FixedZone("CST", 8*3600)) }
	if _, err = s.Do(responseClient(200, body, &calls), request(ctx), "https://api.deepseek.com", "deepseek-flash"); err != nil {
		t.Fatal(err)
	}
	release()
	if m.reserved != 0 || m.dates["2026-11-12"][0] != 300000 || m.dates["2026-11-13"][0] != 300000 {
		t.Fatal(m.dates, m.reserved)
	}
}

// TestWorkflowRechecksLoweredLimit 已预留工作流在降额或禁用后不能继续发起下一步。
func TestWorkflowRechecksLoweredLimit(t *testing.T) {
	limit := DefaultLimit
	m := testLedger(&limit)
	s := service(m)
	ctx, release, err := s.BeginTraining(WithOwner(context.Background(), 1, "training"), "https://api.deepseek.com", "deepseek-flash")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	limit = 0
	calls := 0
	if _, err = s.Do(responseClient(200, `{}`, &calls), request(ctx), "https://api.deepseek.com", "deepseek-flash"); !errors.Is(err, ErrExceeded) || calls != 0 {
		t.Fatal(err, calls)
	}
}

// TestParallelRequestsCannotSpendReservedBudget 第一笔尚未完成时第二笔不能使用已预留的额度。
func TestParallelRequestsCannotSpendReservedBudget(t *testing.T) {
	p, _ := PriceFor("https://api.deepseek.com", "deepseek-flash", time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))
	limit := p.Upper(len("user")+len("hello")+256, 4096)
	m := testLedger(&limit)
	s := service(m)
	started, proceed := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		close(started)
		<-proceed
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":100,"completion_tokens":50}}`)), Header: make(http.Header)}, nil
	})}
	go func() {
		_, err := s.Do(client, request(context.Background()), "https://api.deepseek.com", "deepseek-flash")
		done <- err
	}()
	<-started
	if _, err := s.Do(client, request(context.Background()), "https://api.deepseek.com", "deepseek-flash"); !errors.Is(err, ErrExceeded) {
		close(proceed)
		t.Fatal(err)
	}
	close(proceed)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if m.reserved != 0 || m.spent != 300000 {
		t.Fatal(m.spent, m.reserved)
	}
}

// TestResponseCannotExceedRequestedOutput 供应商返回超出请求上限的 usage 时保留占用待核算。
func TestResponseCannotExceedRequestedOutput(t *testing.T) {
	limit := DefaultLimit
	m := testLedger(&limit)
	calls := 0
	s := service(m)
	req, _ := http.NewRequestWithContext(WithOwner(context.Background(), 1, "test"), "POST", "https://api.deepseek.com/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}],"max_tokens":1}`))
	if _, err := s.Do(responseClient(200, `{"usage":{"prompt_tokens":1,"completion_tokens":2}}`, &calls), req, "https://api.deepseek.com", "deepseek-flash"); !errors.Is(err, ErrUnavailable) || m.reserved == 0 {
		t.Fatal(err, m.reserved)
	}
}

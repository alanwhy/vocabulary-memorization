package billing

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// newID 生成供应商调用唯一 ID，返回随机数错误时不得发出付费请求。
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// finish 使用独立短上下文结算，用户取消或模型超时不能取消已经产生的费用。
func (s *Service) finish(id string, amount int64, state string, details []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.Store.Settle(ctx, id, amount, state, details)
}

// BeginTraining 为两次模型请求及最多六次工具请求预留完整费用，退出释放未发出部分。
func (s *Service) BeginTraining(ctx context.Context, base, model string) (context.Context, func(), error) {
	if s == nil {
		return ctx, func() {}, nil
	}
	user, purpose := Owner(ctx)
	if user <= 0 {
		return ctx, nil, ErrDisabled
	}
	now := s.Now()
	price, known := PriceFor(base, model, now)
	amount := int64(0)
	if known {
		amount = 2*price.Upper(MaxInputBytes+1024, MaxOutputTokens) + 6*ToolCost
	}
	snapshot, err := s.Store.Snapshot(ctx, user, Day(now))
	if err != nil {
		return ctx, nil, err
	}
	if !known && snapshot.Limit != nil {
		return ctx, nil, ErrUnavailable
	}
	id, err := newID()
	if err != nil {
		return ctx, nil, err
	}
	details, _ := json.Marshal(price)
	err = s.Store.Reserve(ctx, Reservation{Unpriced: !known, ID: id, Date: Day(now), Purpose: purpose + "_workflow", Provider: providerName(base), Model: model, UserID: user, Amount: amount, Details: details})
	if err != nil {
		return ctx, nil, err
	}
	return context.WithValue(ctx, parentKey{}, parent{id, Day(now)}), func() { _ = s.finish(id, 0, "released", nil) }, nil
}

// Do 在真正 HTTP 调用前原子预留，完整读取 usage 后结算，再把原响应交还业务客户端。
// 参数 client 保留各入口超时设置；req 必须含服务器确认的 owner；费用不明返回 ErrUnavailable。
func (s *Service) Do(client *http.Client, req *http.Request, base, model string) (*http.Response, error) {
	if s == nil {
		return client.Do(req)
	}
	user, purpose := Owner(req.Context())
	if user <= 0 {
		return nil, ErrDisabled
	}
	if req.Context().Err() != nil {
		return nil, req.Context().Err()
	}
	now := s.Now()
	date := Day(now)
	price, known := PriceFor(base, model, now)
	if later, ok := PriceFor(base, model, now.Add(client.Timeout)); ok && later.Peak && !price.Peak {
		price = later
	}
	isTool := strings.HasSuffix(req.URL.Path, "/tools/search") || strings.HasSuffix(req.URL.Path, "/tools/fetch")
	inputBound := 0
	outputBound := MaxOutputTokens
	amount := int64(0)
	if !isTool {
		payload, err := io.ReadAll(io.LimitReader(req.Body, MaxInputBytes*2+1))
		req.Body.Close()
		if err != nil || len(payload) > MaxInputBytes*2 {
			return nil, ErrUnavailable
		}
		var body struct {
			Messages  []struct{ Role, Content string }
			MaxTokens int `json:"max_tokens"`
		}
		if json.Unmarshal(payload, &body) != nil || body.MaxTokens <= 0 || body.MaxTokens > MaxOutputTokens || len(body.Messages) == 0 || len(body.Messages) > 4 {
			return nil, ErrUnavailable
		}
		outputBound = body.MaxTokens
		for _, m := range body.Messages {
			inputBound += len(m.Role) + len(m.Content) + 256
		}
		if inputBound > MaxInputBytes+1024 {
			return nil, ErrUnavailable
		}
		req.Body = io.NopCloser(bytes.NewReader(payload))
		req.ContentLength = int64(len(payload))
		// 禁止 net/http 在响应前透明重放付费 POST 或把请求重定向到不同供应商。
		req.GetBody = nil
		if known {
			amount = price.Upper(inputBound, body.MaxTokens)
		}
	} else if known {
		amount = ToolCost
	}
	snapshot, err := s.Store.Snapshot(req.Context(), user, date)
	if err != nil {
		return nil, err
	}
	if !known && snapshot.Limit != nil {
		return nil, ErrUnavailable
	}
	id, err := newID()
	if err != nil {
		return nil, err
	}
	parentID := ""
	if p, ok := req.Context().Value(parentKey{}).(parent); ok && p.Date == date {
		parentID = p.ID
	}
	details, _ := json.Marshal(price)
	booking := Reservation{Unpriced: !known, ID: id, ParentID: parentID, Date: date, Purpose: purpose, Provider: providerName(base), Model: model, UserID: user, Amount: amount, Details: details}
	if err = s.Store.Reserve(req.Context(), booking); err != nil {
		return nil, err
	}
	if req.Context().Err() != nil {
		_ = s.finish(id, 0, "released", nil)
		return nil, req.Context().Err()
	}
	safeClient := *client
	safeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := safeClient.Do(req)
	if err != nil {
		_ = s.finish(id, 0, "uncertain", nil)
		return nil, ErrUnavailable
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(raw))
	if readErr != nil || len(raw) > 4<<20 {
		_ = s.finish(id, 0, "uncertain", nil)
		return nil, ErrUnavailable
	}
	if response.StatusCode != http.StatusOK {
		// 4xx 明确拒绝执行；服务端故障/重定向不能假设没有账单。
		state := "uncertain"
		if response.StatusCode >= 400 && response.StatusCode < 500 {
			state = "released"
		}
		if err = s.finish(id, 0, state, nil); err != nil {
			return nil, ErrUnavailable
		}
		if state == "uncertain" {
			return nil, ErrUnavailable
		}
		return response, nil
	}
	if !known {
		_ = s.finish(id, 0, "uncertain", nil)
		return response, nil
	}
	actual := int64(0)
	responseModel, responseID := model, ""
	var usage Usage
	if isTool {
		var result struct {
			Results []json.RawMessage `json:"results"`
			Content string            `json:"content"`
		}
		if json.Unmarshal(raw, &result) != nil {
			_ = s.finish(id, 0, "uncertain", nil)
			return nil, ErrUnavailable
		}
		if strings.HasSuffix(req.URL.Path, "/search") && len(result.Results) > 0 || strings.HasSuffix(req.URL.Path, "/fetch") && strings.TrimSpace(result.Content) != "" {
			actual = ToolCost
		}
	} else {
		var result struct {
			Usage Usage  `json:"usage"`
			Model string `json:"model"`
			ID    string `json:"id"`
		}
		if json.Unmarshal(raw, &result) != nil {
			_ = s.finish(id, 0, "uncertain", nil)
			return nil, ErrUnavailable
		}
		usage = result.Usage
		responseID = result.ID
		if result.Model != "" {
			responseModel = result.Model
		}
		if completed, ok := PriceFor(base, model, s.Now()); ok && completed.Peak && !price.Peak {
			price = completed
		}
		// 别名响应可能是版本化 Flash；不能因其它模型返回而使用廉价费率。
		if result.Model != "" && result.Model != model && !sameModel(model, result.Model) {
			_ = s.finish(id, 0, "uncertain", nil)
			return nil, ErrUnavailable
		}
		actual, err = price.Cost(usage, inputBound, outputBound)
		if err != nil || actual > amount {
			_ = s.finish(id, 0, "uncertain", nil)
			return nil, ErrUnavailable
		}
	}
	audit, _ := json.Marshal(struct {
		Price         Price  `json:"price"`
		Usage         Usage  `json:"usage"`
		ResponseModel string `json:"response_model"`
		ResponseID    string `json:"response_id"`
	}{price, usage, responseModel, responseID})
	if err = s.finish(id, actual, "settled", audit); err != nil {
		return nil, ErrUnavailable
	}
	return response, nil
}

// sameModel 只认可已确认的同费率别名或版本化型号，避免跨模型低估费用。
func sameModel(requested, response string) bool {
	r := strings.ToLower(response)
	switch requested {
	case "deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp":
		return r == "deepseek-flash" || r == "deepseek-v4-flash" || r == "deepseek-v4-flash-vision-exp" || strings.HasPrefix(r, "deepseek-v4.1-flash-")
	case "deepseek-v4-pro":
		return strings.HasPrefix(r, "deepseek-v4-pro-")
	}
	return false
}

// providerName 仅保留供应商地址，不将 URL 中的凭证、查询参数或片段写入账本。
func providerName(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return "unknown"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	value := u.String()
	if len(value) > 512 {
		return "custom-provider"
	}
	return value
}

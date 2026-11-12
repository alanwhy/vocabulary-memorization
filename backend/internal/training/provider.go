package training

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"vocab-backend/internal/billing"
)

type Provider struct {
	Client  *http.Client
	BaseURL string
	APIKey  string
	Model   string
	Billing *billing.Service
}
type ProviderError struct {
	Status    int
	Retryable bool
}

// Error suppresses remote response content and credentials in failure records.
func (e *ProviderError) Error() string { return "出题服务暂时无法完成请求" }

// NewProvider creates a dedicated bounded client without changing the dictionary client.
func NewProvider(base, key, name string, timeout time.Duration) *Provider {
	return &Provider{Client: &http.Client{Timeout: timeout}, BaseURL: strings.TrimRight(base, "/"), APIKey: key, Model: name}
}

// Call performs one generation/review request; the worker owns retries and durable request accounting.
func (p *Provider) Call(ctx context.Context, system, data string) (string, modelUsage, error) {
	payload, _ := json.Marshal(map[string]any{"model": p.Model, "temperature": 0, "max_tokens": billing.MaxOutputTokens, "thinking": map[string]string{"type": "disabled"}, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": data}}})
	req, err := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", modelUsage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.Billing.Do(p.Client, req, p.BaseURL, p.Model)
	if err != nil {
		if billing.Terminal(err) {
			return "", modelUsage{}, err
		}
		return "", modelUsage{}, &ProviderError{Retryable: true}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", modelUsage{}, &ProviderError{Status: res.StatusCode, Retryable: res.StatusCode == 429 || res.StatusCode >= 500}
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return "", modelUsage{}, Fail(422, "provider_response_too_large", "出题服务返回内容过大或不完整")
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage modelUsage `json:"usage"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return "", modelUsage{}, Fail(422, "empty_model_response", "出题服务没有返回有效内容")
	}
	return result.Choices[0].Message.Content, result.Usage, nil
}

type modelUsage struct {
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
}

// Tokens exposes nullable token counts for durable accounting without inventing missing usage.
func (u modelUsage) Tokens() (*int, *int) { return u.PromptTokens, u.CompletionTokens }

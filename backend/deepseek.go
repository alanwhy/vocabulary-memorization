package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var deepseekHTTPClient = &http.Client{Timeout: 20 * time.Second}

type deepseekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepseekChatRequest struct {
	Model       string            `json:"model"`
	Messages    []deepseekMessage `json:"messages"`
	Temperature float64           `json:"temperature"`
}

type deepseekChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// lookupDeepSeek 调用 DeepSeek 的 chat completions 接口查询单词的词性和释义，
// 一个词性对应一条 Sense，一个单词可能返回多条
func lookupDeepSeek(word string) ([]Sense, error) {
	cfg := getDeepSeekConfig()
	if !cfg.Enabled {
		return nil, fmt.Errorf("deepseek 未启用")
	}
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.Model == "" {
		return nil, fmt.Errorf("deepseek 配置不完整")
	}

	prompt := fmt.Sprintf(`你是一个英汉词典。给出英文单词 "%s" 的所有常见词性及对应的简洁中文释义。
严格只返回一个 JSON 数组，不要包含任何解释文字或 markdown 代码块标记。
数组每个元素形如 {"pos":"词性缩写，如 n./v./adj./adv.","translation":"简洁中文释义"}。
每个词性只返回一条：同一词性如果有多个常见释义，合并成一条，用中文分号"；"分隔；不同词性分别返回不同的一条，不要合并到一起。`, word)

	reqBody := deepseekChatRequest{
		Model:       cfg.Model,
		Messages:    []deepseekMessage{{Role: "user", Content: prompt}},
		Temperature: 0,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", cfg.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := deepseekHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek 接口返回状态码 %d", resp.StatusCode)
	}

	var chatResp deepseekChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("解析 deepseek 响应失败: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("deepseek 未返回任何结果")
	}

	senses, err := parseSenses(chatResp.Choices[0].Message.Content)
	if err != nil {
		return nil, err
	}
	return senses, nil
}

// parseSenses 解析模型返回的 JSON 数组文本，容忍模型偶尔多包一层 markdown 代码块
func parseSenses(content string) ([]Sense, error) {
	text := strings.TrimSpace(content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var senses []Sense
	if err := json.Unmarshal([]byte(text), &senses); err != nil {
		return nil, fmt.Errorf("deepseek 返回内容不是预期的 JSON 格式: %w", err)
	}

	valid := make([]Sense, 0, len(senses))
	for _, s := range senses {
		pos := strings.TrimSpace(s.Pos)
		translation := strings.TrimSpace(s.Translation)
		if pos == "" || translation == "" {
			continue
		}
		valid = append(valid, Sense{Pos: pos, Translation: translation})
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("deepseek 未返回有效词性释义")
	}
	return valid, nil
}

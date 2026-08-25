package main

import (
	"bytes"
	"context"
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
func lookupDeepSeek(ctx context.Context, word string, cfg deepseekConfig) ([]Sense, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("deepseek 未启用")
	}
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.FallbackModel == "" {
		return nil, fmt.Errorf("deepseek 配置不完整")
	}

	prompt := fmt.Sprintf(`你是一个英汉词典。给出英文单词 "%s" 的完整信息。
严格只返回一个 JSON 数组，不要包含任何解释文字或 markdown 代码块标记。
数组每个元素形如：
{"pos":"词性缩写，如 n./v./adj./adv.","translation":"简洁中文释义","phonetic":"国际音标，如 /ˈæp(ə)l/","example":"该词性下的一个简短英文例句","example_translation":"该例句的中文翻译","root":"词根，无则空字符串","affix":"词缀，无则空字符串","synonyms":["近义词"],"antonyms":["反义词"],"lookalikes":["形近词"]}
要求：
- 每个词性只返回一条；同一词性多个常见释义合并成一条，用中文分号"；"分隔；不同词性分别返回不同的一条，不要合并到一起。
- phonetic/root/affix/synonyms/antonyms/lookalikes 是词级信息，每条保持一致（重复填在每个元素里）。
- example 是英文例句，example_translation 是它的中文翻译。
- synonyms/antonyms 每个元素格式为"英文词（中文释义）"，例如"chance（机会）"；没有则为空数组 []。
- lookalikes 是拼写相近、容易混淆的形近词，格式同上"英文词（中文释义）"；没有则为空数组 []。`, word)

	reqBody := deepseekChatRequest{
		Model:       cfg.FallbackModel,
		Messages:    []deepseekMessage{{Role: "user", Content: prompt}},
		Temperature: 0,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.BaseURL+"/chat/completions", bytes.NewReader(payload))
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
		s.Pos = strings.TrimSpace(s.Pos)
		s.Translation = strings.TrimSpace(s.Translation)
		s.Phonetic = strings.TrimSpace(s.Phonetic)
		s.Example = strings.TrimSpace(s.Example)
		s.ExampleTranslation = strings.TrimSpace(s.ExampleTranslation)
		s.Root = strings.TrimSpace(s.Root)
		s.Affix = strings.TrimSpace(s.Affix)
		if s.Pos == "" || s.Translation == "" {
			continue
		}
		valid = append(valid, s)
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("deepseek 未返回有效词性释义")
	}
	return valid, nil
}

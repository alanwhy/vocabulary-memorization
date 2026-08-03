package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// translateResult 翻译结果
type translateResult struct {
	Translation string
	Pos         string
	Source      string // "dict" 内置词典 / "online" 在线接口 / "none" 均未获取到
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

// translateWord 依次尝试：内置词典 -> 在线免费接口兜底。
// 即使翻译失败，也不会阻塞记录单词本身（调用方仍会把单词存进数据库，只是释义留空）。
func translateWord(wordKey string) translateResult {
	if entry, ok := lookupBuiltinDict(wordKey); ok {
		return translateResult{Translation: entry.Translation, Pos: entry.Pos, Source: "dict"}
	}

	translation, err := translateOnline(wordKey)
	if err != nil {
		log.Printf("在线翻译失败 word=%s err=%v", wordKey, err)
		return translateResult{Translation: "", Pos: "", Source: "none"}
	}
	return translateResult{Translation: translation, Pos: "未知", Source: "online"}
}

// 说明：这是 Google 翻译免注册、免 API Key 的公开网页接口，未来如需更稳定的翻译效果，
// 可以在此文件里新增一个使用百度翻译开放平台/有道智云正式 API Key 的实现，替换掉 translateOnline 即可。
func translateOnline(word string) (string, error) {
	endpoint := "https://translate.googleapis.com/translate_a/single?client=gtx&sl=en&tl=zh-CN&dt=t&q=" + url.QueryEscape(word)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 响应形如 [[["译文","原文",null,null,3,...], ...], null, "en", ...]
	var result []interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析翻译结果失败: %w", err)
	}
	if len(result) == 0 {
		return "", fmt.Errorf("翻译接口未返回有效结果")
	}
	segments, ok := result[0].([]interface{})
	if !ok || len(segments) == 0 {
		return "", fmt.Errorf("翻译接口未返回有效结果")
	}

	var parts []string
	for _, seg := range segments {
		segArr, ok := seg.([]interface{})
		if !ok || len(segArr) == 0 {
			continue
		}
		if text, ok := segArr[0].(string); ok && text != "" {
			parts = append(parts, text)
		}
	}
	translation := strings.Join(parts, "")
	if translation == "" {
		return "", fmt.Errorf("翻译结果为空")
	}
	return translation, nil
}

package main

import (
	"context"
	"log"
)

// translateResult 翻译结果，一个单词可能有多个词性，每个词性对应一条 Sense。
// IsSpellingError 为 true 表示模型判定该词拼写错误（不是真实英文单词），
// 此时 Senses 为空、Source 仍为 deepseek（不是接口失败，无需重试）。
type translateResult struct {
	Senses          []Sense
	Source          string // "deepseek" / "none" 均未获取到
	IsSpellingError bool
}

// translateWord 调用 DeepSeek 兜底模型查词。查词失败不阻塞记录单词本身（调用方仍会
// 把单词存进数据库，只是释义留空），重试由调用方的退避逻辑负责。
func translateWord(ctx context.Context, wordKey string, cfg deepseekConfig) translateResult {
	senses, isSpellingError, err := lookupDeepSeek(ctx, wordKey, cfg)
	if err != nil {
		log.Printf("deepseek 查词失败 word=%s err=%v", wordKey, err)
		return translateResult{Senses: nil, Source: "none"}
	}
	if isSpellingError {
		return translateResult{Senses: nil, Source: "deepseek", IsSpellingError: true}
	}
	return translateResult{Senses: senses, Source: "deepseek"}
}

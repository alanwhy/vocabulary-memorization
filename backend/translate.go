package main

import (
	"context"
	"log"
)

// translateResult 翻译结果，一个单词可能有多个词性，每个词性对应一条 Sense
type translateResult struct {
	Senses []Sense
	Source string // "deepseek" / "none" 均未获取到
}

// translateWord 调用 DeepSeek 兜底模型查词。查词失败不阻塞记录单词本身（调用方仍会
// 把单词存进数据库，只是释义留空），重试由调用方的退避逻辑负责。
func translateWord(ctx context.Context, wordKey string, cfg deepseekConfig) translateResult {
	senses, err := lookupDeepSeek(ctx, wordKey, cfg)
	if err != nil {
		log.Printf("deepseek 查词失败 word=%s err=%v", wordKey, err)
		return translateResult{Senses: nil, Source: "none"}
	}
	return translateResult{Senses: senses, Source: "deepseek"}
}

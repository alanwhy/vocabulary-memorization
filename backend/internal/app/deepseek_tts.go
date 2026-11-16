// DeepSeek TTS uses binary WAV independently of chat completion and user billing.
package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	ttsTimeout       = 20 * time.Second
	maxTTSAudioBytes = 16 << 20
)

var deepseekTTSHTTPClient = &http.Client{Timeout: ttsTimeout}

// ttsFailure contains a safe public error, never a provider response body or credential.
type ttsFailure struct {
	status  int
	message string
}

// Error implements error with the message safe to show to the caller.
func (e *ttsFailure) Error() string { return e.message }

// ttsEndpoint supports existing base URLs with or without a final /v1.
func ttsEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/tools/tts"
	}
	return base + "/v1/tools/tts"
}

// synthesizeSpeech returns complete WAV or a classified failure; it never retries or writes files.
func synthesizeSpeech(ctx context.Context, text string, cfg ttsConfig) ([]byte, error) {
	if !cfg.complete() {
		return nil, &ttsFailure{503, "语音合成未配置，请在后台配置 DeepSeek"}
	}
	payload, err := json.Marshal(struct {
		Text           string `json:"text"`
		VoiceID        string `json:"voice_id"`
		Format         string `json:"format"`
		FilterMarkdown bool   `json:"filter_markdown"`
	}{text, cfg.VoiceID, "wav", true})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ttsEndpoint(cfg.BaseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, &ttsFailure{502, "语音服务地址无效"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp, err := deepseekTTSHTTPClient.Do(req)
	if err != nil {
		return nil, ttsNetworkFailure(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case 400:
			return nil, &ttsFailure{400, "文本无法合成为语音，请检查内容"}
		case 429:
			return nil, &ttsFailure{429, "语音服务请求过多，请稍后重试"}
		case 503:
			return nil, &ttsFailure{503, "语音服务繁忙，请稍后重试"}
		case 504:
			return nil, &ttsFailure{504, "语音合成超时，请稍后重试"}
		default:
			return nil, &ttsFailure{502, "DeepSeek 语音服务失败，请检查配置或稍后重试"}
		}
	}
	audio, err := io.ReadAll(io.LimitReader(resp.Body, maxTTSAudioBytes+1))
	if err != nil {
		return nil, ttsNetworkFailure(ctx, err)
	}
	if len(audio) > maxTTSAudioBytes || !validWAV(audio) {
		return nil, &ttsFailure{502, "语音服务返回了不完整或无效的音频"}
	}
	return audio, nil
}

// ttsNetworkFailure distinguishes timeout from transport failure and preserves caller cancellation.
func ttsNetworkFailure(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	var timeout interface{ Timeout() bool }
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return &ttsFailure{504, "语音合成超时，请稍后重试"}
	}
	return &ttsFailure{502, "语音合成失败，请稍后重试"}
}

// validWAV checks WAV chunks and data, allowing unknown data lengths used in streamed responses.
func validWAV(audio []byte) bool {
	if len(audio) < 12 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
		return false
	}
	hasFormat := false
	for offset := 12; offset+8 <= len(audio); {
		chunk := string(audio[offset : offset+4])
		size := uint64(binary.LittleEndian.Uint32(audio[offset+4 : offset+8]))
		offset += 8
		remaining := uint64(len(audio) - offset)
		if chunk == "data" {
			if size == 0 || size == 0xffffffff {
				size = remaining
			}
			return hasFormat && size > 0 && size <= remaining
		}
		if size > remaining {
			return false
		}
		if chunk == "fmt " {
			if size < 16 {
				return false
			}
			hasFormat = true
		}
		offset += int(size) + int(size%2)
	}
	return false
}

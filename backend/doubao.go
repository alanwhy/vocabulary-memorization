package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// doubaoTTSEndpoint 火山引擎语音合成的一次性合成接口地址。
const doubaoTTSEndpoint = "https://openspeech.bytedance.com/api/v1/tts"

// doubaoTTSHTTPClient 豆包语音合成用的客户端。合成单个单词很快，但网络抖动要有上限（10s），
// 避免长时间挂起。
var doubaoTTSHTTPClient = &http.Client{Timeout: 10 * time.Second}

// 下面几个结构体对齐火山引擎语音合成接口的请求体字段。
// 新版鉴权走 X-Api-Key 头，请求体 app 里只需 cluster（不再有 appid/token）。
type ttsApp struct {
	Cluster string `json:"cluster"`
}

type ttsUser struct {
	UID string `json:"uid"`
}

type ttsAudio struct {
	VoiceType   string  `json:"voice_type"`
	Encoding    string  `json:"encoding"`
	SpeedRatio  float64 `json:"speed_ratio"`
	VolumeRatio float64 `json:"volume_ratio"`
	PitchRatio  float64 `json:"pitch_ratio"`
}

type ttsReq struct {
	ReqID     string `json:"reqid"`
	Text      string `json:"text"`
	TextType  string `json:"text_type"`
	Operation string `json:"operation"`
}

type ttsRequestBody struct {
	App     ttsApp   `json:"app"`
	User    ttsUser  `json:"user"`
	Audio   ttsAudio `json:"audio"`
	Request ttsReq   `json:"request"`
}

// ttsResponse 火山引擎语音合成响应。Message 故意不给 json tag，让 encoding/json
// 大小写不敏感地同时匹配 message / Message 两种返回形式。
type ttsResponse struct {
	ReqID     string `json:"reqid"`
	Code      int    `json:"code"`
	Message   string
	Operation string `json:"operation"`
	Sequence  int    `json:"sequence"`
	Data      string `json:"data"`
}

// newReqID 生成请求唯一 ID：优先用 crypto/rand（不引入 uuid 依赖），失败时退回纳秒时间戳。
func newReqID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// synthesizeSpeech 调火山引擎语音合成接口，把 text 合成为音频字节（mp3）。
// cfg 由后台配置提供；认证走 X-Api-Key 头。
func synthesizeSpeech(ctx context.Context, text string, cfg ttsConfig) ([]byte, error) {
	body := ttsRequestBody{
		App:     ttsApp{Cluster: cfg.Cluster},
		User:    ttsUser{UID: "vocab"},
		Audio:   ttsAudio{VoiceType: cfg.VoiceType, Encoding: "mp3", SpeedRatio: 1.0, VolumeRatio: 1.0, PitchRatio: 1.0},
		Request: ttsReq{ReqID: newReqID(), Text: text, TextType: "plain", Operation: "query"},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, doubaoTTSEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", cfg.APIKey)

	resp, err := doubaoTTSHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		// 尝试提取火山引擎返回的 message 字段，作为更具体的错误原因透传给前端
		var errResp ttsResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return nil, fmt.Errorf("语音合成失败: %s", errResp.Message)
		}
		return nil, fmt.Errorf("语音合成接口返回状态码 %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseTTSResponse(respBody)
}

// parseTTSResponse 解析火山引擎语音合成响应：code==3000 表示成功，data 是 base64 音频。
// 抽成纯函数便于单测。
func parseTTSResponse(body []byte) ([]byte, error) {
	var resp ttsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析语音合成响应失败: %w", err)
	}
	if resp.Code != 3000 {
		msg := resp.Message
		if msg == "" {
			msg = "未知错误"
		}
		return nil, fmt.Errorf("语音合成失败 code=%d: %s", resp.Code, msg)
	}
	audio, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("解码音频数据失败: %w", err)
	}
	return audio, nil
}

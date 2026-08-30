package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseTTSResponseSuccess(t *testing.T) {
	audio := []byte{0x52, 0x49, 0x46, 0x46} // "RIFF"（wav 文件头）
	body := []byte(`{"reqid":"x","code":3000,"message":"Success","operation":"query","sequence":0,"data":"` +
		base64.StdEncoding.EncodeToString(audio) + `"}`)
	got, err := parseTTSResponse(body)
	if err != nil {
		t.Fatalf("parseTTSResponse 出错: %v", err)
	}
	if string(got) != string(audio) {
		t.Fatalf("got %v, want %v", got, audio)
	}
}

func TestParseTTSResponseCodeFail(t *testing.T) {
	body := []byte(`{"code":3001,"message":"无效的音色"}`)
	_, err := parseTTSResponse(body)
	if err == nil {
		t.Fatalf("expected error for code != 3000")
	}
	if !strings.Contains(err.Error(), "3001") {
		t.Fatalf("error 应包含 code，got %v", err)
	}
}

func TestParseTTSResponseMessageCaseInsensitive(t *testing.T) {
	// Message 字段故意不给 json tag，应同时兼容 message / Message 两种返回
	body := []byte(`{"code":3001,"Message":"voice type not found"}`)
	_, err := parseTTSResponse(body)
	if err == nil || !strings.Contains(err.Error(), "voice type not found") {
		t.Fatalf("error 应包含 Message 内容，got %v", err)
	}
}

func TestParseTTSResponseInvalidJSON(t *testing.T) {
	_, err := parseTTSResponse([]byte(`not json`))
	if err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestParseTTSResponseBadBase64(t *testing.T) {
	body := []byte(`{"code":3000,"data":"!!!not-base64!!!"}`)
	_, err := parseTTSResponse(body)
	if err == nil {
		t.Fatalf("expected error for invalid base64")
	}
}

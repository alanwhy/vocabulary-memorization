package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// sampleWAV provides actual PCM bytes, rather than just a file signature.
func sampleWAV() []byte {
	data := make([]byte, 48)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], 40)
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 24000)
	binary.LittleEndian.PutUint32(data[28:], 48000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 4)
	return data
}

func TestDeepSeekTTSProtocol(t *testing.T) {
	for _, suffix := range []string{"", "/", "/v1/"} {
		t.Run(suffix, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/tools/tts" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("X-Api-Key") != "" {
					t.Errorf("incorrect TTS protocol: %s %s", r.Method, r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body) != 4 || body["text"] != "Hello, world." || body["voice_id"] != "tide" || body["format"] != "wav" || body["filter_markdown"] != true {
					t.Errorf("body=%v", body)
				}
				w.Write(sampleWAV())
			}))
			defer server.Close()
			audio, err := synthesizeSpeech(context.Background(), "Hello, world.", ttsConfig{APIKey: "test-key", BaseURL: server.URL + suffix, VoiceID: "tide"})
			if err != nil || !bytes.Equal(audio, sampleWAV()) {
				t.Fatalf("audio=%v err=%v", audio, err)
			}
		})
	}
}

func TestDeepSeekTTSErrors(t *testing.T) {
	for _, status := range []int{400, 401, 429, 500, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte("private provider message"))
			}))
			defer server.Close()
			_, err := synthesizeSpeech(context.Background(), "apple", ttsConfig{APIKey: "key", BaseURL: server.URL, VoiceID: "stella"})
			var failure *ttsFailure
			want := status
			if status == 401 || status == 500 {
				want = 502
			}
			if !errors.As(err, &failure) || failure.status != want {
				t.Fatalf("err=%v want=%d", err, want)
			}
		})
	}
}

func TestDeepSeekTTSRejectsInvalidAndTruncatedAudio(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("{\"error\":\"not audio\"}"), sampleWAV()[:46]} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
		_, err := synthesizeSpeech(context.Background(), "apple", ttsConfig{APIKey: "key", BaseURL: server.URL, VoiceID: "stella"})
		server.Close()
		if err == nil {
			t.Fatal("accepted incomplete audio")
		}
	}
	streamed := sampleWAV()
	binary.LittleEndian.PutUint32(streamed[4:], 0xffffffff)
	binary.LittleEndian.PutUint32(streamed[40:], 0xffffffff)
	if !validWAV(streamed) {
		t.Fatal("rejected streaming WAV unknown lengths")
	}
}

func TestDeepSeekTTSInterruptedResponseAndTimeout(t *testing.T) {
	interrupted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.Write(sampleWAV())
	}))
	_, err := synthesizeSpeech(context.Background(), "apple", ttsConfig{APIKey: "key", BaseURL: interrupted.URL, VoiceID: "stella"})
	interrupted.Close()
	if err == nil {
		t.Fatal("accepted interrupted response")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = synthesizeSpeech(ctx, "apple", ttsConfig{APIKey: "key", BaseURL: server.URL, VoiceID: "stella"})
	var failure *ttsFailure
	if !errors.As(err, &failure) || failure.status != 504 {
		t.Fatalf("err=%v", err)
	}
}

func TestDeepSeekTTSResponseSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(make([]byte, maxTTSAudioBytes+1)) }))
	defer server.Close()
	_, err := synthesizeSpeech(context.Background(), "apple", ttsConfig{APIKey: "key", BaseURL: server.URL, VoiceID: "stella"})
	if err == nil {
		t.Fatal("accepted oversized response")
	}
}

package training

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestProviderPreservesProtocolAndNullableUsage checks actual HTTP serialization rather than mocking Call itself.
func TestProviderPreservesProtocolAndNullableUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("provider protocol mismatch")
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != "test-model" {
			t.Error("bad payload")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"value\":1}"}}]}`))
	}))
	defer server.Close()
	p := NewProvider(server.URL, "test-key", "test-model", time.Second)
	text, usage, err := p.Call(context.Background(), "system", "data")
	if err != nil || text != "{\"value\":1}" {
		t.Fatalf("%s %v", text, err)
	}
	a, b := usage.Tokens()
	if a != nil || b != nil {
		t.Fatal("missing usage was invented")
	}
}

// TestProviderClassifiesRemoteErrors prevents auth failures triggering an automatic retry loop.
func TestProviderClassifiesRemoteErrors(t *testing.T) {
	for _, status := range []int{401, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte("sensitive upstream content"))
			}))
			defer server.Close()
			_, _, err := NewProvider(server.URL, "test", "model", time.Second).Call(context.Background(), "system", "data")
			remote, ok := err.(*ProviderError)
			if !ok || remote.Retryable != (status != 401) || strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("invalid classification: %v", err)
			}
		})
	}
}

// TestBlindReviewDoesNotReceiveAnswerOrExplanations guards against checking the generated answer by echoing it.
func TestBlindReviewDoesNotReceiveAnswerOrExplanations(t *testing.T) {
	q := sampleQuestion()
	data := ReviewData(q)
	if strings.Contains(data, "correct_option_id") || strings.Contains(data, "解释改成造成") || strings.Contains(data, "mappings") {
		t.Fatal("blind review leaked proposed answer")
	}
}

// TestProviderTimeoutEndsRequest validates a bounded client with a cancellation-aware remote handler.
func TestProviderTimeoutEndsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	_, _, err := NewProvider(server.URL, "test", "model", 20*time.Millisecond).Call(context.Background(), "system", "data")
	if err == nil {
		t.Fatal("timeout accepted")
	}
}

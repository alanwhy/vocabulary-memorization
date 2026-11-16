package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPronounceRejectsInvalidWord ensures the persistent cache cannot accept path input.
func TestPronounceRejectsInvalidWord(t *testing.T) {
	a := New(Dependencies{}, Config{})
	defer a.CancelBackground()
	for _, word := range []string{"../outside", "apple!", "123", ""} {
		r := httptest.NewRequest(http.MethodGet, "/api/pronounce/test", nil)
		r.SetPathValue("wordKey", word)
		w := httptest.NewRecorder()
		a.handlePronounce(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("word %q: status=%d, want 400", word, w.Code)
		}
	}
}

// speechTestApp isolates storage and upstream configuration from local/production audio.
func speechTestApp(t *testing.T, handler http.HandlerFunc) (*App, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); handler(w, r) }))
	a := New(Dependencies{}, Config{})
	a.audioDir = t.TempDir()
	a.ttsConfig = ttsConfig{APIKey: "test-key", BaseURL: server.URL, VoiceID: "stella"}
	t.Cleanup(func() {
		a.CancelBackground()
		if !a.WaitBackground(time.Second) {
			t.Error("TTS did not stop")
		}
		server.Close()
	})
	return a, calls
}

// pronounceRequest invokes the handler with the same canonical path parameter used by the router.
func pronounceRequest(a *App, word string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/api/pronounce/test", nil)
	r.SetPathValue("wordKey", word)
	w := httptest.NewRecorder()
	a.handlePronounce(w, r)
	return w
}

func TestPronounceLegacyAudioIsReadOnlyAndPreferred(t *testing.T) {
	a, calls := speechTestApp(t, func(w http.ResponseWriter, r *http.Request) { w.Write(sampleWAV()) })
	old := []byte("ID3-original-audio")
	path := filepath.Join(a.audioDir, "apple.mp3")
	os.WriteFile(path, old, 0600)
	before, _ := os.Stat(path)
	a.ensureAudioDir()
	os.WriteFile(a.wavPath("apple"), sampleWAV(), 0600)
	a.ttsConfig = ttsConfig{}
	for i := 0; i < 3; i++ {
		a.spawnTTS("apple")
		w := pronounceRequest(a, " APPLE ")
		if w.Code != 200 || w.Header().Get("Content-Type") != "audio/mpeg" || !bytes.Equal(w.Body.Bytes(), old) {
			t.Fatalf("response %d: %q", w.Code, w.Body.String())
		}
	}
	after, _ := os.Stat(path)
	content, _ := os.ReadFile(path)
	if !before.ModTime().Equal(after.ModTime()) || !bytes.Equal(content, old) || calls.Load() != 0 {
		t.Fatal("legacy file modified or provider called")
	}
}

func TestPronouncePersistsAndReusesWAVAcrossInstances(t *testing.T) {
	a, calls := speechTestApp(t, func(w http.ResponseWriter, r *http.Request) { w.Write(sampleWAV()) })
	for i := 0; i < 2; i++ {
		if w := pronounceRequest(a, "apple"); w.Code != 200 || w.Header().Get("Content-Type") != "audio/wav" {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	saved, err := os.ReadFile(a.wavPath("apple"))
	if err != nil || !bytes.Equal(saved, sampleWAV()) || calls.Load() != 1 {
		t.Fatalf("saved=%v calls=%d err=%v", saved, calls.Load(), err)
	}
	another := New(Dependencies{}, Config{})
	defer another.CancelBackground()
	another.audioDir = a.audioDir
	if w := pronounceRequest(another, "apple"); w.Code != 200 {
		t.Fatal("restarted instance missed persisted cache", w.Body.String())
	}
}

func TestPronounceConcurrentBackgroundAndClicksShareJob(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	a, calls := speechTestApp(t, func(w http.ResponseWriter, r *http.Request) { close(started); <-release; w.Write(sampleWAV()) })
	a.spawnTTS("apple")
	<-started
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := pronounceRequest(a, "apple")
			if w.Code != 200 {
				t.Errorf("response=%d %s", w.Code, w.Body.String())
			}
		}()
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
	a.ttsMu.Lock()
	jobs := len(a.ttsJobs)
	a.ttsMu.Unlock()
	if jobs != 0 {
		t.Fatal("completed jobs retained")
	}
	files, _ := os.ReadDir(filepath.Dir(a.wavPath("apple")))
	if len(files) != 1 {
		t.Fatalf("unexpected temporary files: %v", files)
	}
}

func TestPronounceCancelledWaiterStillSaves(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	a, calls := speechTestApp(t, func(w http.ResponseWriter, r *http.Request) { close(started); <-release; w.Write(sampleWAV()) })
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "/api/pronounce/apple", nil).WithContext(ctx)
	r.SetPathValue("wordKey", "apple")
	done := make(chan struct{})
	go func() { a.handlePronounce(httptest.NewRecorder(), r); close(done) }()
	<-started
	cancel()
	<-done
	close(release)
	if w := pronounceRequest(a, "apple"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatal("cancelled waiter duplicated synthesis")
	}
	if _, err := os.Stat(a.wavPath("apple")); err != nil {
		t.Fatal(err)
	}
}

func TestPronounceStorageErrorsDoNotTriggerProvider(t *testing.T) {
	a, calls := speechTestApp(t, func(w http.ResponseWriter, r *http.Request) { w.Write(sampleWAV()) })
	os.Mkdir(filepath.Join(a.audioDir, "apple.mp3"), 0700)
	if w := pronounceRequest(a, "apple"); w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	os.WriteFile(filepath.Join(a.audioDir, "deepseek"), []byte("not a directory"), 0600)
	if w := pronounceRequest(a, "banana"); w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatal("provider called despite storage failure")
	}
}

func TestPronounceBadAudioNeverBecomesCache(t *testing.T) {
	a, calls := speechTestApp(t, func(w http.ResponseWriter, r *http.Request) { w.Write(sampleWAV()[:46]) })
	if w := pronounceRequest(a, "apple"); w.Code != 502 {
		t.Fatal(w.Code, w.Body.String())
	}
	files, _ := os.ReadDir(filepath.Join(a.audioDir, "deepseek"))
	if len(files) != 0 || calls.Load() != 1 {
		t.Fatal("bad audio or temporary file retained")
	}
}

func TestPronounceExampleDoesNotPersistAndReplaysRequest(t *testing.T) {
	text := "She recorded the new words in her notebook."
	a, calls := speechTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["text"] != text {
			t.Errorf("wrong example %v", req)
		}
		w.Write(sampleWAV())
	})
	a.audioDir = filepath.Join(a.audioDir, "must-not-be-created")
	for i := 0; i < 2; i++ {
		payload, _ := json.Marshal(map[string]string{"text": "  " + text + "  "})
		w := httptest.NewRecorder()
		a.handlePronounceExample(w, httptest.NewRequest("POST", "/api/pronounce/example", bytes.NewReader(payload)))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "audio/wav" {
			t.Fatal(w.Code, w.Header(), w.Body.String())
		}
	}
	if calls.Load() != 2 {
		t.Fatal("example unexpectedly cached")
	}
	if _, err := os.Stat(a.audioDir); !os.IsNotExist(err) {
		t.Fatal("example storage created")
	}
}

func TestPronounceExampleRejectsEmptyAndOversizedInput(t *testing.T) {
	a, calls := speechTestApp(t, func(w http.ResponseWriter, r *http.Request) { w.Write(sampleWAV()) })
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"text":"  "}`, 400}, {`{}`, 400}, {`{"text":123}`, 400}, {`{"text":"ok"} {}`, 400},
		{`{"text":"` + strings.Repeat("a", 1001) + `"}`, 413},
		{`{"text":"` + strings.Repeat("汉", 501) + `"}`, 413},
		{`{"text":"` + strings.Repeat("a", 9000) + `"}`, 413},
	} {
		w := httptest.NewRecorder()
		a.handlePronounceExample(w, httptest.NewRequest("POST", "/api/pronounce/example", strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("status=%d want=%d", w.Code, tc.status)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input called provider")
	}
}

func TestTTSShutdownCancelsAndRejectsNewWork(t *testing.T) {
	started := make(chan struct{})
	a, _ := speechTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	})
	a.spawnTTS("apple")
	<-started
	a.CancelBackground()
	if !a.WaitBackground(time.Second) {
		t.Fatal("shutdown failed to cancel TTS")
	}
	if w := pronounceRequest(a, "banana"); w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestPronouncePublishFailureLeavesNoTemporaryOrSuccessfulCache(t *testing.T) {
	var a *App
	a, _ = speechTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		// Simulate the target becoming unwritable after the provider request starts.
		if err := os.Mkdir(a.wavPath("apple"), 0700); err != nil {
			t.Error(err)
		}
		w.Write(sampleWAV())
	})
	if w := pronounceRequest(a, "apple"); w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	files, _ := os.ReadDir(filepath.Join(a.audioDir, "deepseek"))
	for _, file := range files {
		if !file.IsDir() {
			t.Fatalf("partial audio retained: %s", file.Name())
		}
	}
}

func TestPronounceExampleCancellationStopsUpstream(t *testing.T) {
	started, cancelled := make(chan struct{}), make(chan struct{})
	a, _ := speechTestApp(t, func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(cancelled)
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.handlePronounceExample(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/pronounce/example", strings.NewReader(`{"text":"Read this sentence."}`)).WithContext(ctx))
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("example handler did not cancel")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("example upstream did not cancel")
	}
}

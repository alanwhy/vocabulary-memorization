// Pronunciation shares persistent word audio across users. Examples bypass disk storage entirely.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

var audioDir = getEnv("AUDIO_DIR", "audio")

type wordAudio struct {
	data []byte
	mime string
}
type ttsJob struct {
	done  chan struct{}
	audio wordAudio
	err   error
}

// ensureAudioDir prepares the existing volume and the new WAV subdirectory without touching old files.
func (a *App) ensureAudioDir() error {
	return os.MkdirAll(filepath.Join(a.audioDir, "deepseek"), 0o755)
}

// wavPath uses the canonical word's hash inside the original persistent volume.
func (a *App) wavPath(wordKey string) string {
	return filepath.Join(a.audioDir, "deepseek", fmt.Sprintf("%x.wav", sha256.Sum256([]byte(wordKey))))
}

// readWordAudio prefers historical MP3 without requiring credentials; disk errors are not cache misses.
func (a *App) readWordAudio(wordKey string) (wordAudio, bool, error) {
	for _, file := range []struct{ path, mime string }{
		{filepath.Join(a.audioDir, wordKey+".mp3"), "audio/mpeg"},
		{a.wavPath(wordKey), "audio/wav"},
	} {
		data, err := os.ReadFile(file.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return wordAudio{}, false, &ttsFailure{500, "读取单词音频失败，请检查音频存储"}
		}
		if len(data) == 0 || file.mime == "audio/wav" && !validWAV(data) {
			continue
		}
		return wordAudio{data, file.mime}, true, nil
	}
	return wordAudio{}, false, nil
}

// startWordTTS merges background and click requests and removes finished jobs instead of retaining results.
func (a *App) startWordTTS(wordKey string) (*ttsJob, error) {
	if audio, hit, err := a.readWordAudio(wordKey); hit || err != nil {
		if err != nil {
			return nil, err
		}
		job := &ttsJob{done: make(chan struct{}), audio: audio}
		close(job.done)
		return job, nil
	}
	a.ttsMu.Lock()
	defer a.ttsMu.Unlock()
	if a.ttsStopping {
		return nil, &ttsFailure{503, "语音服务正在关闭，请稍后重试"}
	}
	if job := a.ttsJobs[wordKey]; job != nil {
		return job, nil
	}
	job := &ttsJob{done: make(chan struct{})}
	a.ttsJobs[wordKey] = job
	a.ttsWG.Add(1)
	go func() {
		defer a.ttsWG.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				job.err = &ttsFailure{500, "语音生成失败，请稍后重试"}
			}
			a.ttsMu.Lock()
			delete(a.ttsJobs, wordKey)
			close(job.done)
			a.ttsMu.Unlock()
			if job.err != nil {
				log.Printf("单词语音生成失败 word=%s: %v", wordKey, job.err)
			}
		}()
		job.audio, job.err = a.generateWordAudio(wordKey)
	}()
	return job, nil
}

// generateWordAudio rechecks cache, synthesizes once, and atomically publishes a completely written WAV.
func (a *App) generateWordAudio(wordKey string) (wordAudio, error) {
	if audio, hit, err := a.readWordAudio(wordKey); hit || err != nil {
		return audio, err
	}
	if !a.getTTSConfig().complete() {
		return wordAudio{}, &ttsFailure{503, "语音合成未配置，请在后台配置 DeepSeek"}
	}
	if err := a.ensureAudioDir(); err != nil {
		return wordAudio{}, &ttsFailure{500, "保存单词音频失败，请检查音频存储"}
	}
	tmp, err := os.CreateTemp(filepath.Dir(a.wavPath(wordKey)), ".tts-*.tmp")
	if err != nil {
		return wordAudio{}, &ttsFailure{500, "保存单词音频失败，请检查音频存储"}
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	data, err := a.synthesizeTTS(a.bgCtx, wordKey)
	if err != nil {
		return wordAudio{}, err
	}
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if err == nil {
		err = tmp.Close()
	}
	if err != nil {
		return wordAudio{}, &ttsFailure{500, "保存单词音频失败，请检查音频存储"}
	}
	if audio, hit, err := a.readWordAudio(wordKey); hit || err != nil {
		return audio, err
	}
	if err = os.Rename(tmp.Name(), a.wavPath(wordKey)); err != nil {
		return wordAudio{}, &ttsFailure{500, "保存单词音频失败，请检查音频存储"}
	}
	return wordAudio{data, "audio/wav"}, nil
}

// synthesizeTTS limits all upstream speech requests and joins graceful shutdown without caching example audio.
func (a *App) synthesizeTTS(parent context.Context, text string) ([]byte, error) {
	a.ttsMu.Lock()
	if a.ttsStopping {
		a.ttsMu.Unlock()
		return nil, &ttsFailure{503, "语音服务正在关闭，请稍后重试"}
	}
	a.ttsWG.Add(1)
	a.ttsMu.Unlock()
	defer a.ttsWG.Done()
	ctx, cancel := context.WithTimeout(parent, ttsTimeout)
	defer cancel()
	stop := context.AfterFunc(a.bgCtx, cancel)
	defer stop()
	select {
	case a.ttsSem <- struct{}{}:
		defer func() { <-a.ttsSem }()
	case <-ctx.Done():
		return nil, ttsNetworkFailure(ctx, ctx.Err())
	}
	return synthesizeSpeech(ctx, text, a.getTTSConfig())
}

// spawnTTS schedules the same cache-aware job as manual clicks without overwriting existing audio.
func (a *App) spawnTTS(wordKey string) {
	wordKey = strings.ToLower(strings.TrimSpace(wordKey))
	if !wordPattern.MatchString(wordKey) {
		return
	}
	if _, err := a.startWordTTS(wordKey); err != nil {
		log.Printf("预生成语音失败 word=%s: %v", wordKey, err)
	}
}

// handlePronounce returns old/new cached audio or waits on a shared job; waiter cancellation does not cancel saving.
func (a *App) handlePronounce(w http.ResponseWriter, r *http.Request) {
	wordKey := strings.ToLower(strings.TrimSpace(r.PathValue("wordKey")))
	if !wordPattern.MatchString(wordKey) {
		writeError(w, 400, "请输入有效的英文单词（最多 64 字符）")
		return
	}
	job, err := a.startWordTTS(wordKey)
	if err != nil {
		writeTTSFailure(w, err)
		return
	}
	select {
	case <-job.done:
		if job.err != nil {
			writeTTSFailure(w, job.err)
			return
		}
		writeAudio(w, job.audio)
	case <-r.Context().Done():
		writeTTSFailure(w, ttsNetworkFailure(r.Context(), r.Context().Err()))
	}
}

// handlePronounceExample validates bounded plain text and emits no-store WAV, never entering word persistence.
func (a *App) handlePronounceExample(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req struct {
		Text string `json:"text"`
	}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&req)
	if err == nil {
		var extra any
		trailing := decoder.Decode(&extra)
		if trailing != io.EOF {
			if trailing != nil {
				err = trailing
			} else {
				err = errors.New("extra JSON")
			}
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, 413, "例句过长")
			return
		}
		writeError(w, 400, "请求格式不正确")
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, 400, "例句不能为空")
		return
	}
	characters := 0
	for _, character := range text {
		characters++
		if unicode.Is(unicode.Han, character) {
			characters++
		}
	}
	if characters > 1000 {
		writeError(w, 413, "例句过长（最多 1000 个计费字符）")
		return
	}
	data, err := a.synthesizeTTS(r.Context(), text)
	if err != nil {
		writeTTSFailure(w, err)
		return
	}
	writeAudio(w, wordAudio{data, "audio/wav"})
}

// writeTTSFailure uses the existing JSON contract and suppresses errors for a disconnected caller.
func writeTTSFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	var failure *ttsFailure
	if errors.As(err, &failure) {
		writeError(w, failure.status, failure.message)
		return
	}
	writeError(w, 502, "语音合成失败，请稍后重试")
}

// writeAudio keeps stored bytes intact and emits the correct MP3 or WAV MIME.
func writeAudio(w http.ResponseWriter, audio wordAudio) {
	w.Header().Set("Content-Type", audio.mime)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(audio.data); err != nil {
		log.Printf("写入音频响应失败: %v", err)
	}
}

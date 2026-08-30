package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// audioDir 音频文件落盘目录，环境变量 AUDIO_DIR 可配，默认 ./audio（相对后端工作目录）。
// docker 部署时挂载一个 volume 持久化，避免容器重建后音频丢失。
var audioDir = getEnv("AUDIO_DIR", "audio")

// audioPath 返回某个单词发音文件的磁盘路径（mp3 格式）。
// 用 filepath.Base 兜底，防止 wordKey 含路径分隔符导致越界读写。
func audioPath(wordKey string) string {
	return filepath.Join(audioDir, filepath.Base(wordKey)+".mp3")
}

// ensureAudioDir 启动时确保音频目录存在。
func ensureAudioDir() error {
	return os.MkdirAll(audioDir, 0o755)
}

// spawnTTS 后台合成某个单词的发音并落盘：复用 translateSem/translateWG（同 spawnTranslation），
// 失败静默记日志——不阻塞查词，用户点喇叭时 handlePronounce 会再实时合成兜底。
func (a *App) spawnTTS(wordKey string) {
	a.translateWG.Add(1)
	go func() {
		defer a.translateWG.Done()
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("后台合成发音任务发生 panic word=%s: %v", wordKey, rec)
			}
		}()

		select {
		case a.translateSem <- struct{}{}:
		case <-a.bgCtx.Done():
			log.Printf("进程正在关闭，取消尚未开始的合成发音任务 word=%s", wordKey)
			return
		}
		defer func() { <-a.translateSem }()

		a.synthesizeAndSave(a.bgCtx, wordKey)
	}()
}

// synthesizeAndSave 合成发音并落盘；配置不全或合成失败只记日志，不向上抛。
func (a *App) synthesizeAndSave(ctx context.Context, wordKey string) {
	cfg := a.getTTSConfig()
	if !cfg.complete() {
		log.Printf("语音合成未配置，跳过预生成 word=%s", wordKey)
		return
	}
	audio, err := synthesizeSpeech(ctx, wordKey, cfg)
	if err != nil {
		log.Printf("语音合成失败 word=%s: %v", wordKey, err)
		return
	}
	if err := os.WriteFile(audioPath(wordKey), audio, 0o644); err != nil {
		log.Printf("写入音频文件失败 word=%s: %v", wordKey, err)
	}
}

// handlePronounce 播放单词发音（GET /api/pronounce/{wordKey}）。
// 磁盘缓存命中直接返回；未命中则实时合成落盘；未配置或合成失败返回明确错误，
// 前端据此提示用户——豆包语音是唯一读音来源，不做本地合成兜底。
func (a *App) handlePronounce(w http.ResponseWriter, r *http.Request) {
	wordKey := r.PathValue("wordKey")
	if wordKey == "" {
		writeError(w, http.StatusBadRequest, "单词不能为空")
		return
	}

	// 磁盘缓存命中：直接返回音频，不重新合成。
	if audio, err := os.ReadFile(audioPath(wordKey)); err == nil {
		writeAudio(w, audio)
		return
	}

	// 未命中：先检查配置，未配置直接报错（豆包语音是唯一来源，不做本地合成兜底）。
	cfg := a.getTTSConfig()
	if !cfg.complete() {
		writeError(w, http.StatusServiceUnavailable, "语音合成未配置，请在后台管理页配置豆包语音")
		return
	}
	audio, err := synthesizeSpeech(r.Context(), wordKey, cfg)
	if err != nil {
		log.Printf("实时合成发音失败 word=%s: %v", wordKey, err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	// 合成成功落盘，下次命中缓存
	if err := os.WriteFile(audioPath(wordKey), audio, 0o644); err != nil {
		log.Printf("写入音频文件失败 word=%s: %v", wordKey, err)
	}
	writeAudio(w, audio)
}

// writeAudio 以 mp3 的 Content-Type 输出音频字节。
func writeAudio(w http.ResponseWriter, audio []byte) {
	w.Header().Set("Content-Type", "audio/mpeg")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(audio); err != nil {
		log.Printf("写入音频响应失败: %v", err)
	}
}

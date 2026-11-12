// 个人中心母库同步：HTTP 只启动/查询本人任务，后台事务完成后提供结果。
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

const wordSyncTimeout = 5 * time.Minute

type wordSyncStatus struct {
	TaskID     string `json:"task_id,omitempty"`
	Status     string `json:"status"`
	AddedCount int64  `json:"added_count"`
	WordCount  int    `json:"word_count"`
	Message    string `json:"message,omitempty"`
}

// handleStartWordSync 仅为 r 中已登录用户启动后台任务，w 快速返回任务快照。
// 同一用户运行中的任务直接复用；不接受客户端传入的目标用户 ID。
func (a *App) handleStartWordSync(w http.ResponseWriter, r *http.Request) {
	userID := currentUser(r).ID
	a.wordSyncMu.Lock()
	if a.wordSyncStopping || a.bgCtx == nil || a.bgCtx.Err() != nil {
		a.wordSyncMu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "服务正在重启，请稍后重试")
		return
	}
	if job, ok := a.wordSyncJobs[userID]; ok && job.Status == "running" {
		a.wordSyncMu.Unlock()
		writeJSON(w, http.StatusOK, job)
		return
	}
	if a.wordSyncJobs == nil {
		a.wordSyncJobs = make(map[int]wordSyncStatus)
	}
	job := wordSyncStatus{TaskID: randomToken(), Status: "running"}
	a.wordSyncJobs[userID] = job
	a.wordSyncWG.Add(1)
	a.wordSyncMu.Unlock()
	go a.runWordSync(userID, job.TaskID)
	writeJSON(w, http.StatusAccepted, job)
}

// handleWordSyncStatus 将 r 中当前用户的最近同步状态写给 w，不暴露其他账号任务。
func (a *App) handleWordSyncStatus(w http.ResponseWriter, r *http.Request) {
	a.wordSyncMu.Lock()
	job, ok := a.wordSyncJobs[currentUser(r).ID]
	a.wordSyncMu.Unlock()
	if !ok {
		job.Status = "idle"
	}
	writeJSON(w, http.StatusOK, job)
}

// runWordSync 使用进程后台 context 同步 userID；单个 HTTP 断开不会取消任务。
// taskID 标识本次任务；异常、超时或关闭取消会写入失败状态并解除重复启动限制。
func (a *App) runWordSync(userID int, taskID string) {
	defer a.wordSyncWG.Done()
	defer func() {
		if recovered := recover(); recovered != nil {
			a.finishWordSync(userID, taskID, 0, 0, fmt.Errorf("同步任务异常: %v", recovered))
		}
	}()
	ctx, cancel := context.WithTimeout(a.bgCtx, wordSyncTimeout)
	defer cancel()
	added, total, err := a.words.SyncDictionaryToUser(ctx, userID, time.Now())
	a.finishWordSync(userID, taskID, added, total, err)
}

// finishWordSync 保存 userID 的 taskID 结果，added/total 仅成功时生效。
// err 转为可重试的用户提示，底层原因只写日志；旧任务不能覆盖新任务状态。
func (a *App) finishWordSync(userID int, taskID string, added int64, total int, err error) {
	a.wordSyncMu.Lock()
	defer a.wordSyncMu.Unlock()
	job := a.wordSyncJobs[userID]
	if job.TaskID != taskID {
		return
	}
	job.Status = "completed"
	job.AddedCount, job.WordCount = added, total
	if err != nil {
		log.Printf("同步本人单词失败 user_id=%d: %v", userID, err)
		job.Status, job.AddedCount, job.WordCount = "failed", 0, 0
		job.Message = "同步失败，请重新同步"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			job.Message = "同步已中断，请重新同步"
		}
	}
	a.wordSyncJobs[userID] = job
}

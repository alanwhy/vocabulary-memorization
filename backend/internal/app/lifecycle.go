package app

import "time"

const (
	// 查词的完整重试预算约 82 秒；超过 90 秒仍未完成才视为卡死，避免重复触发正常任务。
	stuckTranslationThreshold = 90 * time.Second
	stuckSweepInterval        = 60 * time.Second
)

// Initialize 按原启动顺序加载配置、准备音频目录、恢复未完成任务，再启动后台清理循环。
func (a *App) Initialize() error {
	a.loadSettings()
	if err := ensureAudioDir(); err != nil {
		return err
	}
	a.resumeStuckTranslations()

	go a.startStuckTranslationSweeper()
	go a.loginLimiter.sweep(10*time.Minute, a.bgCtx.Done())
	go a.pwLimiter.sweep(10*time.Minute, a.bgCtx.Done())
	return nil
}

// CancelBackground 先取消后台查词、重试等待和清理循环；HTTP Shutdown 应由组合根随后执行。
func (a *App) CancelBackground() {
	a.bgCancel()
}

// WaitBackground 最多等待 timeout，让已启动的查词和语音任务在数据库关闭前收尾。
func (a *App) WaitBackground(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		a.translateWG.Wait()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

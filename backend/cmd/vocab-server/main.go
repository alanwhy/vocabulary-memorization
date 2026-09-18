// vocab-server 是后端进程的唯一组合根：创建数据库与 repository，注入应用层，
// 再按既定顺序管理 HTTP 服务和后台任务生命周期。业务 Handler 与 SQL 均不放在入口层。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"vocab-backend/internal/app"
	"vocab-backend/internal/storage"
)

const (
	shutdownTimeout   = 15 * time.Second
	bgTaskGracePeriod = 10 * time.Second
)

func main() {
	db := storage.Connect()
	defer db.Close()

	storage.Migrate(db)

	application := app.New(app.Dependencies{
		Users:      storage.NewUserRepository(db),
		Sessions:   storage.NewSessionRepository(db),
		Words:      storage.NewWordRepository(db),
		Dictionary: storage.NewDictionaryRepository(db),
		Settings:   storage.NewSettingsRepository(db),
	}, app.Config{CookieSecure: getEnvBool("COOKIE_SECURE", false)})

	adminID := application.BootstrapAdmin()
	storage.FinalizeWordsUserID(db, adminID)
	if err := application.Initialize(); err != nil {
		log.Fatalf("创建音频目录失败: %v", err)
	}

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      application.Handler("./static"),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("服务启动，监听 %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("收到关闭信号，开始优雅关闭...")

	// 先取消后台等待/重试，再关闭 HTTP；最后给已启动任务一个有限的收尾窗口。
	application.CancelBackground()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("优雅关闭 HTTP 服务失败: %v", err)
	}

	if application.WaitBackground(bgTaskGracePeriod) {
		log.Println("后台查词任务已全部完成")
	} else {
		log.Println("等待后台查词任务超时，放弃剩余任务")
	}

	log.Println("服务已关闭")
}

// getEnvBool 与重构前保持一致：空值或非法布尔值都回退默认值。
func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

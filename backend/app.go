package main

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// Config 收拢原先散落在包级变量里的运行时配置
type Config struct {
	CookieSecure bool
}

// 以下窄接口只声明 handler 实际用到的方法，测试时可以用手写 fake 替换掉真实的
// *xxxRepo，不需要依赖数据库。*userRepo/*sessionRepo/... 天然满足这些接口，
// NewApp 无需任何改动。
type userStore interface {
	Insert(ctx context.Context, username, passwordHash string, isAdmin bool, now time.Time) (User, error)
	FindByUsername(ctx context.Context, username string) (User, string, error)
	FindPasswordHash(ctx context.Context, id int) (string, error)
	UpdatePasswordHash(ctx context.Context, id int, hash string) (int64, error)
	RecordLogin(ctx context.Context, id int, now time.Time) error
	List(ctx context.Context) ([]UserWithStats, error)
	CountAdmins(ctx context.Context) (int, error)
	FirstAdminID(ctx context.Context) (int, error)
}

type sessionStore interface {
	Create(ctx context.Context, token string, userID int, expiresAt, createdAt time.Time) error
	FindWithUser(ctx context.Context, token string) (User, time.Time, error)
	Touch(ctx context.Context, token string, newExpiry time.Time) error
	DeleteByToken(ctx context.Context, token string) error
	DeleteByUser(ctx context.Context, userID int) error
	DeleteByUserExcept(ctx context.Context, userID int, exceptToken string) error
}

type wordStore interface {
	Insert(ctx context.Context, userID int, wordKey, displayWord string, sensesJSON []byte, translating bool, now time.Time) (int, error)
	FindByUserAndKey(ctx context.Context, userID int, wordKey string) (Word, []byte, error)
	IncrementReview(ctx context.Context, id, newCount int, now time.Time) error
	ApplyFlashcardReview(ctx context.Context, id, userID, newCount, intervalDays int, easeFactor float64, dueAt, now time.Time, archived bool) error
	DueFlashcards(ctx context.Context, userID, limit int, now time.Time) ([]Word, error)
	ListPage(ctx context.Context, userID int, archived bool, sort string, limit, offset int) ([]Word, error)
	CountByUser(ctx context.Context, userID int, archived bool) (int, error)
	Delete(ctx context.Context, id, userID int) (int64, error)
	SetArchived(ctx context.Context, id, userID int, archived bool) (int64, error)
	UpdateSenses(ctx context.Context, id int, sensesJSON []byte) error
	MarkTranslationStarted(ctx context.Context, id int, now time.Time) error
	FindTranslating(ctx context.Context) ([]Word, error)
	FindTranslatingStale(ctx context.Context, threshold time.Time) ([]Word, error)
	FindTranslatingByUser(ctx context.Context, userID int) ([]Word, error)
	FindByIDs(ctx context.Context, userID int, ids []int) ([]Word, error)
	Stats(ctx context.Context, userID int, since, since7, todaySince, todayUntil time.Time) (WordStats, error)
}

type dictionaryStore interface {
	UpsertOccurrence(ctx context.Context, wordKey, displayWord string, now time.Time) error
	LookupSenses(ctx context.Context, wordKey string) ([]byte, error)
	SaveSenses(ctx context.Context, wordKey string, sensesJSON []byte) error
	List(ctx context.Context) ([]dictionaryEntry, error)
	ListPage(ctx context.Context, keyword, status string, limit, offset int) ([]dictionaryEntry, error)
	Count(ctx context.Context, keyword, status string) (int, error)
	Delete(ctx context.Context, wordKey string) error
	DeleteMany(ctx context.Context, wordKeys []string) (int64, error)
}

type settingsStore interface {
	SeedIfMissing(ctx context.Context, name, value string) error
	LoadValues(ctx context.Context, names []string) (map[string]string, error)
	UpsertMany(ctx context.Context, updates map[string]string) error
}

// App 持有所有 repository 和运行期状态，取代原先裸露的全局变量；
// handler 从包级函数改为 App 的方法，方便测试时替换成 fake repository。
type App struct {
	users    userStore
	sessions sessionStore
	words    wordStore
	dict     dictionaryStore
	settings settingsStore

	cfg Config

	loginLimiter *attemptTracker
	pwLimiter    *attemptTracker

	settingsMu sync.RWMutex
	dsConfig   deepseekConfig

	// bgCtx 是所有后台任务（后台查词 goroutine）共用的生命周期 context，
	// 进程收到关闭信号时被 cancel，正在等待中的任务据此提前退出。
	bgCtx        context.Context
	bgCancel     context.CancelFunc
	translateSem chan struct{}
	translateWG  sync.WaitGroup
}

func NewApp(db *sql.DB, cfg Config) *App {
	bgCtx, bgCancel := context.WithCancel(context.Background())
	maxConcurrentTranslations := getEnvInt("MAX_CONCURRENT_TRANSLATIONS", 5)
	return &App{
		users:        &userRepo{db: db},
		sessions:     &sessionRepo{db: db},
		words:        &wordRepo{db: db},
		dict:         &dictionaryRepo{db: db},
		settings:     &settingsRepo{db: db},
		cfg:          cfg,
		loginLimiter: newAttemptTracker(15*time.Minute, 5, 15*time.Minute),
		pwLimiter:    newAttemptTracker(15*time.Minute, 5, 15*time.Minute),
		bgCtx:        bgCtx,
		bgCancel:     bgCancel,
		translateSem: make(chan struct{}, maxConcurrentTranslations),
	}
}

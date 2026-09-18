package app

import (
	"context"
	"sync"
	"time"

	"vocab-backend/internal/model"
)

// Config 收拢由组合根传入的运行时配置。
type Config struct {
	CookieSecure bool
}

// UserStore 只声明应用层实际使用的用户持久化能力。
type UserStore interface {
	Insert(ctx context.Context, username, passwordHash string, isAdmin bool, now time.Time) (model.User, error)
	FindByUsername(ctx context.Context, username string) (model.User, string, error)
	FindPasswordHash(ctx context.Context, id int) (string, error)
	UpdatePasswordHash(ctx context.Context, id int, hash string) (int64, error)
	SetDisabled(ctx context.Context, id int, disabled bool) (int64, error)
	Delete(ctx context.Context, id int) (int64, error)
	RecordLogin(ctx context.Context, id int, now time.Time) error
	List(ctx context.Context) ([]model.UserWithStats, error)
	CountAdmins(ctx context.Context) (int, error)
	FirstAdminID(ctx context.Context) (int, error)
}

// SessionStore 只声明应用层实际使用的会话持久化能力。
type SessionStore interface {
	Create(ctx context.Context, token string, userID int, expiresAt, createdAt time.Time) error
	FindWithUser(ctx context.Context, token string) (model.User, time.Time, error)
	Touch(ctx context.Context, token string, newExpiry time.Time) error
	DeleteByToken(ctx context.Context, token string) error
	DeleteByUser(ctx context.Context, userID int) error
	DeleteByUserExcept(ctx context.Context, userID int, exceptToken string) error
}

// WordStore 只声明应用层实际使用的个人单词持久化能力。
type WordStore interface {
	Insert(ctx context.Context, userID int, wordKey, displayWord string, sensesJSON []byte, translating bool, now time.Time) (int, error)
	FindByUserAndKey(ctx context.Context, userID int, wordKey string) (model.Word, []byte, error)
	IncrementReview(ctx context.Context, id, newCount int, now time.Time) error
	ApplyFlashcardReview(ctx context.Context, id, userID, newCount, intervalDays int, easeFactor float64, dueAt, now time.Time, archived bool) error
	DueFlashcards(ctx context.Context, userID, limit int, now time.Time) ([]model.Word, error)
	ListPage(ctx context.Context, userID int, archived bool, keyword, status, sort string, limit, offset int) ([]model.Word, error)
	CountByUser(ctx context.Context, userID int, archived bool, keyword, status string) (int, error)
	ResetReviewCounts(ctx context.Context, userID int) (int64, error)
	Delete(ctx context.Context, id, userID int) (int64, error)
	DeleteByUserID(ctx context.Context, userID int) (int64, error)
	DeleteByWordKey(ctx context.Context, wordKey string) (int64, error)
	DeleteByWordKeys(ctx context.Context, wordKeys []string) (int64, error)
	SetArchived(ctx context.Context, id, userID int, archived bool) (int64, error)
	UpdateSenses(ctx context.Context, id int, sensesJSON []byte) error
	UpdateImportantGlosses(ctx context.Context, id, userID int, glossesJSON []byte) error
	MarkTranslationStarted(ctx context.Context, id int, now time.Time) error
	MarkTranslating(ctx context.Context, id int, now time.Time) error
	FindTranslating(ctx context.Context) ([]model.Word, error)
	FindTranslatingStale(ctx context.Context, threshold time.Time) ([]model.Word, error)
	FindTranslatingByUser(ctx context.Context, userID int) ([]model.Word, error)
	FindByIDs(ctx context.Context, userID int, ids []int) ([]model.Word, error)
	Stats(ctx context.Context, userID int, since, since7, todaySince, todayUntil time.Time) (model.WordStats, error)
}

// DictionaryStore 只声明应用层实际使用的全局词库持久化能力。
type DictionaryStore interface {
	UpsertOccurrence(ctx context.Context, wordKey, displayWord string, now time.Time) error
	LookupSenses(ctx context.Context, wordKey string) ([]byte, error)
	SaveSenses(ctx context.Context, wordKey string, sensesJSON []byte) error
	VocabularyIndex(ctx context.Context) ([]model.VocabularyItem, error)
	List(ctx context.Context) ([]model.DictionaryEntry, error)
	ListPage(ctx context.Context, keyword, status string, limit, offset int) ([]model.DictionaryEntry, error)
	Count(ctx context.Context, keyword, status string) (int, error)
	Delete(ctx context.Context, wordKey string) error
	DeleteMany(ctx context.Context, wordKeys []string) (int64, error)
}

// SettingsStore 只声明应用层实际使用的动态配置持久化能力。
type SettingsStore interface {
	SeedIfMissing(ctx context.Context, name, value string) error
	LoadValues(ctx context.Context, names []string) (map[string]string, error)
	UpsertMany(ctx context.Context, updates map[string]string) error
}

// Dependencies 是组合根注入给应用层的全部持久化依赖。
// 接口由消费方声明，使 app 只依赖领域模型而不依赖具体 MySQL 实现。
type Dependencies struct {
	Users      UserStore
	Sessions   SessionStore
	Words      WordStore
	Dictionary DictionaryStore
	Settings   SettingsStore
}

// App 持有所有 repository 和运行期状态，取代原先裸露的全局变量；
// handler 从包级函数改为 App 的方法，方便测试时替换成 fake repository。
type App struct {
	users    UserStore
	sessions SessionStore
	words    WordStore
	dict     DictionaryStore
	settings SettingsStore

	cfg Config

	loginLimiter *attemptTracker
	pwLimiter    *attemptTracker

	settingsMu   sync.RWMutex
	dsConfig     deepseekConfig
	ttsConfig    ttsConfig
	reviewColors reviewColorConfig

	// bgCtx 是所有后台任务（后台查词 goroutine）共用的生命周期 context，
	// 进程收到关闭信号时被 cancel，正在等待中的任务据此提前退出。
	bgCtx        context.Context
	bgCancel     context.CancelFunc
	translateSem chan struct{}
	translateWG  sync.WaitGroup
}

// New 创建应用实例并初始化共享后台任务上下文；具体 repository 由组合根负责创建。
func New(deps Dependencies, cfg Config) *App {
	bgCtx, bgCancel := context.WithCancel(context.Background())
	maxConcurrentTranslations := getEnvInt("MAX_CONCURRENT_TRANSLATIONS", 5)
	return &App{
		users:        deps.Users,
		sessions:     deps.Sessions,
		words:        deps.Words,
		dict:         deps.Dictionary,
		settings:     deps.Settings,
		cfg:          cfg,
		loginLimiter: newAttemptTracker(15*time.Minute, 5, 15*time.Minute),
		pwLimiter:    newAttemptTracker(15*time.Minute, 5, 15*time.Minute),
		bgCtx:        bgCtx,
		bgCancel:     bgCancel,
		translateSem: make(chan struct{}, maxConcurrentTranslations),
	}
}

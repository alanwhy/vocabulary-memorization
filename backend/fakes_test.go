package main

import (
	"context"
	"database/sql"
	"time"
)

// fakeUserStore 是 userStore 的手写 fake 实现，测试登录/改密码等 handler 时用它替换掉真实数据库
type fakeUserStore struct {
	usersByName map[string]User
	hashes      map[int]string // userID -> password hash
	nextID      int
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{
		usersByName: map[string]User{},
		hashes:      map[int]string{},
		nextID:      1,
	}
}

func (f *fakeUserStore) addUser(username, passwordHash string, isAdmin bool) User {
	u := User{ID: f.nextID, Username: username, IsAdmin: isAdmin, CreatedAt: time.Now()}
	f.usersByName[username] = u
	f.hashes[u.ID] = passwordHash
	f.nextID++
	return u
}

func (f *fakeUserStore) Insert(ctx context.Context, username, passwordHash string, isAdmin bool, now time.Time) (User, error) {
	return f.addUser(username, passwordHash, isAdmin), nil
}

func (f *fakeUserStore) FindByUsername(ctx context.Context, username string) (User, string, error) {
	u, ok := f.usersByName[username]
	if !ok {
		return User{}, "", sql.ErrNoRows
	}
	return u, f.hashes[u.ID], nil
}

func (f *fakeUserStore) FindPasswordHash(ctx context.Context, id int) (string, error) {
	hash, ok := f.hashes[id]
	if !ok {
		return "", sql.ErrNoRows
	}
	return hash, nil
}

func (f *fakeUserStore) UpdatePasswordHash(ctx context.Context, id int, hash string) (int64, error) {
	if _, ok := f.hashes[id]; !ok {
		return 0, nil
	}
	f.hashes[id] = hash
	return 1, nil
}

func (f *fakeUserStore) List(ctx context.Context) ([]User, error) {
	list := make([]User, 0, len(f.usersByName))
	for _, u := range f.usersByName {
		list = append(list, u)
	}
	return list, nil
}

func (f *fakeUserStore) CountAdmins(ctx context.Context) (int, error) {
	count := 0
	for _, u := range f.usersByName {
		if u.IsAdmin {
			count++
		}
	}
	return count, nil
}

func (f *fakeUserStore) FirstAdminID(ctx context.Context) (int, error) {
	for _, u := range f.usersByName {
		if u.IsAdmin {
			return u.ID, nil
		}
	}
	return 0, sql.ErrNoRows
}

type sessionRecord struct {
	userID    int
	expiresAt time.Time
}

// fakeSessionStore 是 sessionStore 的手写 fake 实现；usersByID 由测试直接填充，
// 模拟真实实现里 sessions JOIN users 能查到的用户信息
type fakeSessionStore struct {
	byToken   map[string]sessionRecord
	usersByID map[int]User
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{byToken: map[string]sessionRecord{}, usersByID: map[int]User{}}
}

func (f *fakeSessionStore) Create(ctx context.Context, token string, userID int, expiresAt, createdAt time.Time) error {
	f.byToken[token] = sessionRecord{userID: userID, expiresAt: expiresAt}
	return nil
}

func (f *fakeSessionStore) FindWithUser(ctx context.Context, token string) (User, time.Time, error) {
	rec, ok := f.byToken[token]
	if !ok {
		return User{}, time.Time{}, sql.ErrNoRows
	}
	return f.usersByID[rec.userID], rec.expiresAt, nil
}

func (f *fakeSessionStore) Touch(ctx context.Context, token string, newExpiry time.Time) error {
	rec, ok := f.byToken[token]
	if !ok {
		return sql.ErrNoRows
	}
	rec.expiresAt = newExpiry
	f.byToken[token] = rec
	return nil
}

func (f *fakeSessionStore) DeleteByToken(ctx context.Context, token string) error {
	delete(f.byToken, token)
	return nil
}

func (f *fakeSessionStore) DeleteByUser(ctx context.Context, userID int) error {
	for tok, rec := range f.byToken {
		if rec.userID == userID {
			delete(f.byToken, tok)
		}
	}
	return nil
}

func (f *fakeSessionStore) DeleteByUserExcept(ctx context.Context, userID int, exceptToken string) error {
	for tok, rec := range f.byToken {
		if rec.userID == userID && tok != exceptToken {
			delete(f.byToken, tok)
		}
	}
	return nil
}

// newTestApp 构造一个只装配了 fake 依赖的 App，供 handler 级测试使用，不接触真实数据库
func newTestApp() (*App, *fakeUserStore, *fakeSessionStore) {
	users := newFakeUserStore()
	sessions := newFakeSessionStore()
	app := &App{
		users:        users,
		sessions:     sessions,
		loginLimiter: newAttemptTracker(15*time.Minute, 5, 15*time.Minute),
		pwLimiter:    newAttemptTracker(15*time.Minute, 5, 15*time.Minute),
		cfg:          Config{},
	}
	return app, users, sessions
}

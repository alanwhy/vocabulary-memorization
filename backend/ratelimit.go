package main

import (
	"sync"
	"time"
)

// attemptTracker 是基于内存的滑动窗口失败计数器，用于给登录/改密码接口做基本的
// 暴力破解防护。假设当前是单实例部署，进程重启即清零是可接受的；若未来横向扩容
// 为多实例，需要换成基于数据库或 Redis 的计数方案，内存方案在多实例下无法互相感知。
type attemptTracker struct {
	mu       sync.Mutex
	window   time.Duration
	maxFails int
	lockFor  time.Duration
	failures map[string][]time.Time
	lockedAt map[string]time.Time
}

func newAttemptTracker(window time.Duration, maxFails int, lockFor time.Duration) *attemptTracker {
	return &attemptTracker{
		window:   window,
		maxFails: maxFails,
		lockFor:  lockFor,
		failures: make(map[string][]time.Time),
		lockedAt: make(map[string]time.Time),
	}
}

// Locked 返回 key 当前是否处于锁定期内
func (t *attemptTracker) Locked(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	lockedAt, ok := t.lockedAt[key]
	if !ok {
		return false
	}
	if time.Since(lockedAt) >= t.lockFor {
		delete(t.lockedAt, key)
		delete(t.failures, key)
		return false
	}
	return true
}

// RecordFailure 记录一次失败尝试；在窗口期内失败次数达到阈值则进入锁定
func (t *attemptTracker) RecordFailure(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-t.window)
	fails := t.failures[key]
	kept := fails[:0]
	for _, at := range fails {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	kept = append(kept, now)
	t.failures[key] = kept
	if len(kept) >= t.maxFails {
		t.lockedAt[key] = now
	}
}

// Reset 清空某个 key 的失败记录，用于登录/改密码成功后解除计数
func (t *attemptTracker) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, key)
	delete(t.lockedAt, key)
}

// sweep 周期性清理过期的失败记录和锁定状态，避免 map 无限增长
func (t *attemptTracker) sweep(interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-t.window)
			for key, fails := range t.failures {
				kept := fails[:0]
				for _, at := range fails {
					if at.After(cutoff) {
						kept = append(kept, at)
					}
				}
				if len(kept) == 0 {
					delete(t.failures, key)
				} else {
					t.failures[key] = kept
				}
			}
			for key, lockedAt := range t.lockedAt {
				if now.Sub(lockedAt) >= t.lockFor {
					delete(t.lockedAt, key)
				}
			}
			t.mu.Unlock()
		case <-stop:
			return
		}
	}
}

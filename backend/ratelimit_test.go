package main

import (
	"testing"
	"time"
)

func TestAttemptTrackerLocksAfterThreshold(t *testing.T) {
	tr := newAttemptTracker(time.Minute, 3, time.Minute)
	const key = "user:alice"

	for i := 0; i < 2; i++ {
		tr.RecordFailure(key)
		if tr.Locked(key) {
			t.Fatalf("should not be locked after %d failures", i+1)
		}
	}

	tr.RecordFailure(key)
	if !tr.Locked(key) {
		t.Fatalf("should be locked after reaching threshold")
	}
}

func TestAttemptTrackerResetClearsLock(t *testing.T) {
	tr := newAttemptTracker(time.Minute, 2, time.Minute)
	const key = "user:bob"

	tr.RecordFailure(key)
	tr.RecordFailure(key)
	if !tr.Locked(key) {
		t.Fatalf("expected key to be locked")
	}

	tr.Reset(key)
	if tr.Locked(key) {
		t.Fatalf("expected lock to be cleared after Reset")
	}
}

func TestAttemptTrackerLockExpires(t *testing.T) {
	tr := newAttemptTracker(time.Minute, 1, 10*time.Millisecond)
	const key = "user:carol"

	tr.RecordFailure(key)
	if !tr.Locked(key) {
		t.Fatalf("expected key to be locked immediately after threshold")
	}

	time.Sleep(20 * time.Millisecond)
	if tr.Locked(key) {
		t.Fatalf("expected lock to expire after lockFor duration")
	}
}

func TestAttemptTrackerWindowExpiry(t *testing.T) {
	tr := newAttemptTracker(10*time.Millisecond, 3, time.Minute)
	const key = "user:dave"

	tr.RecordFailure(key)
	tr.RecordFailure(key)
	time.Sleep(20 * time.Millisecond)
	// 前两次失败已经超出窗口期，第三次失败不应该触发锁定
	tr.RecordFailure(key)
	if tr.Locked(key) {
		t.Fatalf("expected old failures outside window to not count toward lock")
	}
}

func TestAttemptTrackerIndependentKeys(t *testing.T) {
	tr := newAttemptTracker(time.Minute, 1, time.Minute)

	tr.RecordFailure("user:eve")
	if tr.Locked("ip:1.2.3.4") {
		t.Fatalf("unrelated key should not be affected")
	}
}

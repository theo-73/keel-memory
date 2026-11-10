package lock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockAcquireAndRelease(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-lock-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	locker := &Locker{
		LockDir: filepath.Join(tmpDir, "locks"),
		TTL:     1 * time.Second,
		Timeout: 50 * time.Millisecond,
	}

	release, err := locker.Acquire("task:test-1")
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	// Immediate second acquire should fail
	_, err = locker.Acquire("task:test-1")
	if err == nil {
		t.Fatal("expected second acquire to fail while locked")
	}

	// Release first lock
	release()

	// Third acquire should succeed now
	release2, err := locker.Acquire("task:test-1")
	if err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
	release2()
}

func TestStaleLockAutoBreak(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-lock-test-stale-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	locker := &Locker{
		LockDir: filepath.Join(tmpDir, "locks"),
		TTL:     50 * time.Millisecond,
	}

	// Acquire lock and don't release it (simulating crash)
	_, err = locker.Acquire("task:crash-sim")
	if err != nil {
		t.Fatal(err)
	}

	// Wait past TTL
	time.Sleep(70 * time.Millisecond)

	// New acquire should detect expiration and break the lock
	release, err := locker.Acquire("task:crash-sim")
	if err != nil {
		t.Fatalf("expected stale lock to break, but got error: %v", err)
	}
	release()
}

func TestEmptyStaleLockAutoBreak(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keel-lock-test-empty-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	locker := &Locker{
		LockDir: filepath.Join(tmpDir, "locks"),
		TTL:     1 * time.Second,
		Timeout: 50 * time.Millisecond,
	}

	// Create 0-byte lock file
	_ = os.MkdirAll(locker.LockDir, 0755)
	emptyLockPath := filepath.Join(locker.LockDir, "empty.lock")
	_ = os.WriteFile(emptyLockPath, []byte(""), 0600)

	// Set ModTime to 5 seconds ago
	past := time.Now().Add(-5 * time.Second)
	_ = os.Chtimes(emptyLockPath, past, past)

	if !isLockStale(emptyLockPath, locker.TTL) {
		t.Errorf("expected 0-byte lock modified in the past to be marked stale")
	}
}

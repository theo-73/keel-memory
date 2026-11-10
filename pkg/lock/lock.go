package lock

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"keel/pkg/platform"
)

const (
	DefaultLockTTL     = 10 * time.Minute
	DefaultLockTimeout = 3 * time.Second
)

// Locker manages safe, deadlock-free locks scoped to a project.
type Locker struct {
	LockDir string
	TTL     time.Duration
	Timeout time.Duration
}

// New creates a new Locker inside `.keel/locks` or specified directory.
func New(projectRoot string) *Locker {
	return &Locker{
		LockDir: filepath.Join(projectRoot, ".keel", "locks"),
		TTL:     DefaultLockTTL,
		Timeout: DefaultLockTimeout,
	}
}

// Acquire attempts to acquire a named lock (e.g. "session:123" or "task:api").
// If a lock file exists, it validates its age and PID. If stale or dead, it breaks the lock automatically.
// Uses an exponential backoff spin-lock until the lock is acquired or timeout expires.
func (l *Locker) Acquire(key string) (func(), error) {
	if err := platform.EnsureDir(l.LockDir); err != nil {
		return nil, fmt.Errorf("ensure lock dir: %w", err)
	}

	hash := sha256.Sum256([]byte(key))
	lockPath := filepath.Join(l.LockDir, hex.EncodeToString(hash[:16])+".lock")

	timeout := l.Timeout
	if timeout <= 0 {
		timeout = DefaultLockTimeout
	}
	deadline := time.Now().Add(timeout)
	backoff := 25 * time.Millisecond
	maxBackoff := 200 * time.Millisecond

	for {
		file, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			// Lock acquired! Record PID and Timestamp
			info := fmt.Sprintf("pid=%d\ncreatedAt=%s\nkey=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano), key)
			_, _ = file.WriteString(info)
			_ = file.Close()

			return func() {
				_ = os.Remove(lockPath)
			}, nil
		}

		// File already exists; inspect for staleness / deadlock
		if isLockStale(lockPath, l.TTL) {
			_ = os.Remove(lockPath)
			continue // Retry acquisition after breaking stale lock
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("resource is locked (key: %s, file: %s)", key, filepath.Base(lockPath))
		}

		time.Sleep(backoff)
		backoff = backoff * 3 / 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func isLockStale(path string, ttl time.Duration) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return true // File truly missing or inaccessible
		}
		if time.Since(info.ModTime()) < 500*time.Millisecond {
			return false // File was modified fractions of a second ago; active creation in progress
		}
		return time.Since(info.ModTime()) > ttl
	}

	lines := strings.Split(string(data), "\n")
	var createdAt time.Time
	var pid int

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "createdAt=") {
			t, err := time.Parse(time.RFC3339Nano, strings.TrimPrefix(line, "createdAt="))
			if err == nil {
				createdAt = t
			}
		} else if strings.HasPrefix(line, "pid=") {
			p, err := strconv.Atoi(strings.TrimPrefix(line, "pid="))
			if err == nil {
				pid = p
			}
		}
	}

	// 1. Fallback for 0-byte or incomplete lock files (crashed before writing info)
	if createdAt.IsZero() || pid == 0 {
		info, statErr := os.Stat(path)
		if statErr == nil {
			if time.Since(info.ModTime()) > 2*time.Second {
				return true // Stale empty or unwritten lock
			}
			return false // In-flight write
		}
		return true
	}

	// 2. Check TTL expiration
	if time.Since(createdAt) > ttl {
		return true // Expired lock
	}

	// 3. Check if PID is still alive on local machine
	if !isProcessAlive(pid) {
		return true // Process is dead, safe to clean up
	}

	return false
}

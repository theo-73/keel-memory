package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NormalizePath cleans and normalizes path separators to forward slashes for cross-platform consistency.
func NormalizePath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// EnsureDir creates directory and parents if they do not exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// AtomicWriteJSON serializes data as indented JSON and writes it atomically to the target path.
func AtomicWriteJSON(path string, data any) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return fmt.Errorf("ensure dir %s: %w", dir, err)
	}

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	bytes = append(bytes, '\n')

	tempFile, err := os.CreateTemp(dir, ".keel-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempName := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempName)
	}()

	if _, err := tempFile.Write(bytes); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

	// Rename atomically replaces existing file on POSIX and Windows (Go 1.5+)
	var renameErr error
	for i := 0; i < 3; i++ {
		renameErr = os.Rename(tempName, path)
		if renameErr == nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("atomic rename to %s: %w", path, renameErr)
}

// ReadJSON reads and deserializes JSON into target.
func ReadJSON[T any](path string) (T, error) {
	var val T
	data, err := os.ReadFile(path)
	if err != nil {
		return val, err
	}
	if err := json.Unmarshal(data, &val); err != nil {
		return val, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return val, nil
}

// IsProjectRelative returns true if the given path is non-empty, relative, and does not escape above root.
func IsProjectRelative(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		return false
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

//go:build !windows

package lock

import (
	"errors"
	"syscall"
)

func isProcessAlive(pid int) bool {
	// Signal 0 checks if process exists without sending a signal
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

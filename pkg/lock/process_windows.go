//go:build windows

package lock

import (
	"os"
	"syscall"
)

const processQueryLimitedInfo = 0x1000

func isProcessAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	const STILL_ACTIVE = 259
	var exitCode uint32

	h, err := syscall.OpenProcess(processQueryLimitedInfo, false, uint32(pid))
	if err != nil {
		if err == syscall.ERROR_ACCESS_DENIED {
			return true // Process is definitely running if access is denied
		}
		h, err = syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
		if err != nil {
			if err == syscall.ERROR_ACCESS_DENIED {
				return true
			}
			return false
		}
	}
	defer syscall.CloseHandle(h)

	err = syscall.GetExitCodeProcess(h, &exitCode)
	if err != nil {
		if err == syscall.ERROR_ACCESS_DENIED {
			return true
		}
		return false
	}
	_ = p
	return exitCode == STILL_ACTIVE
}

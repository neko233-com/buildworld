//go:build windows

package worker

import (
	"golang.org/x/sys/windows"
	"os"
)

func tryFileLock(file *os.File) bool {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{}) == nil
}

func unlockFile(file *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
}

func processSession() string {
	var id uint32
	if windows.ProcessIdToSessionId(uint32(os.Getpid()), &id) != nil {
		return "unavailable"
	}
	return fmtSession(id)
}

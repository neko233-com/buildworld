//go:build !windows

package worker

import (
	"golang.org/x/sys/unix"
	"os"
)

func tryFileLock(file *os.File) bool {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) == nil
}
func unlockFile(file *os.File) { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN) }
func processSession() string   { return "not-applicable" }

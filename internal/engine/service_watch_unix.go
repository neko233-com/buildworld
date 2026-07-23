//go:build !windows

package engine

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

func serviceProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func terminateServiceProcess(pid int, timeout time.Duration) error {
	if !serviceProcessRunning(pid) {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("send SIGTERM: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for serviceProcessRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
	}
	if serviceProcessRunning(pid) {
		return fmt.Errorf("timed out after %s", timeout)
	}
	return nil
}

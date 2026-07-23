//go:build windows

package engine

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

const serviceProcessStillActive = 259 // Win32 STILL_ACTIVE

func serviceProcessRunning(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	return windows.GetExitCodeProcess(handle, &code) == nil && code == serviceProcessStillActive
}

func terminateServiceProcess(pid int, timeout time.Duration) error {
	if !serviceProcessRunning(pid) {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Kill(); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for serviceProcessRunning(pid) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if serviceProcessRunning(pid) {
		return fmt.Errorf("timed out after %s", timeout)
	}
	return nil
}

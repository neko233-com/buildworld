//go:build windows

package engine

import "golang.org/x/sys/windows"

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

//go:build windows

package engine

import "golang.org/x/sys/windows"

func applyProcessBackgroundMode(enabled bool) error {
	priority := uint32(windows.NORMAL_PRIORITY_CLASS)
	if enabled {
		priority = windows.BELOW_NORMAL_PRIORITY_CLASS
	}
	process, err := windows.GetCurrentProcess()
	if err != nil {
		return err
	}
	return windows.SetPriorityClass(process, priority)
}

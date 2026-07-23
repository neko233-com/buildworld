//go:build windows

package api

import "golang.org/x/sys/windows"

func readDiskUsage(path string) (diskUsage, error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return diskUsage{}, err
	}
	var available uint64
	var total uint64
	if err := windows.GetDiskFreeSpaceEx(pathPointer, &available, &total, nil); err != nil {
		return diskUsage{}, err
	}
	return diskUsage{TotalBytes: total, FreeBytes: available}, nil
}

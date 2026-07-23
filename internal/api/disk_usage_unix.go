//go:build linux || darwin

package api

import "golang.org/x/sys/unix"

func readDiskUsage(path string) (diskUsage, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return diskUsage{}, err
	}
	blockSize := uint64(stat.Bsize)
	return diskUsage{
		TotalBytes: uint64(stat.Blocks) * blockSize,
		FreeBytes:  uint64(stat.Bavail) * blockSize,
	}, nil
}

//go:build linux

package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func listProcessIdentities() ([]processIdentity, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	processes := make([]processIdentity, 0)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		executable, err := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		if err != nil {
			continue
		}
		processes = append(processes, processIdentity{
			PID:  pid,
			Name: filepath.Base(strings.TrimSuffix(executable, " (deleted)")),
		})
	}
	return processes, nil
}

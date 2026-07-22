package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func stopProcess(process *os.Process) error { return process.Kill() }

type processIdentity struct {
	PID  int
	Name string
}

func expectedServerProcessName() string {
	name := "buildworld-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func normalizedProcessName(value string) string {
	return filepath.Base(strings.TrimSuffix(strings.TrimSpace(value), " (deleted)"))
}

func matchingBuildWorldServerPIDs(processes []processIdentity, expectedName string, currentPID, recordedPID int) []int {
	target := normalizedProcessName(expectedName)
	seen := make(map[int]struct{})
	matches := make([]int, 0)
	appendMatch := func(process processIdentity) {
		if process.PID <= 0 || process.PID == currentPID {
			return
		}
		if _, exists := seen[process.PID]; exists || !sameProcessName(normalizedProcessName(process.Name), target) {
			return
		}
		seen[process.PID] = struct{}{}
		matches = append(matches, process.PID)
	}
	for _, process := range processes {
		if process.PID == recordedPID {
			appendMatch(process)
		}
	}
	for _, process := range processes {
		appendMatch(process)
	}
	return matches
}

func stopExistingBuildWorldServerProcesses(expectedName string, recordedPID int) (int, error) {
	processes, err := listProcessIdentities()
	if err != nil {
		return 0, fmt.Errorf("list processes: %w", err)
	}
	stopPID := func(pid int) error {
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		stopErr := stopProcess(process)
		_ = process.Release()
		return stopErr
	}
	return stopMatchingBuildWorldServerProcesses(processes, expectedName, os.Getpid(), recordedPID, stopPID, processRunning)
}

func stopMatchingBuildWorldServerProcesses(
	processes []processIdentity,
	expectedName string,
	currentPID int,
	recordedPID int,
	stop func(int) error,
	running func(int) bool,
) (int, error) {
	pids := matchingBuildWorldServerPIDs(processes, expectedName, currentPID, recordedPID)
	stopped := make([]int, 0, len(pids))
	var stopErr error
	for _, pid := range pids {
		if err := stop(pid); err != nil {
			if running(pid) {
				stopErr = errors.Join(stopErr, fmt.Errorf("stop %s process %d: %w", normalizedProcessName(expectedName), pid, err))
			}
			continue
		}
		stopped = append(stopped, pid)
	}

	deadline := time.Now().Add(5 * time.Second)
	for _, pid := range stopped {
		for running(pid) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if running(pid) {
			stopErr = errors.Join(stopErr, fmt.Errorf("%s process %d did not stop", normalizedProcessName(expectedName), pid))
		}
	}
	return len(stopped), stopErr
}

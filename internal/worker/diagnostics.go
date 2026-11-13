package worker

import (
	"os"
	"os/exec"
	"runtime"
)

type processSample struct {
	PID       int    `json:"pid"`
	ParentPID int    `json:"parent_pid"`
	Name      string `json:"name"`
}

// diagnosticSnapshot is read-only and bounded. It never includes process
// arguments or environment; child application output stays in its build log.
func (e *Executor) diagnosticSnapshot() map[string]any {
	stack := make([]byte, 64<<10)
	size := runtime.Stack(stack, true)
	tools := map[string]string{}
	for _, name := range []string{"go", "git", "node", "pwsh", "bash"} {
		if path, err := exec.LookPath(name); err == nil {
			tools[name] = path
		} else {
			tools[name] = "unavailable"
		}
	}
	all, err := hostProcesses()
	owned := []processSample{}
	ids := map[int]bool{os.Getpid(): true}
	for pass := 0; pass < 32; pass++ {
		changed := false
		for _, process := range all {
			if ids[process.ParentPID] && !ids[process.PID] {
				ids[process.PID] = true
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	for _, process := range all {
		if ids[process.PID] && len(owned) < 128 {
			owned = append(owned, process)
		}
	}
	state := "observed"
	if err != nil {
		state = "process enumeration unavailable"
	}
	return map[string]any{"runtime": e.Snapshot(), "go_version": runtime.Version(), "goroutine_stacks": string(stack[:size]), "stacks_truncated": size == len(stack), "owned_processes": owned, "process_sample_state": state, "tools": tools}
}

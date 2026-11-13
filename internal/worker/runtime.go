package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/neko233-com/buildworld/internal/buildinfo"
	"github.com/neko233-com/buildworld/internal/workercontrol"
)

func fmtSession(id uint32) string { return fmt.Sprint(id) }

// Configure changes admission only; reducing capacity never cancels existing work.
func (e *Executor) Configure(capacity int, draining bool) error {
	if capacity < 1 || capacity > 64 {
		return fmt.Errorf("worker capacity must be between 1 and 64")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.capacity, e.draining = capacity, draining
	return nil
}

func (e *Executor) Snapshot() *workercontrol.Runtime {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	executable, _ := os.Executable()
	e.mu.Lock()
	defer e.mu.Unlock()
	tasks := make([]workercontrol.Task, 0, len(e.tasks))
	for _, task := range e.tasks {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return &workercontrol.Runtime{Version: buildinfo.Version, SourceCommit: buildinfo.WorkerReleaseCommit(executable, buildinfo.Version), OS: runtime.GOOS, Arch: runtime.GOARCH, PID: os.Getpid(), Session: processSession(), StartedAt: e.startedAt.Format(time.RFC3339Nano), ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Goroutines: runtime.NumGoroutine(), HeapBytes: memory.HeapAlloc, Capacity: e.capacity, Draining: e.draining, Tasks: tasks}
}

func (e *Executor) track(id, stage, step, state, workspace string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	task, exists := e.tasks[id]
	if !exists {
		return
	}
	task.Stage, task.Step, task.State = stage, step, state
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if workspace != "" {
		task.Workspace = workspace
	}
	e.tasks[id] = task
}

func (e *Executor) executeControl(op *workercontrol.Operation) *workercontrol.Completion {
	result := &workercontrol.Completion{ID: op.ID, Status: "success"}
	snapshot := e.Snapshot()
	switch op.Kind {
	case "diagnose":
		result.Result, _ = json.Marshal(e.diagnosticSnapshot())
		result.Message = "Runtime and active task snapshot captured; environment and command lines excluded"
	case "drain":
		_ = e.Configure(snapshot.Capacity, true)
		result.Message = "New dispatches blocked; existing tasks continue"
	case "resume":
		_ = e.Configure(snapshot.Capacity, false)
		result.Message = "Worker accepts new dispatches"
	case "concurrency":
		if err := e.Configure(op.Capacity, snapshot.Draining); err != nil {
			result.Status, result.Message = "failed", err.Error()
		} else {
			result.Message = "Admission capacity updated; existing tasks preserved"
		}
	default:
		result.Status, result.Message = "failed", "Unsupported worker operation"
	}
	return result
}

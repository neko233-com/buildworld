// Package workercontrol defines the bounded, data-only worker management protocol.
package workercontrol

import "encoding/json"

const MaxReportBytes = 256 << 10

type Task struct {
	ID        string `json:"id"`
	Project   string `json:"project"`
	Stage     string `json:"stage"`
	Step      string `json:"step"`
	State     string `json:"state"`
	Workspace string `json:"workspace"`
	StartedAt string `json:"started_at"`
	UpdatedAt string `json:"updated_at"`
}

// Runtime deliberately excludes environment variables, command lines and credentials.
type Runtime struct {
	Version      string `json:"version"`
	SourceCommit string `json:"source_commit"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	PID          int    `json:"pid"`
	Session      string `json:"session"`
	StartedAt    string `json:"started_at"`
	ObservedAt   string `json:"observed_at"`
	Goroutines   int    `json:"goroutines"`
	HeapBytes    uint64 `json:"heap_bytes"`
	Capacity     int    `json:"capacity"`
	Draining     bool   `json:"draining"`
	Tasks        []Task `json:"tasks"`
}

type Operation struct {
	ID        string          `json:"id"`
	WorkerID  string          `json:"worker_id"`
	RequestID string          `json:"request_id"`
	Kind      string          `json:"kind"`
	Capacity  int             `json:"capacity,omitempty"`
	Version   string          `json:"version,omitempty"`
	SHA256    string          `json:"sha256,omitempty"`
	Platform  string          `json:"platform,omitempty"`
	Status    string          `json:"status"`
	Message   string          `json:"message"`
	Result    json.RawMessage `json:"result,omitempty"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

type Completion struct {
	ID      string          `json:"id"`
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result,omitempty"`
}

type Heartbeat struct {
	Runtime    *Runtime    `json:"runtime,omitempty"`
	Completion *Completion `json:"completion,omitempty"`
}

type Reply struct {
	Status    string     `json:"status"`
	Operation *Operation `json:"operation,omitempty"`
}

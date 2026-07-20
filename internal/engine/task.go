package engine

import "time"

type Task struct {
	ID         string
	ProjectID  int64
	BuildID    int64
	Priority   int
	Status     string
	AssignedTo string
	CreatedAt  time.Time
	StartedAt  time.Time
	FinishedAt time.Time
}

type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusQueued  TaskStatus = "queued"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusSuccess TaskStatus = "success"
	TaskStatusFailed  TaskStatus = "failed"
)

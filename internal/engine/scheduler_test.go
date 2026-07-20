package engine

import (
	"testing"
	"time"
)

func TestSchedulerEnqueue(t *testing.T) {
	scheduler := NewScheduler()

	task := &Task{
		ID:        "build-1",
		ProjectID: 1,
		BuildID:   1,
		Priority:  1,
		CreatedAt: time.Now(),
	}

	err := scheduler.Enqueue(task)
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if scheduler.QueueLen() != 1 {
		t.Errorf("QueueLen = %d, want 1", scheduler.QueueLen())
	}
}

func TestSchedulerDequeue(t *testing.T) {
	scheduler := NewScheduler()

	task := &Task{
		ID:        "build-1",
		ProjectID: 1,
		BuildID:   1,
		Priority:  1,
		CreatedAt: time.Now(),
	}

	scheduler.Enqueue(task)

	dequeued, err := scheduler.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}

	if dequeued.ID != "build-1" {
		t.Errorf("ID = %s, want build-1", dequeued.ID)
	}

	if scheduler.QueueLen() != 0 {
		t.Errorf("QueueLen = %d, want 0", scheduler.QueueLen())
	}
}

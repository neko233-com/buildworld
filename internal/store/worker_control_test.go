package store

import (
	"errors"
	"testing"

	"github.com/neko233-com/buildworld/internal/workercontrol"
)

func TestWorkerControlDrainsWithoutReleasingRunningLeases(t *testing.T) {
	s, cleanup := newTestStore(t)
	defer cleanup()
	_, err := s.CreateWorker("worker-a", "a", "localhost:1", "hash", nil, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.CreateWorker("worker-b", "b", "localhost:2", "hash", nil, 2, "")
	if ok, err := s.TryAcquireWorker("worker-a"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	op, err := s.QueueWorkerOperation("worker-a", workercontrol.Operation{Kind: "drain", RequestID: "stable-1"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.TryAcquireWorker("worker-a"); ok {
		t.Fatal("drained worker was dispatched")
	}
	retry, err := s.QueueWorkerOperation("worker-a", workercontrol.Operation{Kind: "drain", RequestID: "stable-1"})
	if err != nil || retry.ID != op.ID {
		t.Fatal(retry, err)
	}
	if _, err = s.QueueWorkerOperation("worker-a", workercontrol.Operation{Kind: "resume", RequestID: "stable-1"}); !errors.Is(err, ErrWorkerOperationConflict) {
		t.Fatal("conflicting replay accepted", err)
	}
	if _, err = s.ReportWorkerRuntime("worker-b", workercontrol.Heartbeat{Completion: &workercontrol.Completion{ID: op.ID, Status: "success"}}); err == nil {
		t.Fatal("cross-worker receipt accepted")
	}
	next, err := s.ReportWorkerRuntime("worker-a", workercontrol.Heartbeat{Runtime: &workercontrol.Runtime{Capacity: 2, Version: "test"}})
	if err != nil || next.ID != op.ID {
		t.Fatal(next, err)
	}
	worker, _ := s.GetWorker("worker-a")
	if worker.ActiveBuilds != 1 {
		t.Fatal("heartbeat overwrote controller reservation", worker.ActiveBuilds)
	}
	_, err = s.ReportWorkerRuntime("worker-a", workercontrol.Heartbeat{Completion: &workercontrol.Completion{ID: op.ID, Status: "success"}})
	if err != nil {
		t.Fatal(err)
	}
	resume, err := s.QueueWorkerOperation("worker-a", workercontrol.Operation{Kind: "resume", RequestID: "stable-2"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.TryAcquireWorker("worker-a"); ok {
		t.Fatal("queued resume cleared drain before acknowledgment")
	}
	_, err = s.ReportWorkerRuntime("worker-a", workercontrol.Heartbeat{Completion: &workercontrol.Completion{ID: resume.ID, Status: "success"}})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.TryAcquireWorker("worker-a"); !ok {
		t.Fatal("acknowledged resume did not allow dispatch")
	}
	limit, err := s.QueueWorkerOperation("worker-a", workercontrol.Operation{Kind: "concurrency", Capacity: 1, RequestID: "stable-3"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ReportWorkerRuntime("worker-a", workercontrol.Heartbeat{Completion: &workercontrol.Completion{ID: limit.ID, Status: "success"}})
	if err != nil {
		t.Fatal(err)
	}
	worker, _ = s.GetWorker("worker-a")
	if worker.ActiveBuilds != 2 || worker.MaxConcurrentBuilds != 1 {
		t.Fatal("capacity change cancelled or reset existing work", worker)
	}
	if ok, _ := s.TryAcquireWorker("worker-a"); ok {
		t.Fatal("capacity reduction was not enforced")
	}
}

func TestWorkerControlRejectsInvalidAdmissionAndCompletion(t *testing.T) {
	s, cleanup := newTestStore(t)
	defer cleanup()
	_, _ = s.CreateWorker("a", "a", "localhost:1", "hash", nil, 2, "")
	for _, input := range []workercontrol.Operation{{Kind: "shell", RequestID: "x"}, {Kind: "concurrency", Capacity: 0, RequestID: "x"}, {Kind: "concurrency", Capacity: 65, RequestID: "x"}, {Kind: "drain", RequestID: "../x"}, {Kind: "diagnose", RequestID: "x", Capacity: 1}} {
		if _, err := s.QueueWorkerOperation("a", input); err == nil {
			t.Fatalf("invalid input accepted: %+v", input)
		}
	}
	op, _ := s.QueueWorkerOperation("a", workercontrol.Operation{Kind: "diagnose", RequestID: "x"})
	if _, err := s.ReportWorkerRuntime("a", workercontrol.Heartbeat{Completion: &workercontrol.Completion{ID: op.ID, Status: "pending"}}); err == nil {
		t.Fatal("nonterminal completion accepted")
	}
}

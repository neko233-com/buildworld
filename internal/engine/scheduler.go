package engine

import (
	"container/heap"
	"fmt"
	"sync"
)

type Scheduler struct {
	mu    sync.Mutex
	queue taskQueue
}

func NewScheduler() *Scheduler {
	s := &Scheduler{}
	heap.Init(&s.queue)
	return s
}

func (s *Scheduler) Enqueue(task *Task) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	heap.Push(&s.queue, task)
	return nil
}

func (s *Scheduler) Dequeue() (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.queue.Len() == 0 {
		return nil, fmt.Errorf("queue is empty")
	}

	task := heap.Pop(&s.queue).(*Task)
	return task, nil
}

func (s *Scheduler) QueueLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.Len()
}

type taskQueue []*Task

func (q taskQueue) Len() int { return len(q) }

func (q taskQueue) Less(i, j int) bool {
	return q[i].Priority > q[j].Priority
}

func (q taskQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *taskQueue) Push(x interface{}) {
	*q = append(*q, x.(*Task))
}

func (q *taskQueue) Pop() interface{} {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[0 : n-1]
	return item
}

package store

import (
	"errors"
	"testing"
)

func queueFixture(t *testing.T, data *Store, count int) ([]*Build, []*BuildQueueItem) {
	t.Helper()
	project, err := data.CreateProject("queue-order", "", "", "git", "main", `{"stages":[]}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	builds := make([]*Build, 0, count)
	items := make([]*BuildQueueItem, 0, count)
	for index := 0; index < count; index++ {
		build, err := data.CreateBuild(project.ID, index+1, "manual", "main", "", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		item, err := data.CreateBuildQueueItem(build.ID, project.ID, project.Name, 0, build.Trigger, build.Branch)
		if err != nil {
			t.Fatal(err)
		}
		builds = append(builds, build)
		items = append(items, item)
	}
	return builds, items
}

func TestReorderQueuedBuildQueueItemNormalizesStableOrder(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()
	builds, items := queueFixture(t, data, 3)

	position, err := data.ReorderQueuedBuildQueueItem(items[2].ID, func(_, _ int) int { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if position != 1 {
		t.Fatalf("position = %d, want 1", position)
	}
	queue, err := data.ListBuildQueue("")
	if err != nil {
		t.Fatal(err)
	}
	got := []int64{queue[0].BuildID, queue[1].BuildID, queue[2].BuildID}
	want := []int64{builds[2].ID, builds[0].ID, builds[1].ID}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("queue order = %v, want %v", got, want)
		}
		if queue[index].Priority != len(queue)-index || queue[index].QueuePosition != index+1 {
			t.Fatalf("queue[%d] priority/position = %d/%d", index, queue[index].Priority, queue[index].QueuePosition)
		}
	}
}

func TestReorderQueuedBuildQueueItemRejectsRunningBuild(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()
	builds, items := queueFixture(t, data, 2)
	if err := data.StartBuild(builds[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := data.UpdateBuildQueueItemStatus(items[0].ID, "running"); err != nil {
		t.Fatal(err)
	}

	_, err := data.ReorderQueuedBuildQueueItem(items[0].ID, func(current, _ int) int { return current + 1 })
	if !errors.Is(err, ErrBuildQueueItemNotMovable) {
		t.Fatalf("error = %v, want ErrBuildQueueItemNotMovable", err)
	}
	queue, err := data.ListBuildQueue("")
	if err != nil {
		t.Fatal(err)
	}
	var running *BuildQueueItem
	for _, item := range queue {
		if item.ID == items[0].ID {
			running = item
			break
		}
	}
	if running == nil || running.Status != "running" || running.QueuePosition != 0 {
		t.Fatalf("running queue item = %#v, want a non-positioned running item", running)
	}
}

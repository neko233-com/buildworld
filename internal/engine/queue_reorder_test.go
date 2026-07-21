package engine

import (
	"errors"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

type customQueueStrategy struct{}

func (customQueueStrategy) Operation() string        { return "move_middle" }
func (customQueueStrategy) Resolve(_, total int) int { return total / 2 }

func TestBuildQueueReorderRegistryIsExtensible(t *testing.T) {
	registry := NewBuildQueueReorderRegistry([]BuildQueueReorderStrategy{customQueueStrategy{}})
	for _, operation := range []string{"move_up", "move_down", "move_top", "move_bottom", "move_middle"} {
		if _, err := registry.Resolve(operation); err != nil {
			t.Fatalf("operation %q: %v", operation, err)
		}
	}
	if _, err := registry.Resolve("future-operation"); !errors.Is(err, ErrUnknownQueueOperation) {
		t.Fatalf("unknown operation error = %v", err)
	}
}

func TestBuildQueueReorderServiceReturnsVersionedQueue(t *testing.T) {
	data, cleanup := newBuildChainStore(t)
	defer cleanup()
	project, err := data.CreateProject("queue-service", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var last *store.BuildQueueItem
	for number := 1; number <= 3; number++ {
		build, err := data.CreateBuild(project.ID, number, "manual", "main", "", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		last, err = data.CreateBuildQueueItem(build.ID, project.ID, project.Name, 0, build.Trigger, build.Branch)
		if err != nil {
			t.Fatal(err)
		}
	}

	result, err := NewBuildQueueReorderService(data, nil).Apply(last.ID, "move_top")
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != BuildQueueReorderVersion || result.Operation != "move_top" || result.Position != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Items) != 3 || result.Items[0].ID != last.ID {
		t.Fatalf("items = %#v, want moved item first", result.Items)
	}
}

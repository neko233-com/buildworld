package engine

import (
	"fmt"
	"os"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func newBuildChainStore(t *testing.T) (*store.Store, func()) {
	t.Helper()
	file, err := os.CreateTemp("", "build-chain-*.db")
	if err != nil {
		t.Fatal(err)
	}
	name := file.Name()
	file.Close()
	data, err := store.New(name)
	if err != nil {
		os.Remove(name)
		t.Fatal(err)
	}
	return data, func() {
		data.Close()
		os.Remove(name)
	}
}

func TestBuildChainServiceResolvesRetryAndDependencyComponent(t *testing.T) {
	data, cleanup := newBuildChainStore(t)
	defer cleanup()
	project, err := data.CreateProject("chain", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := data.CreateBuild(project.ID, 2, "retry", "main", "", "", nil, &root.ID)
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := data.CreateBuild(project.ID, 3, "finish", "main", "", "", &retry.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuild(project.ID, 4, "manual", "main", "", "", nil, nil); err != nil {
		t.Fatal(err)
	}

	chain, err := NewBuildChainService(data, nil).Resolve(retry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if chain.Version != BuildChainVersion || len(chain.Nodes) != 3 || len(chain.Edges) != 2 {
		t.Fatalf("chain = %#v", chain)
	}
	if len(chain.RootBuildIDs) != 1 || chain.RootBuildIDs[0] != root.ID {
		t.Fatalf("roots = %#v", chain.RootBuildIDs)
	}
	if chain.Edges[0].Type != "retry" || chain.Edges[1].Type != "dependency" {
		t.Fatalf("edges = %#v", chain.Edges)
	}
	if chain.Nodes[1].ID != retry.ID || !chain.Nodes[1].Focus {
		t.Fatalf("focus node = %#v", chain.Nodes[1])
	}
	if chain.Nodes[2].ID != dependent.ID {
		t.Fatalf("dependent node = %#v", chain.Nodes[2])
	}
}

func TestBuildChainRelationRegistryDeduplicatesEdges(t *testing.T) {
	root := &store.Build{ID: 1}
	retry := &store.Build{ID: 2, RetriedFrom: &root.ID}
	builds := map[int64]*store.Build{root.ID: root, retry.ID: retry}
	registry := NewBuildChainRelationRegistry(retryBuildRelationStrategy{}, retryBuildRelationStrategy{})
	if edges := registry.Edges(builds); len(edges) != 1 {
		t.Fatalf("deduplicated edges = %#v", edges)
	}
}

func TestFinishTriggerPersistsSourceBuildRelationship(t *testing.T) {
	data, cleanup := newBuildChainStore(t)
	defer cleanup()
	source, err := data.CreateProject("compile", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	targetConfig := fmt.Sprintf(`import { definePipeline, trigger } from "@buildworld/pipeline"
export default definePipeline({
  stages: [],
  triggers: [trigger("finish", { project_id: "%d" })],
})`, source.ID)
	target, err := data.CreateProject("package", "", "", "git", "main", targetConfig, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sourceBuild, err := data.CreateBuild(source.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sourceBuild.Status = "success"

	NewTriggerChecker(data, nil, nil).HandleBuildFinish(sourceBuild)
	builds, err := data.ListBuildsByProject(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 1 || builds[0].WaitDependencyOn == nil || *builds[0].WaitDependencyOn != sourceBuild.ID {
		t.Fatalf("finish-triggered builds = %#v", builds)
	}
}

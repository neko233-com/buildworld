package engine

import (
	"fmt"
	"sort"

	"github.com/neko233-com/buildworld/internal/store"
)

const BuildChainVersion = 1
const defaultBuildChainLimit = 100

type BuildChainNode struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"project_id"`
	ProjectName string `json:"project_name"`
	Number      int    `json:"number"`
	Status      string `json:"status"`
	Trigger     string `json:"trigger"`
	Branch      string `json:"branch,omitempty"`
	DurationMs  *int64 `json:"duration_ms,omitempty"`
	Pinned      bool   `json:"pinned"`
	Focus       bool   `json:"focus"`
}

type BuildChainEdge struct {
	FromBuildID int64  `json:"from_build_id"`
	ToBuildID   int64  `json:"to_build_id"`
	Type        string `json:"type"`
}

type BuildChain struct {
	Version      int              `json:"version"`
	FocusBuildID int64            `json:"focus_build_id"`
	RootBuildIDs []int64          `json:"root_build_ids"`
	Nodes        []BuildChainNode `json:"nodes"`
	Edges        []BuildChainEdge `json:"edges"`
	Truncated    bool             `json:"truncated"`
}

// BuildChainRelationStrategy is the extension point for new durable build
// relationships. Adding fan-out, promotion, or deployment edges does not
// require changing the graph response schema or UI.
type BuildChainRelationStrategy interface {
	Edges(map[int64]*store.Build) []BuildChainEdge
}

type BuildChainRelationRegistry struct {
	strategies []BuildChainRelationStrategy
}

func NewBuildChainRelationRegistry(strategies ...BuildChainRelationStrategy) *BuildChainRelationRegistry {
	return &BuildChainRelationRegistry{strategies: append([]BuildChainRelationStrategy(nil), strategies...)}
}

func (r *BuildChainRelationRegistry) Edges(builds map[int64]*store.Build) []BuildChainEdge {
	seen := make(map[string]struct{})
	var edges []BuildChainEdge
	for _, strategy := range r.strategies {
		for _, edge := range strategy.Edges(builds) {
			key := fmt.Sprintf("%d:%d:%s", edge.FromBuildID, edge.ToBuildID, edge.Type)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			edges = append(edges, edge)
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].FromBuildID != edges[j].FromBuildID {
			return edges[i].FromBuildID < edges[j].FromBuildID
		}
		if edges[i].ToBuildID != edges[j].ToBuildID {
			return edges[i].ToBuildID < edges[j].ToBuildID
		}
		return edges[i].Type < edges[j].Type
	})
	return edges
}

type retryBuildRelationStrategy struct{}

func (retryBuildRelationStrategy) Edges(builds map[int64]*store.Build) []BuildChainEdge {
	var edges []BuildChainEdge
	for _, build := range builds {
		if build.RetriedFrom == nil || builds[*build.RetriedFrom] == nil {
			continue
		}
		edges = append(edges, BuildChainEdge{
			FromBuildID: *build.RetriedFrom,
			ToBuildID:   build.ID,
			Type:        "retry",
		})
	}
	return edges
}

type dependencyBuildRelationStrategy struct{}

func (dependencyBuildRelationStrategy) Edges(builds map[int64]*store.Build) []BuildChainEdge {
	var edges []BuildChainEdge
	for _, build := range builds {
		if build.WaitDependencyOn == nil || builds[*build.WaitDependencyOn] == nil {
			continue
		}
		edges = append(edges, BuildChainEdge{
			FromBuildID: *build.WaitDependencyOn,
			ToBuildID:   build.ID,
			Type:        "dependency",
		})
	}
	return edges
}

var defaultBuildChainRelationRegistry = NewBuildChainRelationRegistry(
	retryBuildRelationStrategy{},
	dependencyBuildRelationStrategy{},
)

type BuildChainService struct {
	store    *store.Store
	registry *BuildChainRelationRegistry
	limit    int
}

func NewBuildChainService(data *store.Store, registry *BuildChainRelationRegistry) *BuildChainService {
	if registry == nil {
		registry = defaultBuildChainRelationRegistry
	}
	return &BuildChainService{store: data, registry: registry, limit: defaultBuildChainLimit}
}

func (s *BuildChainService) Resolve(focusBuildID int64) (*BuildChain, error) {
	focus, err := s.store.GetBuild(focusBuildID)
	if err != nil {
		return nil, err
	}
	builds := map[int64]*store.Build{focus.ID: focus}
	frontier := []int64{focus.ID}
	truncated := false
	for len(frontier) > 0 {
		neighbors, err := s.store.ListBuildNeighborhood(frontier, s.limit)
		if err != nil {
			return nil, err
		}
		frontier = frontier[:0]
		for _, build := range neighbors {
			if _, exists := builds[build.ID]; exists {
				continue
			}
			if len(builds) >= s.limit {
				truncated = true
				break
			}
			builds[build.ID] = build
			frontier = append(frontier, build.ID)
		}
		if truncated {
			break
		}
	}

	projectNames := make(map[int64]string)
	nodes := make([]BuildChainNode, 0, len(builds))
	for _, build := range builds {
		name, exists := projectNames[build.ProjectID]
		if !exists {
			if project, projectErr := s.store.GetProject(build.ProjectID); projectErr == nil {
				name = project.Name
			}
			projectNames[build.ProjectID] = name
		}
		nodes = append(nodes, BuildChainNode{
			ID: build.ID, ProjectID: build.ProjectID, ProjectName: name,
			Number: build.Number, Status: build.Status, Trigger: build.Trigger,
			Branch: build.Branch, DurationMs: build.DurationMs, Pinned: build.Pinned,
			Focus: build.ID == focusBuildID,
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	edges := s.registry.Edges(builds)
	incoming := make(map[int64]int)
	for _, edge := range edges {
		incoming[edge.ToBuildID]++
	}
	var roots []int64
	for _, node := range nodes {
		if incoming[node.ID] == 0 {
			roots = append(roots, node.ID)
		}
	}
	return &BuildChain{
		Version: BuildChainVersion, FocusBuildID: focusBuildID,
		RootBuildIDs: roots, Nodes: nodes, Edges: edges, Truncated: truncated,
	}, nil
}

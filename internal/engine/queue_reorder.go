package engine

import (
	"errors"
	"fmt"
	"sort"

	"github.com/neko233-com/buildworld/internal/store"
)

const BuildQueueReorderVersion = 1

var ErrUnknownQueueOperation = errors.New("unknown build queue operation")

type BuildQueueReorderStrategy interface {
	Operation() string
	Resolve(current, total int) int
}

type relativeQueueStrategy struct {
	operation string
	offset    int
}

func (s relativeQueueStrategy) Operation() string { return s.operation }
func (s relativeQueueStrategy) Resolve(current, _ int) int {
	return current + s.offset
}

type edgeQueueStrategy struct {
	operation string
	top       bool
}

func (s edgeQueueStrategy) Operation() string { return s.operation }
func (s edgeQueueStrategy) Resolve(_ int, total int) int {
	if s.top {
		return 0
	}
	return total - 1
}

func DefaultBuildQueueReorderStrategies() []BuildQueueReorderStrategy {
	return []BuildQueueReorderStrategy{
		relativeQueueStrategy{operation: "move_up", offset: -1},
		relativeQueueStrategy{operation: "move_down", offset: 1},
		edgeQueueStrategy{operation: "move_top", top: true},
		edgeQueueStrategy{operation: "move_bottom"},
	}
}

type BuildQueueReorderRegistry struct {
	strategies map[string]BuildQueueReorderStrategy
}

func NewBuildQueueReorderRegistry(strategies []BuildQueueReorderStrategy) *BuildQueueReorderRegistry {
	registry := &BuildQueueReorderRegistry{strategies: make(map[string]BuildQueueReorderStrategy)}
	for _, strategy := range DefaultBuildQueueReorderStrategies() {
		registry.Register(strategy)
	}
	for _, strategy := range strategies {
		registry.Register(strategy)
	}
	return registry
}

func (r *BuildQueueReorderRegistry) Register(strategy BuildQueueReorderStrategy) {
	if strategy == nil || strategy.Operation() == "" {
		return
	}
	r.strategies[strategy.Operation()] = strategy
}

func (r *BuildQueueReorderRegistry) Resolve(operation string) (BuildQueueReorderStrategy, error) {
	strategy, ok := r.strategies[operation]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownQueueOperation, operation)
	}
	return strategy, nil
}

func (r *BuildQueueReorderRegistry) Operations() []string {
	operations := make([]string, 0, len(r.strategies))
	for operation := range r.strategies {
		operations = append(operations, operation)
	}
	sort.Strings(operations)
	return operations
}

type BuildQueueReorderResult struct {
	Version   int                     `json:"version"`
	Status    string                  `json:"status"`
	Operation string                  `json:"operation"`
	Position  int                     `json:"position"`
	Items     []*store.BuildQueueItem `json:"items"`
}

type BuildQueueReorderService struct {
	store    *store.Store
	registry *BuildQueueReorderRegistry
}

func NewBuildQueueReorderService(data *store.Store, strategies []BuildQueueReorderStrategy) *BuildQueueReorderService {
	return &BuildQueueReorderService{
		store:    data,
		registry: NewBuildQueueReorderRegistry(strategies),
	}
}

func (s *BuildQueueReorderService) Apply(id int64, operation string) (*BuildQueueReorderResult, error) {
	strategy, err := s.registry.Resolve(operation)
	if err != nil {
		return nil, err
	}
	position, err := s.store.ReorderQueuedBuildQueueItem(id, strategy.Resolve)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListBuildQueue("")
	if err != nil {
		return nil, err
	}
	return &BuildQueueReorderResult{
		Version:   BuildQueueReorderVersion,
		Status:    "ok",
		Operation: operation,
		Position:  position,
		Items:     items,
	}, nil
}

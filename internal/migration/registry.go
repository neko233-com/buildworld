package migration

import (
	"fmt"
	"strings"
	"sync"
)

const ResultVersion = "pipeline-migration/v1"

type Request struct {
	Source string `json:"source"`
	Name   string `json:"name,omitempty"`
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Summary struct {
	StageCount       int `json:"stage_count"`
	EnvironmentCount int `json:"environment_count"`
}

type Hints struct {
	RepositoryURL string `json:"repository_url,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

type Result struct {
	Version      string    `json:"version"`
	SourceFormat string    `json:"source_format"`
	TargetFormat string    `json:"target_format"`
	Config       string    `json:"config"`
	Warnings     []Warning `json:"warnings"`
	Summary      Summary   `json:"summary"`
	Hints        Hints     `json:"hints"`
}

// Strategy converts one external CI definition into Buildworld's native,
// validated pipeline format. New import formats register without changing the
// API or project model.
type Strategy interface {
	SourceFormat() string
	Convert(Request) (*Result, error)
}

type Registry struct {
	mu         sync.RWMutex
	strategies map[string]Strategy
}

func NewRegistry(strategies ...Strategy) *Registry {
	registry := &Registry{strategies: make(map[string]Strategy, len(strategies))}
	for _, strategy := range strategies {
		registry.Register(strategy)
	}
	return registry
}

func NewDefaultRegistry() *Registry {
	return NewRegistry(NewJenkinsfileStrategy())
}

func (r *Registry) Register(strategy Strategy) {
	if r == nil || strategy == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strategies[strings.ToLower(strings.TrimSpace(strategy.SourceFormat()))] = strategy
}

func (r *Registry) Convert(sourceFormat string, request Request) (*Result, error) {
	if r == nil {
		return nil, fmt.Errorf("pipeline migration registry is unavailable")
	}
	r.mu.RLock()
	strategy := r.strategies[strings.ToLower(strings.TrimSpace(sourceFormat))]
	r.mu.RUnlock()
	if strategy == nil {
		return nil, fmt.Errorf("unsupported pipeline source format %q", sourceFormat)
	}
	return strategy.Convert(request)
}

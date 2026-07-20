package portability

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const (
	BundleSchemaVersion = 1
	ProductName         = "buildworld"
)

type ExportOptions struct {
	Sections       []string `json:"sections"`
	IncludeSecrets bool     `json:"include_secrets"`
}

type ImportOptions struct {
	Mode string `json:"mode"`
}

type Section struct {
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data"`
}

type Bundle struct {
	SchemaVersion int                `json:"schema_version"`
	Product       string             `json:"product"`
	ExportedAt    time.Time          `json:"exported_at"`
	Sections      map[string]Section `json:"sections"`
}

type Capability struct {
	Key         string   `json:"key"`
	Version     int      `json:"version"`
	Sensitive   bool     `json:"sensitive"`
	Description string   `json:"description"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

type SectionResult struct {
	Key     string `json:"key"`
	Count   int    `json:"count"`
	Created int    `json:"created,omitempty"`
	Updated int    `json:"updated,omitempty"`
	Skipped int    `json:"skipped,omitempty"`
}

type Inspection struct {
	SchemaVersion int             `json:"schema_version"`
	Product       string          `json:"product"`
	ExportedAt    time.Time       `json:"exported_at"`
	Sections      []SectionResult `json:"sections"`
	Unknown       []string        `json:"unknown_sections,omitempty"`
}

type ImportResult struct {
	Sections []SectionResult `json:"sections"`
	Unknown  []string        `json:"unknown_sections,omitempty"`
}

type Strategy interface {
	Capability() Capability
	Export(context.Context, ExportOptions) (json.RawMessage, error)
	Inspect(json.RawMessage) (int, error)
	Import(context.Context, json.RawMessage, ImportOptions) (SectionResult, error)
}

type Registry struct {
	strategies map[string]Strategy
}

func NewRegistry(strategies ...Strategy) (*Registry, error) {
	registry := &Registry{strategies: make(map[string]Strategy, len(strategies))}
	for _, strategy := range strategies {
		if strategy == nil {
			continue
		}
		capability := strategy.Capability()
		if capability.Key == "" || capability.Version < 1 {
			return nil, fmt.Errorf("invalid portability strategy capability")
		}
		if _, exists := registry.strategies[capability.Key]; exists {
			return nil, fmt.Errorf("duplicate portability strategy %q", capability.Key)
		}
		registry.strategies[capability.Key] = strategy
	}
	return registry, nil
}

func (r *Registry) Capabilities() []Capability {
	result := make([]Capability, 0, len(r.strategies))
	for _, strategy := range r.strategies {
		result = append(result, strategy.Capability())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}

func (r *Registry) Export(ctx context.Context, options ExportOptions) (*Bundle, error) {
	keys := options.Sections
	if len(keys) == 0 {
		for _, capability := range r.Capabilities() {
			if !capability.Sensitive {
				keys = append(keys, capability.Key)
			}
		}
	}
	bundle := &Bundle{
		SchemaVersion: BundleSchemaVersion,
		Product:       ProductName,
		ExportedAt:    time.Now().UTC(),
		Sections:      make(map[string]Section, len(keys)),
	}
	for _, key := range uniqueSorted(keys) {
		strategy, exists := r.strategies[key]
		if !exists {
			return nil, fmt.Errorf("unsupported export section %q", key)
		}
		capability := strategy.Capability()
		if capability.Sensitive && !options.IncludeSecrets {
			return nil, fmt.Errorf("section %q requires include_secrets", key)
		}
		data, err := strategy.Export(ctx, options)
		if err != nil {
			return nil, fmt.Errorf("export %s: %w", key, err)
		}
		bundle.Sections[key] = Section{Version: capability.Version, Data: data}
	}
	return bundle, nil
}

func (r *Registry) Inspect(bundle *Bundle) (*Inspection, error) {
	if err := validateBundle(bundle); err != nil {
		return nil, err
	}
	inspection := &Inspection{
		SchemaVersion: bundle.SchemaVersion,
		Product:       bundle.Product,
		ExportedAt:    bundle.ExportedAt,
	}
	for _, key := range sortedSectionKeys(bundle.Sections) {
		section := bundle.Sections[key]
		strategy, exists := r.strategies[key]
		if !exists {
			inspection.Unknown = append(inspection.Unknown, key)
			continue
		}
		if section.Version > strategy.Capability().Version {
			return nil, fmt.Errorf("section %q version %d is newer than supported version %d", key, section.Version, strategy.Capability().Version)
		}
		count, err := strategy.Inspect(section.Data)
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", key, err)
		}
		inspection.Sections = append(inspection.Sections, SectionResult{Key: key, Count: count})
	}
	return inspection, nil
}

func (r *Registry) Import(ctx context.Context, bundle *Bundle, options ImportOptions) (*ImportResult, error) {
	if options.Mode == "" {
		options.Mode = "skip"
	}
	if options.Mode != "skip" && options.Mode != "overwrite" {
		return nil, fmt.Errorf("import mode must be skip or overwrite")
	}
	if _, err := r.Inspect(bundle); err != nil {
		return nil, err
	}
	result := &ImportResult{}
	keys, err := r.importOrder(bundle.Sections)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		section := bundle.Sections[key]
		strategy := r.strategies[key]
		sectionResult, err := strategy.Import(ctx, section.Data, options)
		if err != nil {
			return nil, fmt.Errorf("import %s: %w", key, err)
		}
		sectionResult.Key = key
		result.Sections = append(result.Sections, sectionResult)
	}
	for _, key := range sortedSectionKeys(bundle.Sections) {
		if _, exists := r.strategies[key]; !exists {
			result.Unknown = append(result.Unknown, key)
		}
	}
	return result, nil
}

// importOrder returns a deterministic topological order for known sections.
// Dependencies are applied only when both sections are present, so users can
// import a focused bundle into a system that already has its prerequisites.
func (r *Registry) importOrder(sections map[string]Section) ([]string, error) {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(sections))
	result := make([]string, 0, len(sections))
	var visit func(string) error
	visit = func(key string) error {
		strategy, known := r.strategies[key]
		if !known {
			return nil
		}
		switch state[key] {
		case visited:
			return nil
		case visiting:
			return fmt.Errorf("portability dependency cycle involving %q", key)
		}
		state[key] = visiting
		dependencies := append([]string(nil), strategy.Capability().DependsOn...)
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if _, present := sections[dependency]; !present {
				continue
			}
			if _, known := r.strategies[dependency]; !known {
				continue
			}
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[key] = visited
		result = append(result, key)
		return nil
	}
	for _, key := range sortedSectionKeys(sections) {
		if err := visit(key); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func validateBundle(bundle *Bundle) error {
	if bundle == nil {
		return fmt.Errorf("bundle is required")
	}
	if bundle.Product != ProductName {
		return fmt.Errorf("unsupported bundle product %q", bundle.Product)
	}
	if bundle.SchemaVersion < 1 || bundle.SchemaVersion > BundleSchemaVersion {
		return fmt.Errorf("unsupported bundle schema version %d", bundle.SchemaVersion)
	}
	if len(bundle.Sections) == 0 {
		return fmt.Errorf("bundle has no sections")
	}
	return nil
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedSectionKeys(sections map[string]Section) []string {
	result := make([]string, 0, len(sections))
	for key := range sections {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

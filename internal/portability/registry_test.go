package portability

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type memoryStrategy struct {
	key      string
	version  int
	depends  []string
	imported int
	order    *[]string
}

func (s *memoryStrategy) Capability() Capability {
	return Capability{Key: s.key, Version: s.version, DependsOn: s.depends}
}
func (s *memoryStrategy) Export(context.Context, ExportOptions) (json.RawMessage, error) {
	return json.RawMessage(`[{"name":"demo"}]`), nil
}
func (s *memoryStrategy) Inspect(data json.RawMessage) (int, error) {
	var values []map[string]any
	err := json.Unmarshal(data, &values)
	return len(values), err
}
func (s *memoryStrategy) Import(_ context.Context, data json.RawMessage, _ ImportOptions) (SectionResult, error) {
	count, err := s.Inspect(data)
	s.imported += count
	if s.order != nil {
		*s.order = append(*s.order, s.key)
	}
	return SectionResult{Count: count, Created: count}, err
}

func TestRegistrySupportsVersionedIndependentStrategies(t *testing.T) {
	projects := &memoryStrategy{key: "projects", version: 2}
	registry, err := NewRegistry(projects)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := registry.Export(context.Background(), ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.SchemaVersion != BundleSchemaVersion || bundle.Sections["projects"].Version != 2 {
		t.Fatalf("unexpected bundle metadata: %#v", bundle)
	}
	bundle.Sections["future"] = Section{Version: 1, Data: json.RawMessage(`[]`)}
	inspection, err := registry.Inspect(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Unknown) != 1 || inspection.Unknown[0] != "future" {
		t.Fatalf("unknown sections = %#v", inspection.Unknown)
	}
	result, err := registry.Import(context.Background(), bundle, ImportOptions{Mode: "skip"})
	if err != nil {
		t.Fatal(err)
	}
	if projects.imported != 1 || len(result.Unknown) != 1 {
		t.Fatalf("import result = %#v, imported=%d", result, projects.imported)
	}
}

func TestRegistryRejectsNewerKnownSectionVersions(t *testing.T) {
	registry, err := NewRegistry(&memoryStrategy{key: "projects", version: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Inspect(&Bundle{
		SchemaVersion: 1,
		Product:       ProductName,
		Sections: map[string]Section{
			"projects": {Version: 2, Data: json.RawMessage(`[]`)},
		},
	})
	if err == nil {
		t.Fatal("expected newer section version to be rejected")
	}
}

func TestRegistryImportsDependenciesBeforeConsumers(t *testing.T) {
	var order []string
	registry, err := NewRegistry(
		&memoryStrategy{key: "projects", version: 1, depends: []string{"vcs_roots"}, order: &order},
		&memoryStrategy{key: "credentials", version: 1, order: &order},
		&memoryStrategy{key: "vcs_roots", version: 1, depends: []string{"credentials"}, order: &order},
	)
	if err != nil {
		t.Fatal(err)
	}
	bundle := &Bundle{
		SchemaVersion: 1,
		Product:       ProductName,
		Sections: map[string]Section{
			"projects":    {Version: 1, Data: json.RawMessage(`[]`)},
			"credentials": {Version: 1, Data: json.RawMessage(`[]`)},
			"vcs_roots":   {Version: 1, Data: json.RawMessage(`[]`)},
		},
	}
	if _, err := registry.Import(context.Background(), bundle, ImportOptions{Mode: "skip"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(order, ","); got != "credentials,vcs_roots,projects" {
		t.Fatalf("import order = %q, want credentials,vcs_roots,projects", got)
	}
}

func TestRegistryRejectsDependencyCycles(t *testing.T) {
	registry, err := NewRegistry(
		&memoryStrategy{key: "first", version: 1, depends: []string{"second"}},
		&memoryStrategy{key: "second", version: 1, depends: []string{"first"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Import(context.Background(), &Bundle{
		SchemaVersion: 1,
		Product:       ProductName,
		Sections: map[string]Section{
			"first":  {Version: 1, Data: json.RawMessage(`[]`)},
			"second": {Version: 1, Data: json.RawMessage(`[]`)},
		},
	}, ImportOptions{Mode: "skip"})
	if err == nil {
		t.Fatal("expected dependency cycle to fail")
	}
}

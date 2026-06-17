package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginLoader(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
  "name": "test-plugin",
  "version": "1.0.0",
  "description": "A test plugin"
}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(pluginDir, "index.js"), []byte(`
function onLoad() {
  return "Plugin loaded!";
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)

	if err := loader.Load("test-plugin"); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	plugin := loader.Get("test-plugin")
	if plugin == nil {
		t.Fatal("Get() returned nil")
	}

	if plugin.Name != "test-plugin" {
		t.Errorf("Name = %s, want test-plugin", plugin.Name)
	}

	if plugin.Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", plugin.Version)
	}
}

func TestPluginList(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	for _, name := range []string{"plugin-a", "plugin-b"} {
		dir := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{
  "name": "`+name+`",
  "version": "1.0.0",
  "description": "Test"
}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.js"), []byte(`
function onLoad() {
  return "ok";
}
`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	loader := NewLoader(tmpDir)
	if err := loader.Load("plugin-a"); err != nil {
		t.Fatal(err)
	}
	if err := loader.Load("plugin-b"); err != nil {
		t.Fatal(err)
	}

	list := loader.List()
	if len(list) != 2 {
		t.Errorf("List() length = %d, want 2", len(list))
	}
}

func TestPluginLoadNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	loader := NewLoader(tmpDir)
	err = loader.Load("nonexistent-plugin")
	if err == nil {
		t.Error("Load() should return error for non-existent plugin")
	}
}

func TestPluginLoadInvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pluginDir := filepath.Join(tmpDir, "bad-json")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{invalid json`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "index.js"), []byte(`function onLoad() { return "ok"; }`), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	err = loader.Load("bad-json")
	if err == nil {
		t.Error("Load() should return error for invalid JSON")
	}
}

func TestPluginLoadMissingScript(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pluginDir := filepath.Join(tmpDir, "no-script")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
  "name": "no-script",
  "version": "1.0.0",
  "description": "Missing script"
}`), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	err = loader.Load("no-script")
	if err == nil {
		t.Error("Load() should return error when index.js is missing")
	}
}

func TestPluginUnloadNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	loader := NewLoader(tmpDir)
	err = loader.Unload("nonexistent")
	if err != nil {
		t.Errorf("Unload() should not error for non-existent plugin, got: %v", err)
	}
}

func TestPluginLoadMultiple(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	names := []string{"alpha", "beta", "gamma"}
	for _, name := range names {
		dir := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		meta := `{"name":"` + name + `","version":"1.0.0","description":"Plugin ` + name + `"}`
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(meta), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.js"), []byte(`function onLoad() { return "ok"; }`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	loader := NewLoader(tmpDir)
	for _, name := range names {
		if err := loader.Load(name); err != nil {
			t.Fatalf("Load(%s) error = %v", name, err)
		}
	}

	list := loader.List()
	if len(list) != 3 {
		t.Errorf("List() length = %d, want 3", len(list))
	}

	for _, name := range names {
		p := loader.Get(name)
		if p == nil {
			t.Errorf("Get(%s) returned nil", name)
		} else if p.Name != name {
			t.Errorf("Get(%s).Name = %s", name, p.Name)
		}
	}
}

func TestPluginMetadataParsing(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pluginDir := filepath.Join(tmpDir, "meta-test")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
  "name": "meta-test",
  "version": "2.3.1",
  "description": "A plugin for testing metadata"
}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "index.js"), []byte(`function onLoad() { return "ok"; }`), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	if err := loader.Load("meta-test"); err != nil {
		t.Fatal(err)
	}

	plugin := loader.Get("meta-test")
	if plugin == nil {
		t.Fatal("Get() returned nil")
	}

	tests := []struct {
		field string
		got   string
		want  string
	}{
		{"Name", plugin.Name, "meta-test"},
		{"Version", plugin.Version, "2.3.1"},
		{"Description", plugin.Description, "A plugin for testing metadata"},
		{"Path", plugin.Path, filepath.Join(tmpDir, "meta-test")},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.field, tt.got, tt.want)
		}
	}
}

func TestPluginUnload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pluginDir := filepath.Join(tmpDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
  "name": "test-plugin",
  "version": "1.0.0",
  "description": "A test plugin"
}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "index.js"), []byte(`
function onLoad() {
  return "ok";
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(tmpDir)
	if err := loader.Load("test-plugin"); err != nil {
		t.Fatal(err)
	}

	if err := loader.Unload("test-plugin"); err != nil {
		t.Fatalf("Unload() error = %v", err)
	}

	if loader.Get("test-plugin") != nil {
		t.Error("Get() should return nil after Unload()")
	}
}

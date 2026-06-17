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

	if err := os.WriteFile(filepath.Join(pluginDir, "index.lua"), []byte(`
function onLoad()
  return "Plugin loaded!"
end
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
		if err := os.WriteFile(filepath.Join(dir, "index.lua"), []byte(`
function onLoad()
  return "ok"
end
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
	if err := os.WriteFile(filepath.Join(pluginDir, "index.lua"), []byte(`
function onLoad()
  return "ok"
end
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

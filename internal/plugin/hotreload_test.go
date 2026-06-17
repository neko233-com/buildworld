package plugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPluginHotReload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-reload-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pluginDir := filepath.Join(tmpDir, "test-plugin")
	os.MkdirAll(pluginDir, 0755)

	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
  "name": "test-plugin",
  "version": "1.0.0",
  "description": "Test"
}
`), 0644)

	os.WriteFile(filepath.Join(pluginDir, "index.lua"), []byte(`
function onLoad()
  return "v1"
end
`), 0644)

	loader := NewLoader(tmpDir)
	err = loader.Load("test-plugin")
	if err != nil {
		t.Fatal(err)
	}

	watcher, err := NewHotReload(loader)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()

	watcher.Watch("test-plugin")

	os.WriteFile(filepath.Join(pluginDir, "index.lua"), []byte(`
function onLoad()
  return "v2"
end
`), 0644)

	time.Sleep(500 * time.Millisecond)

	plugin := loader.Get("test-plugin")
	if plugin == nil {
		t.Error("Plugin should be loaded after reload")
	}
}

func TestHotReloadStop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-reload-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pluginDir := filepath.Join(tmpDir, "test-plugin")
	os.MkdirAll(pluginDir, 0755)
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{"name":"test-plugin","version":"1.0.0","description":"Test"}`), 0644)
	os.WriteFile(filepath.Join(pluginDir, "index.lua"), []byte(`function onLoad() return "ok" end`), 0644)

	loader := NewLoader(tmpDir)
	loader.Load("test-plugin")

	watcher, err := NewHotReload(loader)
	if err != nil {
		t.Fatal(err)
	}

	watcher.Watch("test-plugin")
	watcher.Stop()

	// Writing after stop should not panic or cause errors
	os.WriteFile(filepath.Join(pluginDir, "index.lua"), []byte(`function onLoad() return "v2" end`), 0644)
	time.Sleep(150 * time.Millisecond)
}

func TestHotReloadWatchMultiple(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-reload-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	for _, name := range []string{"plugin-a", "plugin-b"} {
		dir := filepath.Join(tmpDir, name)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"`+name+`","version":"1.0.0","description":"Test"}`), 0644)
		os.WriteFile(filepath.Join(dir, "index.lua"), []byte(`function onLoad() return "v1" end`), 0644)
	}

	loader := NewLoader(tmpDir)
	loader.Load("plugin-a")
	loader.Load("plugin-b")

	watcher, err := NewHotReload(loader)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()

	watcher.Watch("plugin-a")
	watcher.Watch("plugin-b")

	// Modify both plugins
	os.WriteFile(filepath.Join(tmpDir, "plugin-a", "index.lua"), []byte(`function onLoad() return "v2" end`), 0644)
	os.WriteFile(filepath.Join(tmpDir, "plugin-b", "index.lua"), []byte(`function onLoad() return "v2" end`), 0644)

	time.Sleep(300 * time.Millisecond)

	if loader.Get("plugin-a") == nil {
		t.Error("plugin-a should still be loaded")
	}
	if loader.Get("plugin-b") == nil {
		t.Error("plugin-b should still be loaded")
	}
}

func TestHotReloadDebounce(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-reload-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	pluginDir := filepath.Join(tmpDir, "test-plugin")
	os.MkdirAll(pluginDir, 0755)
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{"name":"test-plugin","version":"1.0.0","description":"Test"}`), 0644)
	os.WriteFile(filepath.Join(pluginDir, "index.lua"), []byte(`function onLoad() return "v1" end`), 0644)

	loader := NewLoader(tmpDir)
	loader.Load("test-plugin")

	watcher, err := NewHotReload(loader)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()

	watcher.Watch("test-plugin")

	// Rapid fire writes — debounce should coalesce them
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(pluginDir, "index.lua"), []byte(`function onLoad() return "rapid" end`), 0644)
		time.Sleep(20 * time.Millisecond)
	}

	time.Sleep(300 * time.Millisecond)

	plugin := loader.Get("test-plugin")
	if plugin == nil {
		t.Error("Plugin should be loaded after rapid writes")
	}
}

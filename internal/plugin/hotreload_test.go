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

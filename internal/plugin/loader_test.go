package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeBinaryTestPlugin(t *testing.T, root, name, version string, steps ...string) string {
	t.Helper()
	pluginDir := filepath.Join(root, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entrypoint := "plugin"
	entryFile := entrypoint
	if runtime.GOOS == "windows" {
		entryFile += ".exe"
	}
	if err := os.WriteFile(filepath.Join(pluginDir, entryFile), []byte("placeholder"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := BinaryManifest{
		APIVersion: BinaryAPIVersion,
		Name:       name,
		Version:    version,
		Entrypoint: entrypoint,
		Steps:      steps,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin-buildworld.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return pluginDir
}

func TestLoaderLoadsOnlyBinaryManifest(t *testing.T) {
	root := t.TempDir()
	writeBinaryTestPlugin(t, root, "binary", "1.2.3", "deploy")
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "plugin.json"), []byte(`{"name":"legacy"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "index.js"), []byte(`registerStep("legacy", () => {})`), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(root)
	if err := loader.LoadAll(); err != nil {
		t.Fatal(err)
	}
	loaded := loader.Get("binary")
	if loaded == nil || loaded.Name != "binary" || loaded.Version != "1.2.3" {
		t.Fatalf("loaded binary plugin = %#v", loaded)
	}
	if loader.Get("legacy") != nil {
		t.Fatal("legacy JavaScript plugin was loaded")
	}
	if err := loader.Load("legacy"); err == nil {
		t.Fatal("Load accepted a directory without plugin-buildworld.json")
	}
}

func TestLoaderEnabledListReloadAndUnload(t *testing.T) {
	root := t.TempDir()
	writeBinaryTestPlugin(t, root, "alpha", "1.0.0", "alpha-step")
	writeBinaryTestPlugin(t, root, "beta", "1.0.0", "beta-step")
	loader := NewLoader(root)
	if err := loader.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if len(loader.List()) != 2 || len(loader.ListAll()) != 2 {
		t.Fatalf("initial plugin lists = enabled %d, all %d", len(loader.List()), len(loader.ListAll()))
	}
	loader.SetEnabled("alpha", false)
	if loader.Get("alpha") != nil || loader.LookupStep("alpha-step") != nil || len(loader.List()) != 1 {
		t.Fatal("disabled plugin remains executable")
	}
	if err := loader.Reload("alpha"); err != nil {
		t.Fatal(err)
	}
	if loader.IsEnabled("alpha") || loader.Get("alpha") != nil {
		t.Fatal("reload changed disabled state")
	}
	loader.SetEnabled("alpha", true)
	if loader.Get("alpha") == nil || loader.LookupStep("alpha-step") == nil {
		t.Fatal("enabled plugin is unavailable")
	}
	if err := loader.Unload("alpha"); err != nil {
		t.Fatal(err)
	}
	if loader.getLoaded("alpha") != nil {
		t.Fatal("unloaded plugin remains registered")
	}
}

func TestLoaderRejectsUnsafeNamesAndManifestMismatch(t *testing.T) {
	root := t.TempDir()
	loader := NewLoader(root)
	for _, name := range []string{"../escape", `bad/name`, `bad\\name`, ""} {
		if err := loader.Load(name); err == nil {
			t.Fatalf("Load(%q) accepted an unsafe name", name)
		}
	}
	pluginDir := writeBinaryTestPlugin(t, root, "directory-name", "1.0.0", "step")
	data, err := os.ReadFile(filepath.Join(pluginDir, "plugin-buildworld.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest BinaryManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Name = "different-name"
	data, _ = json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin-buildworld.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := loader.Load("directory-name"); err == nil {
		t.Fatal("Load accepted a manifest/directory name mismatch")
	}
}

func TestLoaderRejectsDuplicateStepOwnership(t *testing.T) {
	root := t.TempDir()
	writeBinaryTestPlugin(t, root, "alpha", "1.0.0", "shared:step")
	writeBinaryTestPlugin(t, root, "beta", "1.0.0", "shared:step")
	loader := NewLoader(root)
	if err := loader.Load("alpha"); err != nil {
		t.Fatal(err)
	}
	if err := loader.Load("beta"); err == nil {
		t.Fatal("second plugin claimed an already registered step type")
	}
	if loader.GetInstalled("beta") != nil {
		t.Fatal("conflicting plugin was registered")
	}
}

func TestDeletePluginStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	pluginDir := writeBinaryTestPlugin(t, root, "remove-me", "1.0.0", "step")
	loader := NewLoader(root)
	if err := loader.Load("remove-me"); err != nil {
		t.Fatal(err)
	}
	if err := loader.DeletePlugin("../outside"); err == nil {
		t.Fatal("DeletePlugin accepted traversal")
	}
	if _, err := os.Stat(pluginDir); err != nil {
		t.Fatalf("unsafe delete touched valid plugin: %v", err)
	}
	if err := loader.DeletePlugin("remove-me"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Fatalf("plugin directory still exists: %v", err)
	}
}

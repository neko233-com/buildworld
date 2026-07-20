package cli

import (
	"path/filepath"
	"testing"
)

func TestEnsureConfigCreatesSelfContainedDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "buildworld.yaml")
	written, cfg, err := ensureConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if written != path {
		t.Fatalf("config path = %q, want %q", written, path)
	}
	if cfg.Server.Port != 8700 {
		t.Fatalf("default port = %d, want 8700", cfg.Server.Port)
	}
	if cfg.Database.Path == "" || cfg.Storage.BuildTemp == "" || cfg.Plugins.Path == "" {
		t.Fatalf("default config is incomplete: %#v", cfg)
	}
	_, reloaded, err := ensureConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Database.Path != cfg.Database.Path {
		t.Fatalf("existing config was not preserved: %q != %q", reloaded.Database.Path, cfg.Database.Path)
	}
}

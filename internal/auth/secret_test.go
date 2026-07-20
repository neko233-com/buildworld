package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateJWTSecretPersistsAcrossRestarts(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "buildworld.db")

	first, path, generated, err := LoadOrCreateJWTSecret("", databasePath)
	if err != nil {
		t.Fatalf("first LoadOrCreateJWTSecret() error = %v", err)
	}
	if !generated || len(first) != 64 {
		t.Fatalf("first secret generated=%v len=%d, want generated 64-char secret", generated, len(first))
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		t.Fatalf("persisted secret stat = %v, info=%v", err, info)
	}

	second, secondPath, generated, err := LoadOrCreateJWTSecret("", databasePath)
	if err != nil {
		t.Fatalf("second LoadOrCreateJWTSecret() error = %v", err)
	}
	if generated || secondPath != path || second != first {
		t.Fatalf("second secret was not reused: generated=%v path=%q", generated, secondPath)
	}
}

func TestLoadOrCreateJWTSecretPrefersConfiguredValue(t *testing.T) {
	secret, path, generated, err := LoadOrCreateJWTSecret(" configured-secret ", filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatalf("LoadOrCreateJWTSecret() error = %v", err)
	}
	if secret != "configured-secret" || path != "" || generated {
		t.Fatalf("result = (%q, %q, %v), want configured value", secret, path, generated)
	}
}

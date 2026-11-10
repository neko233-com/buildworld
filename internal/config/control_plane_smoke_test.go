//go:build buildworld_smoke

package config

import "testing"

func TestSmokeControlPlaneUsesIsolatedPort(t *testing.T) {
	t.Setenv("BUILDWORLD_SMOKE_PORT", "54321")
	cfg := &Config{}
	cfg.EnforceControlPlanePort()
	if cfg.Server.Port != 54321 {
		t.Fatalf("smoke control-plane port = %d, want 54321", cfg.Server.Port)
	}
}

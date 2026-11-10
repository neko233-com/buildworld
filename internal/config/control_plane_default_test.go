//go:build !buildworld_smoke

package config

import "testing"

func TestProductionControlPlaneIgnoresSmokePort(t *testing.T) {
	t.Setenv("BUILDWORLD_SMOKE_PORT", "54321")
	cfg := &Config{}
	cfg.EnforceControlPlanePort()
	if cfg.Server.Port != ControlPlanePort {
		t.Fatalf("production control-plane port = %d, want %d", cfg.Server.Port, ControlPlanePort)
	}
}

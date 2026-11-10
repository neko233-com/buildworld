//go:build !buildworld_smoke

package config

func effectiveControlPlanePort() int { return ControlPlanePort }

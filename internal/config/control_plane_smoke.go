//go:build buildworld_smoke

package config

import (
	"os"
	"strconv"
)

// Smoke binaries can bind an isolated loopback port without altering release binaries.
func effectiveControlPlanePort() int {
	port, err := strconv.Atoi(os.Getenv("BUILDWORLD_SMOKE_PORT"))
	if err == nil && port > 0 && port <= 65535 {
		return port
	}
	return ControlPlanePort
}

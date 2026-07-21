//go:build !windows && !linux && !darwin

package cli

import "fmt"

func enableAutostart(_, _ string) error {
	return fmt.Errorf("autostart is supported on Windows, macOS, and Linux")
}
func disableAutostart() error              { return nil }
func stopAutostartService() (bool, error)  { return false, nil }
func startAutostartService() (bool, error) { return false, nil }

//go:build !darwin && !linux

package systemupdate

import "fmt"

func launchDetached(_ string, _ []string, _ string) error {
	return fmt.Errorf("system update is unsupported on this platform")
}

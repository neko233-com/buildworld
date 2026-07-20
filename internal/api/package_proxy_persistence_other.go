//go:build !windows

package api

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func persistUserEnvironment(values map[string]string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	directory := filepath.Join(home, ".config", "environment.d")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create user environment directory: %w", err)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var output strings.Builder
	output.WriteString("# Managed by BuildWorld package proxy settings.\n")
	for _, key := range keys {
		output.WriteString(key + "=" + strings.ReplaceAll(values[key], "\n", "") + "\n")
	}
	return os.WriteFile(filepath.Join(directory, "90-buildworld-package-proxy.conf"), []byte(output.String()), 0o600)
}

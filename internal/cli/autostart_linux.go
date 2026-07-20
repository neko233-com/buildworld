//go:build linux

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const systemdServiceName = "buildworld.service"

func systemdEscape(value string) string { return strings.ReplaceAll(value, " ", "\\x20") }

func enableAutostart(server, config string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	directory := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	contents := fmt.Sprintf("[Unit]\nDescription=BuildWorld CI/CD server\nAfter=network-online.target\n\n[Service]\nType=simple\nExecStart=%s -config %s\nRestart=on-failure\nRestartSec=3\n\n[Install]\nWantedBy=default.target\n", systemdEscape(server), systemdEscape(config))
	if err := os.WriteFile(filepath.Join(directory, systemdServiceName), []byte(contents), 0o600); err != nil {
		return fmt.Errorf("write systemd user service: %w", err)
	}
	if output, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("reload systemd user services: %w: %s", err, output)
	}
	if output, err := exec.Command("systemctl", "--user", "enable", "--now", systemdServiceName).CombinedOutput(); err != nil {
		return fmt.Errorf("enable BuildWorld autostart: %w: %s", err, output)
	}
	return nil
}

func disableAutostart() error {
	_ = exec.Command("systemctl", "--user", "disable", "--now", systemdServiceName).Run()
	return nil
}

func stopAutostartService() (bool, error) {
	if err := exec.Command("systemctl", "--user", "stop", systemdServiceName).Run(); err != nil {
		return false, nil
	}
	return true, nil
}

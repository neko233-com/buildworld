//go:build windows

package cli

import (
	"fmt"
	"os/exec"
)

const autostartTaskName = "BuildWorld Server"

func enableAutostart(server, config string) error {
	command := fmt.Sprintf("\\\"%s\\\" -config \\\"%s\\\"", server, config)
	if output, err := exec.Command("schtasks", "/Create", "/TN", autostartTaskName, "/SC", "ONLOGON", "/TR", command, "/RL", "LIMITED", "/F").CombinedOutput(); err != nil {
		return fmt.Errorf("create Windows autostart task: %w: %s", err, output)
	}
	return nil
}

func disableAutostart() error {
	output, err := exec.Command("schtasks", "/Delete", "/TN", autostartTaskName, "/F").CombinedOutput()
	if err != nil && len(output) > 0 {
		return fmt.Errorf("remove Windows autostart task: %w: %s", err, output)
	}
	return nil
}

func stopAutostartService() (bool, error) {
	if err := exec.Command("schtasks", "/End", "/TN", autostartTaskName).Run(); err != nil {
		return false, nil
	}
	return true, nil
}

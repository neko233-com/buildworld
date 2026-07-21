//go:build windows

package cli

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const autostartTaskName = "BuildWorld Server"
const autostartScriptName = "autostart.ps1"
const schtasksActionLimit = 262

func enableAutostart(server, config string) error {
	script := filepath.Join(filepath.Dir(config), autostartScriptName)
	if err := os.WriteFile(script, windowsAutostartScript(server, config), 0o600); err != nil {
		return fmt.Errorf("write Windows autostart launcher: %w", err)
	}
	command, err := windowsAutostartCommand(script)
	if err != nil {
		return err
	}
	if output, err := exec.Command("schtasks", "/Create", "/TN", autostartTaskName, "/SC", "ONLOGON", "/TR", command, "/RL", "LIMITED", "/F").CombinedOutput(); err != nil {
		return fmt.Errorf("create Windows autostart task: %w: %s", err, output)
	}
	return nil
}

func windowsAutostartScript(server, config string) []byte {
	logPath := filepath.Join(filepath.Dir(config), "server.log")
	command := fmt.Sprintf(
		"$ErrorActionPreference = 'Stop'\r\n& %s -config %s >> %s 2>&1\r\nexit $LASTEXITCODE\r\n",
		powerShellLiteral(server),
		powerShellLiteral(config),
		powerShellLiteral(logPath),
	)
	encoded := utf16.Encode([]rune(command))
	bytes := make([]byte, 2+len(encoded)*2)
	binary.LittleEndian.PutUint16(bytes, 0xfeff)
	for index, value := range encoded {
		binary.LittleEndian.PutUint16(bytes[2+index*2:], value)
	}
	return bytes
}

func windowsAutostartCommand(script string) (string, error) {
	if strings.Contains(script, `"`) {
		return "", fmt.Errorf("Windows autostart launcher path contains a quote: %s", script)
	}
	command := `powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -WindowStyle Hidden -File "` + script + `"`
	if utf8.RuneCountInString(command) > schtasksActionLimit {
		return "", fmt.Errorf("Windows autostart command exceeds schtasks limit of %d characters", schtasksActionLimit)
	}
	return command, nil
}

func powerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
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

func startAutostartService() (bool, error) {
	if err := exec.Command("schtasks", "/Query", "/TN", autostartTaskName).Run(); err != nil {
		return false, nil
	}
	if output, err := exec.Command("schtasks", "/Run", "/TN", autostartTaskName).CombinedOutput(); err != nil {
		return false, fmt.Errorf("start Windows autostart task: %w: %s", err, output)
	}
	return true, nil
}

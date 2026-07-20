//go:build darwin

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const launchAgentName = "com.buildworld.server"

func plistValue(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(value)
}

func enableAutostart(server, config string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	directory := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	path := filepath.Join(directory, launchAgentName+".plist")
	stateDir := filepath.Dir(config)
	contents := fmt.Sprintf("<?xml version=\"1.0\" encoding=\"UTF-8\"?><!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\"><plist version=\"1.0\"><dict><key>Label</key><string>%s</string><key>ProgramArguments</key><array><string>%s</string><string>-config</string><string>%s</string></array><key>WorkingDirectory</key><string>%s</string><key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string><key>RunAtLoad</key><true/><key>KeepAlive</key><true/></dict></plist>", launchAgentName, plistValue(server), plistValue(config), plistValue(stateDir), plistValue(filepath.Join(stateDir, "server.log")), plistValue(filepath.Join(stateDir, "server.log")))
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return err
	}
	uid := strconv.Itoa(os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid, path).Run()
	if output, err := exec.Command("launchctl", "bootstrap", "gui/"+uid, path).CombinedOutput(); err != nil {
		return fmt.Errorf("load macOS LaunchAgent: %w: %s", err, output)
	}
	return nil
}

func disableAutostart() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", launchAgentName+".plist")
	_ = exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), path).Run()
	return os.Remove(path)
}

func stopAutostartService() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", launchAgentName+".plist")
	if err := exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid()), path).Run(); err != nil {
		return false, nil
	}
	return true, nil
}

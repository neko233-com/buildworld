package cli

import (
	"fmt"
	"path"
	"strings"
)

const launchAgentName = "com.buildworld.server"
const macOSAutostartLauncherName = "autostart.sh"

type macOSAutostartPaths struct {
	plist    string
	launcher string
}

func macOSAutostartPathsForHome(home string) macOSAutostartPaths {
	return macOSAutostartPaths{
		plist:    path.Join(home, "Library", "LaunchAgents", launchAgentName+".plist"),
		launcher: path.Join(home, "Library", "Application Support", "buildworld", macOSAutostartLauncherName),
	}
}

// macOSLaunchAgentStopResult distinguishes an absent agent from one that
// launchctl cannot unload, so the caller never kills a supervised process
// while KeepAlive is still active.
func macOSLaunchAgentStopResult(bootErr error, bootOutput string, listing []byte, listErr error) (bool, error) {
	if bootErr == nil {
		return true, nil
	}
	if listErr != nil {
		return false, fmt.Errorf("cannot inspect macOS LaunchAgents after bootout failed: %w", listErr)
	}
	for _, line := range strings.Split(string(listing), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[len(fields)-1] == launchAgentName {
			return false, fmt.Errorf("cannot unload active macOS LaunchAgent %s: %w: %s", launchAgentName, bootErr, strings.TrimSpace(bootOutput))
		}
	}
	return false, nil
}

func macOSShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func macOSAutostartLauncher(server, config, workingDirectory string) []byte {
	return []byte(fmt.Sprintf(
		"#!/bin/sh\nset -eu\numask 077\ncd %s\nexec %s -config %s\n",
		macOSShellLiteral(workingDirectory),
		macOSShellLiteral(server),
		macOSShellLiteral(config),
	))
}

func plistValue(value string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(value)
}

func macOSLaunchAgentPlist(launcher, workingDirectory, logPath string) []byte {
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/sh</string>
    <string>%s</string>
  </array>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
</dict>
</plist>
`, launchAgentName, plistValue(launcher), plistValue(workingDirectory), plistValue(logPath), plistValue(logPath)))
}

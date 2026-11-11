package systemupdate

import (
	"os"
	"strconv"
	"strings"
)

const systemSupervisorPath = "/usr/local/libexec/buildworld-system-supervisor"
const systemSupervisorSHA256 = "3b8e7e8841bf5dcdc4a058b2978162912ea53913f91c6ab9ad560db03a9aa15c"

func trustedRootFile(mode os.FileMode, uid uint32) bool {
	return uid == 0 && mode.IsRegular() && mode.Perm()&0o022 == 0
}

func supportedSystemSupervisorPlist(currentUser, daemonUser, runAtLoad, keepAlive string) bool {
	return currentUser != "" && currentUser == daemonUser && runAtLoad == "true" && keepAlive == "true"
}

// systemLaunchDaemonMatches requires both the system domain and the current
// server PID, so an unrelated user service with the same label is unaffected.
func systemLaunchDaemonMatches(output string, pid int) bool {
	system, samePID := false, false
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"`)
		value = strings.Trim(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), ";")), `"`)
		switch key {
		case "LimitLoadToSessionType":
			system = value == "System"
		case "PID":
			parsed, err := strconv.Atoi(value)
			samePID = err == nil && pid > 0 && parsed == pid
		}
	}
	return system && samePID
}

// systemLaunchDaemonPrintMatches ties the privileged status to this process.
func systemLaunchDaemonPrintMatches(output string, pid int) bool {
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && strings.TrimSpace(key) == "pid" {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			return err == nil && pid > 0 && parsed == pid
		}
	}
	return false
}

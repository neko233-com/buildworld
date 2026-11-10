package systemupdate

import (
	"strconv"
	"strings"
)

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

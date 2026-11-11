package systemupdate

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSystemSupervisorBridgeMatchesTrustedScript(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "scripts", "buildworld-system-supervisor.sh"))
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != systemSupervisorSHA256 {
		t.Fatalf("supervisor bridge digest mismatch: %v", err)
	}
	helper, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "scripts", "apply-update.sh"))
	if err != nil || !strings.Contains(string(helper), systemSupervisorSHA256) {
		t.Fatalf("update helper bridge digest mismatch: %v", err)
	}
	if !systemLaunchDaemonPrintMatches("system/com.buildworld.server = {\n\tpid = 65118\n}", 65118) || systemLaunchDaemonPrintMatches("pid = 65119", 65118) || systemLaunchDaemonPrintMatches("pid = 0", 0) {
		t.Fatal("system supervisor status must identify the current server")
	}
}

func TestSystemSupervisorRejectsUnsafeFilesAndPlists(t *testing.T) {
	if !trustedRootFile(0o755, 0) || trustedRootFile(0o775, 0) || trustedRootFile(0o755, 501) || trustedRootFile(os.ModeSymlink|0o755, 0) {
		t.Fatal("unsafe root-owned file was accepted")
	}
	if !supportedSystemSupervisorPlist("gamer", "gamer", "true", "true") ||
		supportedSystemSupervisorPlist("gamer", "root", "true", "true") ||
		supportedSystemSupervisorPlist("gamer", "gamer", "false", "true") ||
		supportedSystemSupervisorPlist("gamer", "gamer", "true", "false") ||
		supportedSystemSupervisorPlist("", "", "true", "true") {
		t.Fatal("unsafe system service plist was accepted")
	}
}

func TestSystemLaunchDaemonMatchesCurrentServer(t *testing.T) {
	const system = `{
    "LimitLoadToSessionType" = "System";
    "PID" = 65118;
};`
	const user = `{
    "LimitLoadToSessionType" = "Aqua";
    "PID" = 65118;
};`
	for _, tc := range []struct {
		name, output string
		pid          int
		want         bool
	}{
		{"current system daemon", system, 65118, true},
		{"different process", system, 65119, false},
		{"user agent", user, 65118, false},
		{"missing PID", `"LimitLoadToSessionType" = "System";`, 65118, false},
		{"invalid PID", `"PID" = 0;`, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := systemLaunchDaemonMatches(tc.output, tc.pid); got != tc.want {
				t.Fatalf("systemLaunchDaemonMatches = %t, want %t", got, tc.want)
			}
		})
	}
}

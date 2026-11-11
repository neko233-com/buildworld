package systemupdate

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSystemSupervisorBridgeLimitsActions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the bridge requires a POSIX shell")
	}
	_, source, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "scripts", "buildworld-system-supervisor.sh"))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "space ' fixture")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	launchctl := filepath.Join(root, "launchctl")
	writeTestFile(t, launchctl, "#!/bin/sh\nprintf '%s\\n' \"$*\"\n", 0o700)
	if !strings.Contains(string(data), "/bin/launchctl") {
		t.Fatal("bridge command missing")
	}
	bridge := filepath.Join(root, "bridge")
	writeTestFile(t, bridge, strings.ReplaceAll(string(data), "/bin/launchctl", shellFixturePath(launchctl)), 0o700)
	for _, tc := range []struct{ action, want string }{
		{"status", "print system/com.buildworld.server"},
		{"stop", "bootout system/com.buildworld.server"},
		{"start", "bootstrap system /Library/LaunchDaemons/com.buildworld.server.plist"},
	} {
		cmd := exec.Command("sh", bridge)
		cmd.Stdin = strings.NewReader(tc.action + "\n")
		output, err := cmd.CombinedOutput()
		if err != nil || strings.TrimSpace(string(output)) != tc.want {
			t.Fatalf("%s: output=%q err=%v", tc.action, output, err)
		}
	}
	for _, action := range []string{"", "restart", "status; id", "start /tmp/other"} {
		cmd := exec.Command("sh", bridge)
		cmd.Stdin = strings.NewReader(action + "\n")
		if output, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("unsafe action %q accepted: %s", action, output)
		}
	}
	cmd := exec.Command("sh", bridge, "extra")
	cmd.Stdin = strings.NewReader("status\n")
	if err := cmd.Run(); err == nil {
		t.Fatal("bridge accepted extra arguments")
	}
}

// TestSystemSupervisorUpdate uses the real update helper with isolated command fixtures.
func TestSystemSupervisorUpdate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the update helper requires a POSIX shell")
	}
	for _, tc := range []struct {
		name, failure, status, current, previous string
	}{
		{"success", "", "succeeded", "new", "current"},
		{"cannot stop", "stop", "rolled_back", "current", "older"},
		{"new start fails", "start", "rolled_back", "current", ""},
		{"migration advanced", "migration-advanced", "failed", "new", "current"},
		{"bridge tampered", "bridge-tampered", "failed", "current", "older"},
		{"bridge missing", "bridge-missing", "failed", "current", "older"},
		{"bridge permission denied", "bridge-denied", "failed", "current", "older"},
		{"wrong owner", "wrong-owner", "failed", "current", "older"},
		{"wrong process", "wrong-process", "failed", "current", "older"},
		{"unhealthy controller", "unhealthy-before", "failed", "current", "older"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "space ' fixture")
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			install, previous := filepath.Join(root, "install"), filepath.Join(root, "install.previous")
			home, bin, operation := filepath.Join(root, "home"), filepath.Join(root, "bin"), filepath.Join(root, "operation")
			for _, dir := range []string{install, previous, home, bin, operation} {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			writeTestFile(t, filepath.Join(install, "marker"), "current", 0o600)
			writeTestFile(t, filepath.Join(previous, "marker"), "older", 0o600)
			cli := updateTestCLI("current")
			if tc.failure == "unhealthy-before" {
				cli = strings.Replace(cli, "status) echo 'is running'", "status) echo 'is stopped'", 1)
			}
			writeTestFile(t, filepath.Join(install, "buildworld"), cli, 0o700)
			writeTestFile(t, filepath.Join(home, "loaded"), "yes", 0o600)
			writeTestFile(t, filepath.Join(bin, "uname"), "#!/bin/sh\necho Darwin\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "id"), "#!/bin/sh\necho gamer\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "sleep"), "#!/bin/sh\nexit 0\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "buildworld"), "#!/bin/sh\nexit 0\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "stat"), "#!/bin/sh\ncase \"$2\" in %u) if [ \"$UPDATE_TEST_FAILURE\" = wrong-owner ]; then echo 501; else echo 0; fi;; *) echo 644;; esac\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "ps"), "#!/bin/sh\nif [ \"$UPDATE_TEST_FAILURE\" = wrong-process ]; then echo /other/server; else printf '%s/buildworld-server\\n' \"$TEST_INSTALL\"; fi\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "nc"), "#!/bin/sh\ntest -f \"$HOME/loaded\"\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "launchctl"), "#!/bin/sh\ntest -f \"$HOME/loaded\" || exit 1\nprintf '\"LimitLoadToSessionType\" = \"System\";\\n\"PID\" = 4242;\\n'\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "plistbuddy"), "#!/bin/sh\ncase \"$2\" in 'Print :UserName') echo gamer;; *) echo true;; esac\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "sudo"), "#!/bin/sh\nread -r action\ncase \"$action\" in\nstatus) [ \"$UPDATE_TEST_FAILURE\" != bridge-denied ] || exit 1; test -f \"$HOME/loaded\" || exit 1; echo 'pid = 4242';;\nstop) [ \"$UPDATE_TEST_FAILURE\" != stop ] || exit 1; rm -f \"$HOME/loaded\";;\nstart) if [ \"$(cat \"$TEST_INSTALL/marker\")\" = new ]; then [ \"$UPDATE_TEST_FAILURE\" != start ] || exit 1; touch \"$HOME/started-new\"; else touch \"$HOME/restored\"; fi; touch \"$HOME/loaded\";;\n*) exit 1;; esac\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "curl"), "#!/bin/sh\ntest -f \"$HOME/loaded\" || exit 1\nif [ \"$UPDATE_TEST_FAILURE\" = migration-advanced ] && [ \"$(cat \"$TEST_INSTALL/marker\")\" = new ]; then exit 1; fi\ncase \"$*\" in */api/health*) echo '{\"status\":\"ok\"}';; *) if [ \"$(cat \"$TEST_INSTALL/marker\")\" = new ]; then echo '{\"version\":\"v1.2.3\"}'; else echo '{\"version\":\"v1.0.0\"}'; fi;; esac\n", 0o700)
			_, source, _, _ := runtime.Caller(0)
			scripts := filepath.Join(filepath.Dir(source), "..", "..", "scripts")
			bridge, err := os.ReadFile(filepath.Join(scripts, "buildworld-system-supervisor.sh"))
			if err != nil {
				t.Fatal(err)
			}
			bridgePath, plistPath := filepath.Join(root, "supervisor"), filepath.Join(root, "service.plist")
			if err := os.WriteFile(bridgePath, bridge, 0o700); err != nil {
				t.Fatal(err)
			}
			if tc.failure == "bridge-tampered" {
				writeTestFile(t, bridgePath, "tampered", 0o700)
			}
			if tc.failure == "bridge-missing" {
				if err := os.Remove(bridgePath); err != nil {
					t.Fatal(err)
				}
			}
			writeTestFile(t, plistPath, "fixture", 0o600)
			actual, err := os.ReadFile(filepath.Join(scripts, "apply-update.sh"))
			if err != nil {
				t.Fatal(err)
			}
			helper := string(actual)
			for old, replacement := range map[string]string{
				"/Library/LaunchDaemons/com.buildworld.server.plist": plistPath,
				"/usr/local/libexec/buildworld-system-supervisor":    bridgePath,
				"/usr/libexec/PlistBuddy":                            filepath.Join(bin, "plistbuddy"),
				"/usr/bin/sudo":                                      filepath.Join(bin, "sudo"), "/bin/launchctl": filepath.Join(bin, "launchctl"),
				"/bin/ps": filepath.Join(bin, "ps"), "/usr/bin/nc": filepath.Join(bin, "nc"),
				"/usr/bin/stat": filepath.Join(bin, "stat"),
			} {
				if !strings.Contains(helper, old) {
					t.Fatalf("missing fixture substitution %s", old)
				}
				helper = strings.ReplaceAll(helper, old, shellFixturePath(replacement))
			}
			helperPath := filepath.Join(root, "apply-update.sh")
			writeTestFile(t, helperPath, helper, 0o700)
			bundle := updateTestBundle(t)
			bundlePath := filepath.Join(operation, "bundle.tar.gz")
			if err := os.WriteFile(bundlePath, bundle, 0o600); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(bundle)
			status := filepath.Join(operation, "status.json")
			cmd := exec.Command("sh", helperPath, bundlePath, fmt.Sprintf("%x", sum), "1.2.3", "abcdef", status, filepath.Join(operation, "lock"), install, "")
			cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_INSTALL="+install, "UPDATE_TEST_FAILURE="+tc.failure)
			output, err := cmd.CombinedOutput()
			if (err == nil) != (tc.status == "succeeded") {
				t.Fatalf("exit=%v output=%s", err, output)
			}
			state, err := os.ReadFile(status)
			if err != nil || !strings.Contains(string(state), `"status":"`+tc.status+`"`) {
				t.Fatalf("status=%s err=%v output=%s", state, err, output)
			}
			for path, want := range map[string]string{install: tc.current, previous: tc.previous} {
				got, err := os.ReadFile(filepath.Join(path, "marker"))
				if want == "" && os.IsNotExist(err) {
					continue
				}
				if err != nil || string(got) != want {
					t.Fatalf("marker %s=%q err=%v want=%q", path, got, err, want)
				}
			}
		})
	}
}

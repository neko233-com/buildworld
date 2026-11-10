package systemupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestUpdateHelperPreservesSnapshots exercises the actual Unix helper in an isolated installation.
func TestUpdateHelperPreservesSnapshots(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the update helper is used on macOS and Linux")
	}
	for _, tc := range []struct {
		name, fail, wantStatus, wantCurrent, wantPrevious, wantSnapshot string
		collision                                                       bool
	}{
		{"success", "", "succeeded", "new", "current", "older", false},
		{"new start fails", "new-start", "rolled_back", "current", "", "older", false},
		{"restored server cannot become healthy", "rollback-unhealthy", "failed", "current", "", "older", false},
		{"pause fails", "pause", "rolled_back", "current", "older", "", false},
		{"pause fails but supervisor keeps old server healthy", "pause-start", "rolled_back", "current", "older", "", false},
		{"snapshot collision", "", "failed", "current", "older", "existing", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			install := filepath.Join(root, "install")
			previous := install + ".previous"
			snapshot := install + ".snapshot-abcdef"
			state := filepath.Join(root, "operation")
			home := filepath.Join(root, "home")
			bin := filepath.Join(root, "bin")
			for _, dir := range []string{install, previous, state, home, bin} {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			writeTestFile(t, filepath.Join(install, "marker"), "current", 0o600)
			writeTestFile(t, filepath.Join(previous, "marker"), "older", 0o600)
			writeTestFile(t, filepath.Join(install, "buildworld"), updateTestCLI("current"), 0o700)
			writeTestFile(t, filepath.Join(bin, "buildworld"), "#!/bin/sh\nexit 0\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "sleep"), "#!/bin/sh\nexit 0\n", 0o700)
			writeTestFile(t, filepath.Join(bin, "curl"), "#!/bin/sh\nif [ \"$UPDATE_TEST_FAILURE\" = rollback-unhealthy ] && [ -f \"$HOME/restored\" ]; then exit 1; fi\ncase \"$*\" in */api/health*) echo '{\"status\":\"ok\"}';; *) if [ \"$UPDATE_TEST_FAILURE\" = pause-start ] || [ -f \"$HOME/restored\" ]; then echo '{\"version\":\"v1.0.0\"}'; else echo '{\"version\":\"v1.2.3\"}'; fi;; esac\n", 0o700)
			if tc.collision {
				if err := os.Mkdir(snapshot, 0o700); err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, filepath.Join(snapshot, "marker"), "existing", 0o600)
			}
			bundle := updateTestBundle(t)
			bundlePath := filepath.Join(state, "bundle.tar.gz")
			if err := os.WriteFile(bundlePath, bundle, 0o600); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(bundle)
			status := filepath.Join(state, "status.json")
			lock := filepath.Join(state, "lock")
			_, source, _, _ := runtime.Caller(0)
			helper := filepath.Join(filepath.Dir(source), "..", "..", "scripts", "apply-update.sh")
			cmd := exec.Command("sh", helper, bundlePath, fmt.Sprintf("%x", sum), "1.2.3", "abcdef", status, lock, install, "")
			cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "UPDATE_TEST_FAILURE="+tc.fail)
			output, err := cmd.CombinedOutput()
			if (err == nil) != (tc.wantStatus == "succeeded") {
				t.Fatalf("exit=%v output=%s", err, output)
			}
			statusBytes, err := os.ReadFile(status)
			if err != nil || !strings.Contains(string(statusBytes), `"status":"`+tc.wantStatus+`"`) {
				t.Fatalf("status=%s err=%v output=%s", statusBytes, err, output)
			}
			for path, want := range map[string]string{install: tc.wantCurrent, previous: tc.wantPrevious, snapshot: tc.wantSnapshot} {
				got, err := os.ReadFile(filepath.Join(path, "marker"))
				if want == "" && os.IsNotExist(err) {
					continue
				}
				if err != nil || string(got) != want {
					t.Fatalf("marker %s: got %q err=%v, want %q", path, got, err, want)
				}
			}
		})
	}
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func updateTestCLI(role string) string {
	return "#!/bin/sh\ncase \"$1\" in\nstatus) echo 'is running';;\nversion) echo 'buildworld v1.0.0';;\npause) [ \"$UPDATE_TEST_FAILURE\" != pause ] && [ \"$UPDATE_TEST_FAILURE\" != pause-start ] || exit 1;;\nstart) [ \"$UPDATE_TEST_FAILURE\" != pause-start ] || exit 1; if [ " + role + " = new ] && { [ \"$UPDATE_TEST_FAILURE\" = new-start ] || [ \"$UPDATE_TEST_FAILURE\" = rollback-unhealthy ]; }; then exit 1; fi; [ " + role + " != current ] || touch \"$HOME/restored\";;\nesac\nexit 0\n"
}

func updateTestBundle(t *testing.T) []byte {
	t.Helper()
	files := map[string]string{
		"buildworld": updateTestCLI("new"), "buildworld-server": "server", "buildworld-worker": "worker",
		"apply-update.sh": "helper", "web/dist/index.html": "web", "sdk/pipeline/index.d.ts": "types",
		"sdk/pipeline/index.js": "sdk", "sdk/pipeline/package.json": "{}", "marker": "new",
	}
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

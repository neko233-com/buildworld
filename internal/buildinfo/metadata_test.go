package buildinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCommitRequiresMatchingReleaseMetadata(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "buildworld-server")
	commit := strings.Repeat("a", 40)
	valid := fmt.Sprintf(`{"schemaVersion":1,"repository":"https://github.com/neko233-com/buildworld","version":"v1.2.3","sourceCommit":%q}`, commit)
	for _, tc := range []struct {
		name, content, version, want string
	}{
		{"valid", valid, "1.2.3", commit},
		{"prefixed version", valid, "v1.2.3", commit},
		{"different version", valid, "1.2.4", ""},
		{"development server", valid, "dev", ""},
		{"wrong repository", strings.ReplaceAll(valid, "neko233-com/buildworld", "example/other"), "1.2.3", ""},
		{"invalid commit", strings.ReplaceAll(valid, commit, "not-a-commit"), "1.2.3", ""},
		{"invalid schema", strings.ReplaceAll(valid, `"schemaVersion":1`, `"schemaVersion":2`), "1.2.3", ""},
		{"truncated", "{", "1.2.3", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, "release-info.json"), []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := ReleaseCommit(executable, tc.version); got != tc.want {
				t.Fatalf("commit = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIndependentWorkerMetadataPreservesControllerIdentity(t *testing.T) {
	dir := t.TempDir()
	serverCommit := strings.Repeat("a", 40)
	workerCommit := strings.Repeat("b", 40)
	metadata := func(version, commit string) []byte {
		return []byte(fmt.Sprintf(`{"schemaVersion":1,"repository":"https://github.com/neko233-com/buildworld","version":%q,"sourceCommit":%q}`, version, commit))
	}
	_ = os.WriteFile(filepath.Join(dir, "release-info.json"), metadata("v1.16.38", serverCommit), 0600)
	_ = os.WriteFile(filepath.Join(dir, "buildworld-worker.release-info.json"), metadata("v1.17.0", workerCommit), 0600)
	if got := ReleaseCommit(filepath.Join(dir, "buildworld-server"), "v1.16.38"); got != serverCommit {
		t.Fatal("worker update changed controller identity", got)
	}
	if got := WorkerReleaseCommit(filepath.Join(dir, "buildworld-worker"), "v1.17.0"); got != workerCommit {
		t.Fatal("worker identity mismatch", got)
	}
}

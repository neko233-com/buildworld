//go:build darwin || linux

package selfrelease

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunResumesVerifiedBundleWithoutUploadingTwice(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, args := range [][]string{
		{"init", "-b", "main"}, {"config", "user.name", "BuildWorld Test"},
		{"config", "user.email", "test@example.invalid"},
	} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	if err := os.WriteFile("marker", []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "marker"}, {"commit", "-m", "fixture"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	output, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(output))
	home := filepath.Join(root, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	configRoot, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	jobHash := sha256.Sum256([]byte("self-release"))
	directory := filepath.Join(configRoot, "buildworld", "self-release", hex.EncodeToString(jobHash[:]), "1")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	bundle := []byte("bundle")
	if err := os.WriteFile(filepath.Join(directory, "bundle.tar.gz"), bundle, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(bundle)
	checksum := hex.EncodeToString(sum[:])
	state := record{Version: "1.2.3", Commit: commit, BundleSHA: checksum, Phase: "prepared"}
	if err := saveRecord(filepath.Join(directory, "state.json"), state); err != nil {
		t.Fatal(err)
	}
	fixture := &updateFixture{state: "idle", version: "v1.2.2", expectedSHA: checksum, wantCommit: commit}
	server := httptest.NewServer(fixture)
	defer server.Close()
	variables := map[string]string{
		"RELEASE_VERSION": "v1.2.3", "SOURCE_COMMIT": commit, "GIT_COMMIT": commit,
		"JOB_NAME": "self-release", "BUILD_NUMBER": "1", "BUILDWORLD_UPDATE_TOKEN": "test-token",
		"BUILDWORLD_SELFRELEASE_URL": server.URL,
	}
	for i := 0; i < 2; i++ {
		if err := Run(context.Background(), func(name string) string { return variables[name] }); err != nil {
			t.Fatal(err)
		}
	}
	if fixture.posts != 1 {
		t.Fatalf("uploads after recovery=%d, want 1", fixture.posts)
	}
	variables["SOURCE_COMMIT"] = strings.Repeat("f", 40)
	if err := Run(context.Background(), func(name string) string { return variables[name] }); err == nil {
		t.Fatal("accepted a different workspace commit")
	}
}

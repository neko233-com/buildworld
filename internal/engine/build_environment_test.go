package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestBuildEnvironmentCreatesDedicatedLanguageCaches(t *testing.T) {
	root := filepath.Join(t.TempDir(), "build_temp")
	environment := NewBuildEnvironment(root)
	workspace := filepath.Join(environment.WorkspacesRoot(), "build-1")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	values, err := environment.Environment(workspace)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, value := range values {
		key, data, found := strings.Cut(value, "=")
		if found {
			env[key] = data
		}
	}
	for _, key := range []string{"TMP", "TEMP", "TMPDIR", "GOCACHE", "GOMODCACHE", "GOPATH", "NPM_CONFIG_CACHE", "NPM_CONFIG_PREFIX", "NPM_CONFIG_USERCONFIG", "COREPACK_HOME", "PNPM_HOME", "BUILDWORLD_WORKSPACE", "WORKSPACE"} {
		value := env[key]
		if value == "" {
			t.Fatalf("%s was not configured", key)
		}
		relative, err := filepath.Rel(root, value)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("%s=%q is outside build root %q", key, value, root)
		}
	}
	if env["GOCACHE"] == env["NPM_CONFIG_CACHE"] {
		t.Fatal("Go and npm caches must be isolated")
	}
	if env["WORKSPACE"] != env["BUILDWORLD_WORKSPACE"] {
		t.Fatalf("Jenkins workspace alias = %q, BuildWorld workspace = %q", env["WORKSPACE"], env["BUILDWORLD_WORKSPACE"])
	}
}

func TestBuiltinTemporaryDirectorySurvivesDeleteDirAndIsRemovedAfterBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Jenkins sh uses the native POSIX shell; Windows verifies the same cleanup operation separately")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("POSIX shell unavailable")
	}
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	shell, before, after := "sh", `printf old > stale.txt; test -d "$TMPDIR"`, `test ! -e stale.txt && test -d "$TMPDIR" && test "$TMPDIR" = "$TMP" && test "$TMPDIR" = "$TEMP" && touch "$TMPDIR/probe" && echo TEMP_SURVIVED`
	config := &BuildConfig{Name: "clean-temp", Stages: []Stage{{Name: "clean", Steps: []Step{
		{Name: "before", Type: "shell", Shell: shell, Command: before},
		{Name: "deleteDir", Type: "jenkins_delete_dir"},
		{Name: "after", Type: "shell", Shell: shell, Command: after},
	}}}}
	source, err := FormatTypeScriptPipeline(config)
	if err != nil {
		t.Fatal(err)
	}
	project, err := db.CreateProject("clean-temp", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	environment := NewBuildEnvironment(filepath.Join(root, "build-temp"))
	runner := NewBuildRunner(db, nil, environment.WorkspacesRoot(), nil)
	runner.SetBuildEnvironment(environment)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	finished := waitForBuildStatus(t, db, build.ID, 10*time.Second, "success", "failed")
	if finished.Status != "success" || !strings.Contains(finished.Log, "TEMP_SURVIVED") {
		t.Fatalf("build status=%s log=%s", finished.Status, finished.Log)
	}
	deadline := time.Now().Add(time.Second)
	for {
		entries, err := os.ReadDir(filepath.Join(environment.Root(), "tmp"))
		if err == nil && len(entries) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("build temporary files not cleaned: %v %v", entries, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLocalTemporaryDirectoryIsIsolatedFromWorkspaceCleanup(t *testing.T) {
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	environment := NewBuildEnvironment(filepath.Join(root, "build-temp"))
	workspace := filepath.Join(environment.WorkspacesRoot(), "fixture")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "stale"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, cleanup, err := environment.LocalEnvironment(workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	vars := map[string]string{}
	for _, value := range values {
		key, data, ok := strings.Cut(value, "=")
		if ok {
			vars[key] = data
		}
	}
	temporary := vars["TMPDIR"]
	if temporary == "" || temporary != vars["TMP"] || temporary != vars["TEMP"] {
		t.Fatal("temporary environment mismatch")
	}
	runner := NewBuildRunner(db, nil, environment.WorkspacesRoot(), nil)
	defer runner.Shutdown(context.Background())
	if err := runner.execBaseStep(context.Background(), Step{Type: "jenkins_delete_dir"}, workspace, nil, nil, nil, nil, "", func(string) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "stale")); !os.IsNotExist(err) {
		t.Fatal("deleteDir did not clean workspace")
	}
	file, err := os.CreateTemp(temporary, "probe-")
	if err != nil {
		t.Fatalf("temporary directory did not survive deleteDir: %v", err)
	}
	_ = file.Close()
	cleanup()
	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatalf("temporary directory not cleaned: %v", err)
	}
}

func TestGoRuntimeUsesDeclaredToolchain(t *testing.T) {
	values := AppendRuntimeEnvironment(nil, &BuildConfig{Toolchains: map[string][]string{"go": {"1.26"}}}, "go")
	joined := strings.Join(values, "\n")
	for _, expected := range []string{"BUILDWORLD_RUNTIME_GO=1.26", "GOTOOLCHAIN=go1.26"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("runtime environment %q does not contain %q", joined, expected)
		}
	}
}

func TestConfiguredEnvironmentExpandsJenkinsStylePathAndDependencies(t *testing.T) {
	values := ExpandConfiguredEnvironment(
		map[string]string{
			"PATH":       "${PATH}:/usr/local/go/bin",
			"TARGET_DIR": "/srv/game",
			"LOG_FILE":   "${TARGET_DIR}/logs/server.log",
		},
		[]string{"PATH=/usr/bin", "HOME=/home/worker"},
	)
	environment := map[string]string{}
	for _, value := range values {
		key, data, found := strings.Cut(value, "=")
		if found {
			environment[key] = data
		}
	}
	if environment["PATH"] != "/usr/bin:/usr/local/go/bin" {
		t.Fatalf("PATH = %q", environment["PATH"])
	}
	if environment["LOG_FILE"] != "/srv/game/logs/server.log" {
		t.Fatalf("LOG_FILE = %q", environment["LOG_FILE"])
	}
}

func TestWorkspaceManagerRefusesCleanupOutsideRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspaces")
	manager := NewWorkspaceManager(root)
	outside := filepath.Join(filepath.Dir(root), "keep")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := manager.Clean(outside); err == nil {
		t.Fatal("cleanup outside workspace root should fail")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside directory was modified: %v", err)
	}
}

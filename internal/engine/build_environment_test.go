package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

package plugin

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSelectReleaseForMatchingPlatform(t *testing.T) {
	manifest := BinaryManifest{Name: "echo", Releases: []BinaryRelease{
		{GOOS: "windows", GOARCH: "amd64", URL: "https://github.com/acme/echo/releases/download/v1/echo.exe", ChecksumSHA256: strings.Repeat("a", 64)},
		{GOOS: "darwin", GOARCH: "arm64", URL: "https://github.com/acme/echo/releases/download/v1/echo", ChecksumSHA256: strings.Repeat("d", 64)},
	}}
	release, ok, err := selectRelease(manifest, "windows", "amd64")
	if err != nil || !ok {
		t.Fatalf("selectRelease() = %#v, %t, %v", release, ok, err)
	}
	if release.ChecksumSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("checksum = %q", release.ChecksumSHA256)
	}
}

func TestSelectReleaseRejectsMissingCurrentPlatform(t *testing.T) {
	manifest := BinaryManifest{Name: "echo", Releases: []BinaryRelease{{GOOS: "darwin", GOARCH: "arm64", URL: "https://github.com/acme/echo/releases/download/v1/echo", ChecksumSHA256: strings.Repeat("d", 64)}}}
	if _, _, err := selectRelease(manifest, "windows", "amd64"); err == nil {
		t.Fatal("selectRelease() accepted an unsupported platform")
	}
}

func TestSelectReleaseRequiresChecksum(t *testing.T) {
	manifest := BinaryManifest{Name: "echo", Releases: []BinaryRelease{{GOOS: "linux", GOARCH: "amd64", URL: "https://github.com/acme/echo/releases/download/v1/echo"}}}
	if _, _, err := selectRelease(manifest, "linux", "amd64"); err == nil {
		t.Fatal("selectRelease() accepted a release without checksum")
	}
}

func TestBinaryReferencesRequireTrustedPortablePlugin(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "echo")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := "plugin"
	if runtime.GOOS == "windows" {
		entry += ".exe"
	}
	if err := os.WriteFile(filepath.Join(pluginDir, entry), []byte("placeholder"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := BinaryManifest{APIVersion: BinaryAPIVersion, Name: "echo", Version: "1.2.3", Description: "test", Source: "https://github.com/acme/buildworld-plugin-echo", Entrypoint: "plugin", Steps: []string{"echo-plugin"}}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin-buildworld.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	loader := NewLoader(root)
	if err := loader.Load("echo"); err != nil {
		t.Fatal(err)
	}
	references, err := loader.BinaryReferencesForStepTypes([]string{"shell", "echo-plugin", "echo-plugin"})
	if err != nil {
		t.Fatal(err)
	}
	if len(references) != 1 {
		t.Fatalf("references = %#v", references)
	}
	ref := references[0]
	if ref.Name != manifest.Name || ref.Version != manifest.Version || ref.Source != manifest.Source || ref.ManifestSHA256 == "" {
		t.Fatalf("reference = %#v", ref)
	}
	if _, err := loader.BinaryReferencesForStepTypes([]string{"unknown-plugin-step"}); err == nil {
		t.Fatal("unknown remote plugin step was accepted")
	}
}

func TestInvokeBinaryExchangesDataOnlyStepContext(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "plugin")
	if runtime.GOOS == "windows" {
		entry += ".exe"
	}
	source := filepath.Join(root, "main.go")
	program := `package main
import (
  "encoding/json"
  "os"
)
func main() {
  var request map[string]any
  if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil { panic(err) }
  _ = json.NewEncoder(os.Stdout).Encode(map[string]any{
    "logs": []string{"binary-ran"},
    "env": map[string]string{"REGION":"local"},
    "outputs": map[string]string{"IMAGE":"app:1"},
  })
}`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("go", "build", "-o", entry, source).CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v: %s", err, output)
	}
	stepContext := &StepContext{Workspace: root, Config: map[string]string{"target": "test"}}
	if err := invokeBinary(t.Context(), entry, "fixture:step", stepContext); err != nil {
		t.Fatal(err)
	}
	if len(stepContext.Logs) != 1 || stepContext.Logs[0] != "binary-ran" || stepContext.Env["REGION"] != "local" || stepContext.Outputs["IMAGE"] != "app:1" {
		t.Fatalf("binary response = %#v", stepContext)
	}
}

func TestPluginInstallSourceReturnsBinarySourceOnly(t *testing.T) {
	empty := &Plugin{}
	if got := empty.InstallSource(); got != "" {
		t.Fatalf("empty InstallSource() = %q", got)
	}
	binary := &Plugin{binary: &BinaryManifest{Source: "https://github.com/acme/buildworld-plugin"}}
	if got := binary.InstallSource(); got != "https://github.com/acme/buildworld-plugin" {
		t.Fatalf("binary InstallSource() = %q", got)
	}
}

func TestValidateBinaryManifestRejectsDuplicateStepsAndInvalidSource(t *testing.T) {
	manifest := &BinaryManifest{
		APIVersion: BinaryAPIVersion,
		Name:       "echo",
		Version:    "1.0.0",
		Entrypoint: "echo",
		Steps:      []string{"echo", "echo"},
	}
	if err := validateBinaryManifest(manifest, "echo"); err == nil {
		t.Fatal("duplicate plugin steps were accepted")
	}
	manifest.Steps = []string{"echo"}
	manifest.Source = "https://example.com/acme/echo"
	if err := validateBinaryManifest(manifest, "echo"); err == nil {
		t.Fatal("non-GitHub plugin source was accepted")
	}
}

func TestNormalizeGitHubSourceRequiresRepositoryURL(t *testing.T) {
	for _, source := range []string{
		"http://github.com/acme/echo",
		"https://github.com:8443/acme/echo",
		"https://user@github.com/acme/echo",
		"https://github.com/acme",
		"https://example.com/acme/echo",
	} {
		if _, err := normalizeGitHubSource(source); err == nil {
			t.Fatalf("normalizeGitHubSource(%q) succeeded", source)
		}
	}
	got, err := normalizeGitHubSource("https://github.com/acme/echo.git/")
	if err != nil || got != "https://github.com/acme/echo" {
		t.Fatalf("normalized source = %q, %v", got, err)
	}
}

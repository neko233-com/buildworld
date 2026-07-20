package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSelectReleaseForMatchingPlatform(t *testing.T) {
	manifest := BinaryManifest{Name: "echo", Releases: []BinaryRelease{
		{GOOS: "windows", GOARCH: "amd64", URL: "https://github.com/acme/echo/releases/download/v1/echo.exe", ChecksumSHA256: "abc"},
		{GOOS: "darwin", GOARCH: "arm64", URL: "https://github.com/acme/echo/releases/download/v1/echo", ChecksumSHA256: "def"},
	}}
	release, ok, err := selectRelease(manifest, "windows", "amd64")
	if err != nil || !ok {
		t.Fatalf("selectRelease() = %#v, %t, %v", release, ok, err)
	}
	if release.ChecksumSHA256 != "abc" {
		t.Fatalf("checksum = %q", release.ChecksumSHA256)
	}
}

func TestSelectReleaseRejectsMissingCurrentPlatform(t *testing.T) {
	manifest := BinaryManifest{Name: "echo", Releases: []BinaryRelease{{GOOS: "darwin", GOARCH: "arm64", URL: "https://github.com/acme/echo/releases/download/v1/echo", ChecksumSHA256: "def"}}}
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

func TestPluginInstallSourceDistinguishesBuiltinsAndGitHubBinaries(t *testing.T) {
	builtin := &Plugin{}
	if got := builtin.InstallSource(); got != "builtin" {
		t.Fatalf("builtin InstallSource() = %q", got)
	}
	binary := &Plugin{binary: &BinaryManifest{Source: "https://github.com/acme/buildworld-plugin"}}
	if got := binary.InstallSource(); got != "https://github.com/acme/buildworld-plugin" {
		t.Fatalf("binary InstallSource() = %q", got)
	}
}

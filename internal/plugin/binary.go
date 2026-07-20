package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const BinaryAPIVersion = "buildworld.plugin/v1"
const maxBinaryPluginBytes = 256 << 20

type binaryRequest struct {
	Operation, Step string
	Context         StepContext
}
type binaryResponse struct {
	Logs    []string          `json:"logs"`
	Env     map[string]string `json:"env"`
	Outputs map[string]string `json:"outputs"`
	Error   string            `json:"error"`
}

func (l *Loader) loadBinary(name, pluginPath string) error {
	data, err := os.ReadFile(filepath.Join(pluginPath, "plugin-buildworld.json"))
	if err != nil {
		return err
	}
	var manifest BinaryManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse plugin-buildworld.json: %w", err)
	}
	if manifest.APIVersion != BinaryAPIVersion {
		return fmt.Errorf("unsupported binary plugin API %q", manifest.APIVersion)
	}
	if manifest.Name == "" || manifest.Entrypoint == "" || len(manifest.Steps) == 0 {
		return fmt.Errorf("binary plugin requires name, entrypoint, and steps")
	}
	entry, err := resolveEntrypoint(pluginPath, manifest.Entrypoint)
	if err != nil {
		return err
	}
	if manifest.ChecksumSHA256 != "" {
		checksum, err := checksumFile(entry)
		if err != nil || !strings.EqualFold(checksum, manifest.ChecksumSHA256) {
			return fmt.Errorf("binary plugin checksum mismatch")
		}
	}
	p := &Plugin{PluginMeta: PluginMeta{Name: manifest.Name, Version: manifest.Version, Description: manifest.Description}, Path: pluginPath, binary: &manifest, stepTypes: map[string]StepHandler{}, triggerTypes: map[string]TriggerHandler{}}
	for _, stepType := range manifest.Steps {
		typ := stepType
		p.stepTypes[typ] = func(ctx context.Context, sc *StepContext) error { return invokeBinary(ctx, entry, typ, sc) }
	}
	l.mu.Lock()
	l.plugins[name] = p
	l.enabledPlugins[name] = true
	l.mu.Unlock()
	return nil
}

func invokeBinary(ctx context.Context, entry, step string, sc *StepContext) error {
	input, _ := json.Marshal(binaryRequest{Operation: "execute", Step: step, Context: *sc})
	cmd := exec.CommandContext(ctx, entry, "execute")
	cmd.Stdin = strings.NewReader(string(input))
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("binary plugin %s: %w", step, err)
	}
	var response binaryResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return fmt.Errorf("binary plugin response: %w", err)
	}
	if response.Error != "" {
		return fmt.Errorf("binary plugin: %s", response.Error)
	}
	sc.Logs = append(sc.Logs, response.Logs...)
	if sc.Env == nil {
		sc.Env = map[string]string{}
	}
	for k, v := range response.Env {
		sc.Env[k] = v
	}
	if sc.Outputs == nil {
		sc.Outputs = map[string]string{}
	}
	for k, v := range response.Outputs {
		sc.Outputs[k] = v
	}
	return nil
}

// InstallGitHub builds a Go plugin from a public GitHub repository. The repo
// must contain plugin-buildworld.json at its root; no JavaScript is executed.
func (l *Loader) InstallGitHub(ctx context.Context, source string) (*BinaryManifest, error) {
	if !strings.HasPrefix(source, "https://github.com/") {
		return nil, fmt.Errorf("plugin source must be an https GitHub URL")
	}
	tmp, err := os.MkdirTemp("", "buildworld-plugin-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if out, err := exec.CommandContext(ctx, "git", "clone", "--depth", "1", source, tmp).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("clone plugin: %w: %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "plugin-buildworld.json"))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest BinaryManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if manifest.APIVersion != BinaryAPIVersion || manifest.Name == "" || manifest.Entrypoint == "" || len(manifest.Steps) == 0 {
		return nil, fmt.Errorf("invalid binary plugin manifest")
	}
	manifest.Source = strings.TrimSuffix(source, "/")
	dest := filepath.Join(l.path, manifest.Name)
	// Shared GitHub URLs are idempotent: an installed matching name/version is
	// reused instead of compiling or downloading it again.
	if existingData, err := os.ReadFile(filepath.Join(dest, "plugin-buildworld.json")); err == nil {
		var existing BinaryManifest
		if json.Unmarshal(existingData, &existing) == nil && existing.Name == manifest.Name && existing.Version == manifest.Version && existing.Source == manifest.Source {
			if l.Get(manifest.Name) == nil {
				_ = l.Load(manifest.Name)
			}
			return &existing, nil
		}
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, err
	}
	entry, err := targetEntrypoint(dest, manifest.Entrypoint)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		return nil, err
	}
	if release, ok, err := selectRelease(manifest, runtime.GOOS, runtime.GOARCH); err != nil {
		return nil, err
	} else if ok {
		if err := downloadRelease(ctx, release, entry); err != nil {
			return nil, err
		}
		manifest.ChecksumSHA256 = release.ChecksumSHA256
	} else {
		pkg := manifest.Package
		if pkg == "" {
			pkg = "."
		}
		build := exec.CommandContext(ctx, "go", "build", "-o", entry, pkg)
		build.Dir = tmp
		if out, err := build.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("build Go plugin: %w: %s", err, out)
		}
	}
	resolvedEntry, err := resolveEntrypoint(dest, manifest.Entrypoint)
	if err != nil {
		return nil, err
	}
	if checksum, err := checksumFile(resolvedEntry); err != nil {
		return nil, err
	} else if manifest.ChecksumSHA256 == "" {
		manifest.ChecksumSHA256 = checksum
	} else if !strings.EqualFold(checksum, manifest.ChecksumSHA256) {
		return nil, fmt.Errorf("binary plugin checksum mismatch")
	} else {
		data, _ = json.MarshalIndent(manifest, "", "  ")
	}
	data, _ = json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dest, "plugin-buildworld.json"), data, 0o644); err != nil {
		return nil, err
	}
	if err := l.Load(manifest.Name); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func manifestDigest(manifest *BinaryManifest) (string, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}

func isRemoteBuiltinStep(stepType string) bool {
	switch stepType {
	case "", "shell", "tail", "service_watch", "powershell", "ps1", "pwsh", "bash", "sh", "python", "python3", "cmd", "script", "git":
		return true
	default:
		return false
	}
}

// BinaryReferencesForStepTypes returns only Go binary plugins that the worker
// may resolve independently. Legacy in-process JavaScript plugins intentionally
// cannot be dispatched remotely because their execution model is not portable.
func (l *Loader) BinaryReferencesForStepTypes(stepTypes []string) ([]BinaryReference, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	seen := map[string]bool{}
	refs := make([]BinaryReference, 0)
	for _, stepType := range stepTypes {
		if isRemoteBuiltinStep(stepType) {
			continue
		}
		var matched *Plugin
		for name, candidate := range l.plugins {
			if l.enabledPlugins[name] && candidate.stepTypes[stepType] != nil {
				matched = candidate
				break
			}
		}
		if matched == nil {
			return nil, fmt.Errorf("remote worker cannot resolve plugin step type %q", stepType)
		}
		if matched.binary == nil {
			return nil, fmt.Errorf("plugin step type %q is legacy JavaScript and cannot run on a remote worker", stepType)
		}
		if matched.binary.Source == "" {
			return nil, fmt.Errorf("plugin %q must be installed from a GitHub source before remote execution", matched.binary.Name)
		}
		if seen[matched.binary.Name] {
			continue
		}
		digest, err := manifestDigest(matched.binary)
		if err != nil {
			return nil, err
		}
		seen[matched.binary.Name] = true
		refs = append(refs, BinaryReference{Name: matched.binary.Name, Version: matched.binary.Version, Source: matched.binary.Source, ManifestSHA256: digest})
	}
	return refs, nil
}

// EnsureBinaryReference installs or refreshes one server-approved Go plugin in
// a worker cache and proves that the resolved manifest is exactly the expected
// version and digest before it is eligible to execute build steps.
func (l *Loader) EnsureBinaryReference(ctx context.Context, ref BinaryReference) error {
	if ref.Name == "" || ref.Version == "" || ref.ManifestSHA256 == "" {
		return fmt.Errorf("invalid remote plugin reference")
	}
	if !strings.HasPrefix(ref.Source, "https://github.com/") {
		return fmt.Errorf("remote plugin %q source must be an https GitHub URL", ref.Name)
	}
	l.mu.RLock()
	current := l.plugins[ref.Name]
	if current != nil && l.enabledPlugins[ref.Name] && current.binary != nil {
		digest, err := manifestDigest(current.binary)
		if err == nil && current.binary.Name == ref.Name && current.binary.Version == ref.Version && current.binary.Source == strings.TrimSuffix(ref.Source, "/") && strings.EqualFold(digest, ref.ManifestSHA256) {
			l.mu.RUnlock()
			return nil
		}
	}
	l.mu.RUnlock()
	manifest, err := l.InstallGitHub(ctx, ref.Source)
	if err != nil {
		return err
	}
	digest, err := manifestDigest(manifest)
	if err != nil {
		return err
	}
	if manifest.Name != ref.Name || manifest.Version != ref.Version || !strings.EqualFold(digest, ref.ManifestSHA256) {
		return fmt.Errorf("remote plugin %q manifest does not match the server reference", ref.Name)
	}
	return nil
}

func selectRelease(manifest BinaryManifest, goos, goarch string) (BinaryRelease, bool, error) {
	for _, release := range manifest.Releases {
		if release.GOOS == goos && release.GOARCH == goarch {
			if release.URL == "" || release.ChecksumSHA256 == "" {
				return BinaryRelease{}, false, fmt.Errorf("prebuilt plugin release for %s/%s requires url and checksum_sha256", goos, goarch)
			}
			return release, true, nil
		}
	}
	if len(manifest.Releases) > 0 {
		return BinaryRelease{}, false, fmt.Errorf("plugin %s has no release for %s/%s", manifest.Name, goos, goarch)
	}
	return BinaryRelease{}, false, nil
}

func targetEntrypoint(pluginPath, entrypoint string) (string, error) {
	entry := filepath.Clean(filepath.Join(pluginPath, entrypoint))
	if !strings.HasPrefix(entry, filepath.Clean(pluginPath)+string(os.PathSeparator)) {
		return "", fmt.Errorf("plugin entrypoint escapes plugin directory")
	}
	return entry, nil
}

func resolveEntrypoint(pluginPath, entrypoint string) (string, error) {
	entry, err := targetEntrypoint(pluginPath, entrypoint)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(entry); err != nil && runtime.GOOS == "windows" {
		entry += ".exe"
	}
	if _, err := os.Stat(entry); err != nil {
		return "", fmt.Errorf("binary entrypoint: %w", err)
	}
	return entry, nil
}

func downloadRelease(ctx context.Context, release BinaryRelease, destination string) error {
	assetURL, err := url.Parse(release.URL)
	if err != nil || assetURL.Scheme != "https" || assetURL.Host == "" {
		return fmt.Errorf("prebuilt plugin release requires an https URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.URL, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("download prebuilt plugin: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download prebuilt plugin: %s", resp.Status)
	}
	if resp.ContentLength > maxBinaryPluginBytes {
		return fmt.Errorf("prebuilt plugin exceeds %d MiB limit", maxBinaryPluginBytes>>20)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer file.Close()
	written, err := io.Copy(file, io.LimitReader(resp.Body, maxBinaryPluginBytes+1))
	if err != nil {
		return err
	}
	if written > maxBinaryPluginBytes {
		_ = os.Remove(destination)
		return fmt.Errorf("prebuilt plugin exceeds %d MiB limit", maxBinaryPluginBytes>>20)
	}
	return nil
}

func checksumFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

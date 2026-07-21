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
	"path/filepath"
	"runtime"
	"strings"

	"github.com/neko233-com/buildworld/internal/processtree"
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
	if err := validateBinaryManifest(&manifest, name); err != nil {
		return err
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
	p := &Plugin{PluginMeta: PluginMeta{Name: manifest.Name, Version: manifest.Version, Description: manifest.Description, Author: manifest.Author}, Path: pluginPath, binary: &manifest, stepTypes: map[string]StepHandler{}}
	for _, stepType := range manifest.Steps {
		typ := stepType
		p.stepTypes[typ] = func(ctx context.Context, sc *StepContext) error { return invokeBinary(ctx, entry, typ, sc) }
	}
	l.mu.Lock()
	for installedName, installed := range l.plugins {
		if installedName == name {
			continue
		}
		for _, stepType := range manifest.Steps {
			if installed.stepTypes[stepType] != nil {
				l.mu.Unlock()
				return fmt.Errorf("plugin step type %q is already registered by %q", stepType, installedName)
			}
		}
	}
	l.plugins[name] = p
	l.enabledPlugins[name] = true
	l.mu.Unlock()
	return nil
}

func validateBinaryManifest(manifest *BinaryManifest, expectedName string) error {
	if manifest.APIVersion != BinaryAPIVersion {
		return fmt.Errorf("unsupported binary plugin API %q", manifest.APIVersion)
	}
	if err := validatePluginName(manifest.Name); err != nil {
		return err
	}
	if expectedName != "" && manifest.Name != expectedName {
		return fmt.Errorf("plugin manifest name %q does not match directory %q", manifest.Name, expectedName)
	}
	if strings.TrimSpace(manifest.Version) == "" || strings.TrimSpace(manifest.Entrypoint) == "" || len(manifest.Steps) == 0 {
		return fmt.Errorf("binary plugin requires name, version, entrypoint, and steps")
	}
	seenSteps := make(map[string]struct{}, len(manifest.Steps))
	for _, step := range manifest.Steps {
		step = strings.TrimSpace(step)
		if step == "" {
			return fmt.Errorf("binary plugin step names cannot be empty")
		}
		if _, duplicate := seenSteps[step]; duplicate {
			return fmt.Errorf("binary plugin step %q is duplicated", step)
		}
		seenSteps[step] = struct{}{}
	}
	if manifest.Source != "" {
		if _, err := normalizeGitHubSource(manifest.Source); err != nil {
			return err
		}
	}
	if manifest.ChecksumSHA256 != "" && !isSHA256(manifest.ChecksumSHA256) {
		return fmt.Errorf("binary plugin checksum_sha256 must be a 64-character hexadecimal digest")
	}
	releaseTargets := make(map[string]struct{}, len(manifest.Releases))
	for _, release := range manifest.Releases {
		target := release.GOOS + "/" + release.GOARCH
		if release.GOOS == "" || release.GOARCH == "" || release.URL == "" || !isSHA256(release.ChecksumSHA256) {
			return fmt.Errorf("binary plugin release %q requires goos, goarch, url, and checksum_sha256", target)
		}
		assetURL, err := url.Parse(release.URL)
		if err != nil || assetURL.Scheme != "https" || assetURL.Host == "" || assetURL.User != nil {
			return fmt.Errorf("binary plugin release %q requires an https URL", target)
		}
		if _, duplicate := releaseTargets[target]; duplicate {
			return fmt.Errorf("binary plugin release %q is duplicated", target)
		}
		releaseTargets[target] = struct{}{}
	}
	return nil
}

func normalizeGitHubSource(source string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") || parsed.Port() != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("plugin source must be an https GitHub repository URL")
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("plugin source must identify one GitHub owner and repository")
	}
	parsed.Path = strings.TrimSuffix(strings.TrimSuffix(parsed.Path, "/"), ".git")
	parsed.RawPath = ""
	return parsed.String(), nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

func invokeBinary(ctx context.Context, entry, step string, sc *StepContext) error {
	input, _ := json.Marshal(binaryRequest{Operation: "execute", Step: step, Context: *sc})
	cmd := processtree.CommandContext(ctx, entry, "execute")
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
	normalizedSource, err := normalizeGitHubSource(source)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(l.path, 0o755); err != nil {
		return nil, fmt.Errorf("create plugin root: %w", err)
	}
	tmp, err := os.MkdirTemp("", "buildworld-plugin-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if out, err := processtree.CommandContext(ctx, "git", "clone", "--depth", "1", normalizedSource, tmp).CombinedOutput(); err != nil {
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
	if err := validateBinaryManifest(&manifest, ""); err != nil {
		return nil, err
	}
	manifest.Source = normalizedSource
	dest, err := l.pluginPath(manifest.Name)
	if err != nil {
		return nil, err
	}
	// Shared GitHub URLs are idempotent: an installed matching name/version is
	// reused instead of compiling or downloading it again.
	if existingData, err := os.ReadFile(filepath.Join(dest, "plugin-buildworld.json")); err == nil {
		var existing BinaryManifest
		if json.Unmarshal(existingData, &existing) == nil && existing.Name == manifest.Name && existing.Version == manifest.Version && existing.Source == manifest.Source {
			if l.getLoaded(manifest.Name) == nil {
				if err := l.Load(manifest.Name); err != nil {
					return nil, err
				}
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
		build := processtree.CommandContext(ctx, "go", "build", "-o", entry, pkg)
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

// BinaryReferencesForStepTypes returns the Go binary plugins that a worker must
// resolve independently for the requested custom step types.
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
	if validatePluginName(ref.Name) != nil || strings.TrimSpace(ref.Version) == "" || !isSHA256(ref.ManifestSHA256) {
		return fmt.Errorf("invalid remote plugin reference")
	}
	normalizedSource, err := normalizeGitHubSource(ref.Source)
	if err != nil {
		return fmt.Errorf("remote plugin %q: %w", ref.Name, err)
	}
	l.mu.RLock()
	current := l.plugins[ref.Name]
	if current != nil && l.enabledPlugins[ref.Name] && current.binary != nil {
		digest, err := manifestDigest(current.binary)
		if err == nil && current.binary.Name == ref.Name && current.binary.Version == ref.Version && current.binary.Source == normalizedSource && strings.EqualFold(digest, ref.ManifestSHA256) {
			l.mu.RUnlock()
			return nil
		}
	}
	l.mu.RUnlock()
	manifest, err := l.InstallGitHub(ctx, normalizedSource)
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
			if release.URL == "" || !isSHA256(release.ChecksumSHA256) {
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
	root, err := filepath.Abs(pluginPath)
	if err != nil {
		return "", fmt.Errorf("resolve plugin directory: %w", err)
	}
	entry, err := filepath.Abs(filepath.Join(root, entrypoint))
	if err != nil {
		return "", fmt.Errorf("resolve plugin entrypoint: %w", err)
	}
	relative, err := filepath.Rel(root, entry)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
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

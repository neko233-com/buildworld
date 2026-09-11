package systemupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const OfficialRepository = "neko233-com/buildworld"

const officialReleaseAPI = "https://api.github.com/repos/neko233-com/buildworld/releases/latest"

const (
	githubMirrorEnv     = "BUILDWORLD_GITHUB_MIRROR"
	defaultGitHubMirror = "https://gh-proxy.com"
)

const (
	maxReleaseMetadataBytes = 2 << 20
	maxReleaseAssetBytes    = MaxBundleBytes + 1
)

var partNamePattern = regexp.MustCompile(`^buildworld-[a-z0-9]+-[a-z0-9]+\.(?:tar\.gz|zip)\.part[0-9]{3}$`)
var partNumberPattern = regexp.MustCompile(`^\d{3}$`)

type ReleaseInfo struct {
	Version     string
	ReleaseURL  string
	PublishedAt string
	AssetName   string
	AssetSize   int64

	checksumAsset githubAsset
	parts         []githubAsset
}

type Bundle struct {
	Version string
	SHA256  string
	Reader  io.ReadCloser
}

type Catalog interface {
	Check(context.Context) (ReleaseInfo, error)
	Download(context.Context, ReleaseInfo) (Bundle, error)
}

type GitHubCatalog struct {
	client       *http.Client
	apiURL       string
	githubMirror *url.URL
	goos         string
	goarch       string
	assetOS      string
}

func NewGitHubCatalog(client *http.Client) *GitHubCatalog {
	return newGitHubCatalog(client, os.Getenv(githubMirrorEnv))
}

func newGitHubCatalog(client *http.Client, mirror string) *GitHubCatalog {
	githubMirror := parseGitHubMirror(mirror)
	if strings.TrimSpace(mirror) == "" {
		githubMirror = parseGitHubMirror(defaultGitHubMirror)
	}
	catalog := &GitHubCatalog{
		apiURL:       officialReleaseAPI,
		githubMirror: githubMirror,
		goos:         runtime.GOOS,
		goarch:       runtime.GOARCH,
		assetOS:      runtime.GOOS,
	}
	if client == nil {
		client = &http.Client{
			Timeout: 45 * time.Second,
			CheckRedirect: func(request *http.Request, previous []*http.Request) error {
				if len(previous) >= 4 || !catalog.isTrustedRequestURL(request.URL) {
					return errors.New("release download redirected to an untrusted host")
				}
				return nil
			},
		}
	}
	catalog.client = client
	return catalog
}

func parseGitHubMirror(value string) *url.URL {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "off") || strings.EqualFold(value, "none") {
		return nil
	}
	parsed, err := url.Parse(strings.TrimRight(value, "/"))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil
	}
	return parsed
}

func (c *GitHubCatalog) isTrustedRequestURL(value *url.URL) bool {
	if isTrustedDownloadURL(value) {
		return true
	}
	if c == nil || c.githubMirror == nil || value == nil || value.Scheme != c.githubMirror.Scheme || value.User != nil {
		return false
	}
	return strings.EqualFold(value.Host, c.githubMirror.Host)
}

func (c *GitHubCatalog) sourceURL(raw string) string {
	if c == nil || c.githubMirror == nil || !isTrustedDownloadURLString(raw) {
		return raw
	}
	mirrored := *c.githubMirror
	mirrored.Path = strings.TrimRight(mirrored.Path, "/") + "/" + raw
	mirrored.RawPath = ""
	mirrored.RawQuery = ""
	mirrored.Fragment = ""
	return mirrored.String()
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt string        `json:"published_at"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (c *GitHubCatalog) Check(ctx context.Context) (ReleaseInfo, error) {
	if c == nil || c.client == nil {
		return ReleaseInfo{}, errors.New("official update catalog is unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.sourceURL(c.apiURL), nil)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("create release check request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "BuildWorld system updater")
	response, err := c.client.Do(request)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("check official release: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ReleaseInfo{}, fmt.Errorf("official release check returned HTTP %d", response.StatusCode)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, maxReleaseMetadataBytes)).Decode(&release); err != nil {
		return ReleaseInfo{}, fmt.Errorf("decode official release metadata: %w", err)
	}
	if release.Draft || release.Prerelease {
		return ReleaseInfo{}, errors.New("official latest release is not stable")
	}
	version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if !versionPattern.MatchString(version) {
		return ReleaseInfo{}, fmt.Errorf("official release has invalid version %q", release.TagName)
	}
	assetName := releaseAssetName(c.goos, c.goarch)
	parts, err := selectReleaseAssets(release.Assets, assetName)
	if err != nil {
		return ReleaseInfo{}, err
	}
	checksum, ok := findAsset(release.Assets, "checksums.txt")
	if !ok {
		return ReleaseInfo{}, errors.New("official release is missing checksums.txt")
	}
	if checksum.Size <= 0 || checksum.Size > maxReleaseMetadataBytes || !isTrustedDownloadURLString(checksum.BrowserDownloadURL) {
		return ReleaseInfo{}, errors.New("official release checksums asset is invalid")
	}
	var size int64
	for _, part := range parts {
		if part.Size <= 0 || part.Size > maxReleaseAssetBytes || size > MaxBundleBytes-part.Size {
			return ReleaseInfo{}, errors.New("official release bundle is too large")
		}
		if !isTrustedDownloadURLString(part.BrowserDownloadURL) {
			return ReleaseInfo{}, fmt.Errorf("official release asset %q has an untrusted download URL", part.Name)
		}
		size += part.Size
	}
	return ReleaseInfo{
		Version:       version,
		ReleaseURL:    release.HTMLURL,
		PublishedAt:   release.PublishedAt,
		AssetName:     assetName,
		AssetSize:     size,
		checksumAsset: checksum,
		parts:         parts,
	}, nil
}

func (c *GitHubCatalog) Download(ctx context.Context, release ReleaseInfo) (Bundle, error) {
	if c == nil || c.client == nil {
		return Bundle{}, errors.New("official update catalog is unavailable")
	}
	if !versionPattern.MatchString(release.Version) || len(release.parts) == 0 || release.checksumAsset.Name != "checksums.txt" {
		return Bundle{}, errors.New("invalid official release metadata")
	}
	checksumsData, err := c.downloadAsset(ctx, release.checksumAsset, maxReleaseMetadataBytes)
	if err != nil {
		return Bundle{}, fmt.Errorf("download official checksums: %w", err)
	}
	if err := verifyAssetDigest(release.checksumAsset, checksumsData); err != nil {
		return Bundle{}, fmt.Errorf("verify official checksums: %w", err)
	}
	checksums, err := parseChecksums(checksumsData)
	if err != nil {
		return Bundle{}, err
	}
	expectedBundleChecksum, ok := checksums[release.AssetName]
	if !ok {
		return Bundle{}, fmt.Errorf("official checksums are missing %s", release.AssetName)
	}
	if len(release.parts) > 4096 {
		return Bundle{}, errors.New("official release contains too many bundle parts")
	}
	sort.Slice(release.parts, func(i, j int) bool { return release.parts[i].Name < release.parts[j].Name })

	temporary, err := os.CreateTemp("", "buildworld-update-*.bundle")
	if err != nil {
		return Bundle{}, fmt.Errorf("create temporary update bundle: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	hash := sha256.New()
	var total int64
	for _, part := range release.parts {
		expectedPartChecksum, ok := checksums[part.Name]
		if !ok {
			cleanup()
			return Bundle{}, fmt.Errorf("official checksums are missing %s", part.Name)
		}
		data, downloadErr := c.downloadAsset(ctx, part, part.Size+1)
		if downloadErr != nil {
			cleanup()
			return Bundle{}, fmt.Errorf("download official bundle part %s: %w", part.Name, downloadErr)
		}
		if err := verifyChecksum(data, expectedPartChecksum); err != nil {
			cleanup()
			return Bundle{}, fmt.Errorf("verify official bundle part %s: %w", part.Name, err)
		}
		if err := verifyAssetDigest(part, data); err != nil {
			cleanup()
			return Bundle{}, fmt.Errorf("verify official bundle part %s digest: %w", part.Name, err)
		}
		if total > MaxBundleBytes-int64(len(data)) {
			cleanup()
			return Bundle{}, errors.New("official release bundle is too large")
		}
		if _, err := io.Copy(io.MultiWriter(temporary, hash), bytes.NewReader(data)); err != nil {
			cleanup()
			return Bundle{}, fmt.Errorf("assemble official update bundle: %w", err)
		}
		total += int64(len(data))
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return Bundle{}, fmt.Errorf("sync temporary update bundle: %w", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expectedBundleChecksum {
		cleanup()
		return Bundle{}, fmt.Errorf("assembled official bundle checksum mismatch: got %s", actual)
	}
	if release.AssetSize > 0 && release.AssetSize != total {
		cleanup()
		return Bundle{}, fmt.Errorf("assembled official bundle size mismatch: %d != %d", total, release.AssetSize)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return Bundle{}, fmt.Errorf("close temporary update bundle: %w", err)
	}
	reader, err := os.Open(temporaryPath)
	if err != nil {
		_ = os.Remove(temporaryPath)
		return Bundle{}, fmt.Errorf("open temporary update bundle: %w", err)
	}
	return Bundle{Version: release.Version, SHA256: actual, Reader: &temporaryBundleReader{File: reader, path: temporaryPath}}, nil
}

type temporaryBundleReader struct {
	*os.File
	path string
}

func (r *temporaryBundleReader) Close() error {
	err := r.File.Close()
	if removeErr := os.Remove(r.path); err == nil {
		err = removeErr
	}
	return err
}

func (c *GitHubCatalog) downloadAsset(ctx context.Context, asset githubAsset, limit int64) ([]byte, error) {
	if asset.Size < 0 || asset.Size >= limit {
		return nil, fmt.Errorf("asset size %d exceeds limit", asset.Size)
	}
	if !isTrustedDownloadURLString(asset.BrowserDownloadURL) {
		return nil, errors.New("asset download URL is not trusted")
	}
	source := c.sourceURL(asset.BrowserDownloadURL)
	parsedSource, err := url.Parse(source)
	if err != nil || !c.isTrustedRequestURL(parsedSource) {
		return nil, errors.New("asset source URL is not trusted")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, fmt.Errorf("create asset request: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "BuildWorld system updater")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset download returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != asset.Size {
		return nil, fmt.Errorf("asset size mismatch: got %d, expected %d", len(data), asset.Size)
	}
	return data, nil
}

func selectReleaseAssets(assets []githubAsset, archiveName string) ([]githubAsset, error) {
	partPrefix := archiveName + ".part"
	parts := make([]githubAsset, 0)
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, partPrefix) {
			suffix := strings.TrimPrefix(asset.Name, partPrefix)
			if !partNamePattern.MatchString(asset.Name) || !partNumberPattern.MatchString(suffix) {
				return nil, fmt.Errorf("official release has invalid bundle part %q", asset.Name)
			}
			parts = append(parts, asset)
		}
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("official release is missing parts for %s", archiveName)
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].Name < parts[j].Name })
	for index, part := range parts {
		expected := fmt.Sprintf("%s.part%03d", archiveName, index)
		if part.Name != expected {
			return nil, fmt.Errorf("official release bundle parts are not contiguous at %q", expected)
		}
	}
	return parts, nil
}

func findAsset(assets []githubAsset, name string) (githubAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return githubAsset{}, false
}

func releaseAssetName(goos, goarch string) string {
	extension := "tar.gz"
	if goos == "windows" {
		extension = "zip"
	}
	return fmt.Sprintf("buildworld-%s-%s.%s", goos, goarch, extension)
}

func parseChecksums(data []byte) (map[string]string, error) {
	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 {
			return nil, fmt.Errorf("official checksums contain malformed line")
		}
		hash, err := hex.DecodeString(fields[0])
		if err != nil || len(hash) != sha256.Size {
			return nil, errors.New("official checksums contain invalid SHA-256")
		}
		name := strings.TrimPrefix(fields[1], "*")
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("official checksums contain duplicate %s", name)
		}
		result[name] = strings.ToLower(fields[0])
	}
	if len(result) == 0 {
		return nil, errors.New("official checksums are empty")
	}
	return result, nil
}

func verifyChecksum(data []byte, expected string) error {
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return fmt.Errorf("SHA-256 mismatch: got %s", actual)
	}
	return nil
}

func verifyAssetDigest(asset githubAsset, data []byte) error {
	digest := strings.TrimSpace(asset.Digest)
	if digest == "" {
		return nil
	}
	if !strings.HasPrefix(strings.ToLower(digest), "sha256:") {
		return errors.New("asset digest is not SHA-256")
	}
	return verifyChecksum(data, strings.TrimSpace(digest[len("sha256:"):]))
}

func isTrustedDownloadURLString(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && isTrustedDownloadURL(parsed)
}

func isTrustedDownloadURL(value *url.URL) bool {
	if value == nil || value.Scheme != "https" || value.User != nil {
		return false
	}
	host := strings.ToLower(value.Hostname())
	return host == "github.com" || host == "api.github.com" || host == "objects.githubusercontent.com" || host == "release-assets.githubusercontent.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

func CompareVersions(left, right string) int {
	leftParts := parseVersion(left)
	rightParts := parseVersion(right)
	for index := range leftParts {
		if leftParts[index] < rightParts[index] {
			return -1
		}
		if leftParts[index] > rightParts[index] {
			return 1
		}
	}
	return 0
}

func IsNewerVersion(current, candidate string) bool {
	return versionPattern.MatchString(strings.TrimPrefix(strings.TrimSpace(candidate), "v")) && versionPattern.MatchString(strings.TrimPrefix(strings.TrimSpace(current), "v")) && CompareVersions(current, candidate) < 0
}

func parseVersion(value string) [3]int {
	var result [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(value), "v"), ".")
	for index := 0; index < len(parts) && index < len(result); index++ {
		result[index], _ = strconv.Atoi(parts[index])
	}
	return result
}

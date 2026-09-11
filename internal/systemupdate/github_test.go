package systemupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
)

type releaseRoundTripper struct {
	responses map[string][]byte
}

func (t releaseRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	body, ok := t.responses[request.URL.Path]
	if !ok {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil)), Request: request}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header), Request: request}, nil
}

func TestGitHubCatalogAssemblesChecksummedMultipartRelease(t *testing.T) {
	part0 := []byte("first part\n")
	part1 := []byte("second part\n")
	bundle := append(append([]byte{}, part0...), part1...)
	archiveName := "buildworld-darwin-arm64.tar.gz"
	checksums := fmt.Sprintf("%s  %s\n%s  %s\n%s  %s\n",
		digest(bundle), archiveName,
		digest(part0), archiveName+".part000",
		digest(part1), archiveName+".part001",
	)
	checksumsDigest := digest([]byte(checksums))
	partURL := func(name string) string {
		return "https://github.com/neko233-com/buildworld/releases/download/v1.20.0/" + url.PathEscape(name)
	}
	releaseJSON := fmt.Sprintf(`{
  "tag_name":"v1.20.0",
  "html_url":"https://github.com/neko233-com/buildworld/releases/tag/v1.20.0",
  "published_at":"2026-09-11T01:02:03Z",
  "draft":false,
  "prerelease":false,
  "assets":[
    {"name":"checksums.txt","size":%d,"digest":"sha256:%s","browser_download_url":%q},
    {"name":%q,"size":%d,"digest":"sha256:%s","browser_download_url":%q},
    {"name":%q,"size":%d,"digest":"sha256:%s","browser_download_url":%q}
  ]
}`,
		len(checksums), checksumsDigest, "https://github.com/neko233-com/buildworld/releases/download/v1.20.0/checksums.txt",
		archiveName+".part000", len(part0), digest(part0), partURL(archiveName+".part000"),
		archiveName+".part001", len(part1), digest(part1), partURL(archiveName+".part001"),
	)
	transport := releaseRoundTripper{responses: map[string][]byte{
		"/repos/neko233-com/buildworld/releases/latest":                                 []byte(releaseJSON),
		"/neko233-com/buildworld/releases/download/v1.20.0/checksums.txt":               []byte(checksums),
		"/neko233-com/buildworld/releases/download/v1.20.0/" + archiveName + ".part000": part0,
		"/neko233-com/buildworld/releases/download/v1.20.0/" + archiveName + ".part001": part1,
	}}
	catalog := &GitHubCatalog{
		client: &http.Client{Transport: transport},
		apiURL: "https://api.github.com/repos/neko233-com/buildworld/releases/latest",
		goos:   "darwin",
		goarch: "arm64",
	}
	release, err := catalog.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "1.20.0" || release.AssetName != archiveName || release.AssetSize != int64(len(bundle)) || len(release.parts) != 2 {
		t.Fatalf("release = %#v", release)
	}
	assembled, err := catalog.Download(context.Background(), release)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(assembled.Reader)
	closeErr := assembled.Reader.Close()
	if err != nil || closeErr != nil || !bytes.Equal(data, bundle) || assembled.SHA256 != digest(bundle) {
		t.Fatalf("assembled bundle: data=%q sha=%s readErr=%v closeErr=%v", data, assembled.SHA256, err, closeErr)
	}
}

func TestVersionComparisonRequiresStableSemanticVersions(t *testing.T) {
	if !IsNewerVersion("1.9.9", "1.10.0") || IsNewerVersion("1.10.0", "1.9.9") || IsNewerVersion("dev", "1.20.0") {
		t.Fatal("unexpected semantic version comparison")
	}
	if CompareVersions("v1.2.3", "1.2.4") >= 0 {
		t.Fatal("expected v1.2.3 to be older")
	}
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

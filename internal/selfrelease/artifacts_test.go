package selfrelease

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyArtifactsRequiresAllPlatformsAndMatchingBundleMetadata(t *testing.T) {
	root := t.TempDir()
	commit := strings.Repeat("a", 40)
	metadata, err := json.Marshal(releaseMetadata{SchemaVersion: 1, Repository: "https://github.com/neko233-com/buildworld", Version: "v1.2.3", SourceCommit: commit})
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	compressed := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(compressed)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "./release-info.json", Mode: 0o644, Size: int64(len(metadata))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(metadata); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"buildworld", "buildworld-server", "buildworld-worker", "apply-update.sh", "web/dist/index.html", "sdk/pipeline/index.d.ts", "sdk/pipeline/index.js", "sdk/pipeline/package.json"} {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	assets := map[string][]byte{
		"buildworld-windows-amd64.zip": []byte("zip"), "buildworld-windows-arm64.zip": []byte("zip"),
		"buildworld-linux-amd64.tar.gz": []byte("linux"), "buildworld-linux-arm64.tar.gz": []byte("linux"),
		"buildworld-darwin-amd64.tar.gz": []byte("mac"), "buildworld-darwin-arm64.tar.gz": archive.Bytes(),
		"install.ps1": []byte("installer"), "install.sh": []byte("installer"), "release-info.json": metadata,
	}
	var manifest strings.Builder
	for name, data := range assets {
		filename := filepath.Join(root, name)
		if err := os.WriteFile(filename, data, 0o600); err != nil {
			t.Fatal(err)
		}
		hash, err := fileSHA256(filename)
		if err != nil {
			t.Fatal(err)
		}
		manifest.WriteString(hash + "  " + name + "\n")
	}
	if err := os.WriteFile(filepath.Join(root, "checksums.txt"), []byte(manifest.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	filename, hash, err := verifyArtifacts(root, "1.2.3", commit, "darwin-arm64")
	if err != nil || filepath.Base(filename) != "buildworld-darwin-arm64.tar.gz" || len(hash) != 64 {
		t.Fatalf("bundle=%s hash=%s error=%v", filename, hash, err)
	}
	if _, _, err := verifyArtifacts(root, "1.2.4", commit, "darwin-arm64"); err == nil {
		t.Fatal("accepted metadata for another version")
	}
	if err := os.WriteFile(filepath.Join(root, "buildworld-linux-arm64.tar.gz"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := verifyArtifacts(root, "1.2.3", commit, "darwin-arm64"); err == nil {
		t.Fatal("accepted a tampered non-host platform")
	}
}

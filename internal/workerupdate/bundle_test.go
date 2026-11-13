package workerupdate

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func makeBundle(t *testing.T, dir, extra string) string {
	t.Helper()
	file := filepath.Join(dir, "release.zip")
	out, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(out)
	worker := "buildworld-worker"
	if runtime.GOOS == "windows" {
		worker += ".exe"
	}
	self, _ := os.Executable()
	binary, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{worker: binary, "release-info.json": []byte(fmt.Sprintf(`{"schemaVersion":1,"repository":"https://github.com/neko233-com/buildworld","version":"v1.17.0","sourceCommit":%q}`, strings.Repeat("a", 40))), "web/dist/index.html": []byte("html"), "sdk/pipeline/index.js": {}, "sdk/pipeline/index.d.ts": {}, "sdk/pipeline/package.json": []byte("{}")}
	if extra != "" {
		files[extra] = []byte("rejected")
	}
	for name, data := range files {
		entry, e := archive.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = entry.Write(data); e != nil {
			t.Fatal(e)
		}
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	out.Close()
	return file
}

func TestWorkerBundleValidatesWholeUploadPlatformAndIdentity(t *testing.T) {
	archive := makeBundle(t, t.TempDir(), "")
	identity, err := Extract(archive, t.TempDir(), "1.17.0", runtime.GOOS+"/"+runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if !DigestPattern.MatchString(identity.BinarySHA256) || identity.SourceCommit != strings.Repeat("a", 40) {
		t.Fatal(identity)
	}
	if _, err = Extract(archive, t.TempDir(), "1.17.1", runtime.GOOS+"/"+runtime.GOARCH); err == nil {
		t.Fatal("wrong version accepted")
	}
	arch := "arm64"
	if runtime.GOARCH == arch {
		arch = "amd64"
	}
	if _, err = Extract(archive, t.TempDir(), "1.17.0", runtime.GOOS+"/"+arch); err == nil {
		t.Fatal("wrong platform accepted")
	}
	data, _ := os.ReadFile(archive)
	sum := sha256.Sum256(data)
	destination := filepath.Join(t.TempDir(), "bundle")
	if err = StageArchive(bytes.NewReader(data), destination, strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong upload checksum accepted")
	}
	if _, err = os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("failed upload was retained")
	}
	if err = StageArchive(bytes.NewReader(data), destination, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerBundleRejectsUnsafePathsBeforeInstallation(t *testing.T) {
	for _, path := range []string{"../escape", "/absolute", "folder/../escape", "C:/drive", "folder\\escape"} {
		t.Run(path, func(t *testing.T) {
			archive := makeBundle(t, t.TempDir(), path)
			if _, err := Extract(archive, t.TempDir(), "1.17.0", runtime.GOOS+"/"+runtime.GOARCH); err == nil {
				t.Fatal("unsafe path accepted")
			}
		})
	}
}

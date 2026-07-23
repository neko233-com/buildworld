package systemupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerStagesVerifiedBundleAndLocksConcurrentUpdate(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "buildworld")
	stateDir := filepath.Join(root, "state")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(installDir, "apply-update.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	var launched bool
	manager, err := NewManager(Options{
		CurrentVersion: "1.0.0",
		ConfigPath:     filepath.Join(root, "config.yaml"),
		InstallDir:     installDir,
		StateDir:       stateDir,
		ScriptPath:     script,
		Launch: func(_ string, args []string, _ string) error {
			launched = true
			if len(args) != 8 || args[2] != "1.0.1" {
				t.Fatalf("launch args = %#v", args)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle := completeBundle(t)
	sum := sha256.Sum256(bundle)
	status, err := manager.Start(context.Background(), Request{
		Version: "v1.0.1",
		SHA256:  hex.EncodeToString(sum[:]),
		Bundle:  bytes.NewReader(bundle),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !launched || status.Status != "accepted" || status.Version != "1.0.1" || status.OperationID == "" {
		t.Fatalf("status = %#v, launched = %t", status, launched)
	}
	if manager.Status().OperationID != status.OperationID {
		t.Fatalf("persisted status = %#v", manager.Status())
	}
	if _, err := manager.Start(context.Background(), Request{
		Version: "1.0.2",
		SHA256:  hex.EncodeToString(sum[:]),
		Bundle:  bytes.NewReader(bundle),
	}); err != ErrUpdateInProgress {
		t.Fatalf("concurrent update error = %v, want %v", err, ErrUpdateInProgress)
	}
}

func TestManagerRejectsChecksumMismatchAndReleasesLock(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "buildworld")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(installDir, "apply-update.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(Options{
		InstallDir: installDir,
		StateDir:   filepath.Join(root, "state"),
		ScriptPath: script,
		Launch:     func(string, []string, string) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Start(context.Background(), Request{
		Version: "1.0.1",
		SHA256:  strings.Repeat("0", 64),
		Bundle:  bytes.NewReader(completeBundle(t)),
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("checksum error = %v", err)
	}
	if _, statErr := os.Stat(manager.lockPath()); !os.IsNotExist(statErr) {
		t.Fatalf("lock remains after rejected bundle: %v", statErr)
	}
}

func TestValidateBundleRejectsTraversal(t *testing.T) {
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	tarWriter.Close()
	gzipWriter.Close()
	path := filepath.Join(t.TempDir(), "unsafe.tar.gz")
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateBundle(path); err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("validateBundle error = %v", err)
	}
}

func completeBundle(t *testing.T) []byte {
	t.Helper()
	files := map[string]struct {
		mode int64
		body string
	}{
		"buildworld":                {0o755, "cli"},
		"buildworld-server":         {0o755, "server"},
		"buildworld-worker":         {0o755, "worker"},
		"apply-update.sh":           {0o755, "#!/bin/sh\n"},
		"web/dist/index.html":       {0o644, "<html></html>"},
		"sdk/pipeline/index.d.ts":   {0o644, "export {}"},
		"sdk/pipeline/index.js":     {0o644, "export {}"},
		"sdk/pipeline/package.json": {0o644, "{}"},
	}
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, file := range files {
		if err := tarWriter.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     file.mode,
			Size:     int64(len(file.body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(file.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

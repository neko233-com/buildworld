//go:build darwin

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStopMacOSLaunchAgentReportsUnsupportedDomain(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	launcher := filepath.Join(dir, "launchctl")
	script := "#!/bin/sh\ncase \"$1\" in\nbootout) echo 'Domain does not support specified action' >&2; exit 125;;\nlist) printf 'PID\\tStatus\\tLabel\\n65118\\t0\\tcom.buildworld.server\\n';;\n*) exit 1;;\nesac\n"
	if err := os.WriteFile(launcher, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	stopped, err := stopAutostartService()
	if stopped || err == nil || !strings.Contains(err.Error(), "Domain does not support") {
		t.Fatalf("stopped=%v err=%v", stopped, err)
	}
}

func TestWriteMacOSAutostartFileReplacesAtomicallyWithRequestedMode(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, macOSAutostartLauncherName)
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeMacOSAutostartFile(path, []byte("new\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new\n" {
		t.Fatalf("contents = %q", contents)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %o, want 700", info.Mode().Perm())
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != macOSAutostartLauncherName {
		t.Fatalf("temporary files remain after replacement: %#v", entries)
	}
}

func TestSecureMacOSAutostartLogPreservesContentsAndRestrictsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	if err := os.WriteFile(path, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := secureMacOSAutostartLog(path); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "existing\n" {
		t.Fatalf("contents = %q", contents)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

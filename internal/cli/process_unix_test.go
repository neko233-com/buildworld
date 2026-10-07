//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestUnixStopProcessPreservesGracefulCleanup(t *testing.T) {
	dir := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestUnixStopProcessHelper$")
	command.Env = append(os.Environ(), "BUILDWORLD_STOP_HELPER="+dir)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := stopProcess(command.Process); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("helper was force-killed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "drained")); err != nil {
		t.Fatalf("graceful cleanup skipped: %v", err)
	}
}

func TestUnixStopProcessHelper(t *testing.T) {
	dir := os.Getenv("BUILDWORLD_STOP_HELPER")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	if err := os.WriteFile(filepath.Join(dir, "ready"), nil, 0600); err != nil {
		os.Exit(2)
	}
	<-signals
	if err := os.WriteFile(filepath.Join(dir, "drained"), nil, 0600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestUnixProcessNameMatchIsCaseSensitive(t *testing.T) {
	if sameProcessName("BuildWorld-Server", "buildworld-server") {
		t.Fatal("Unix process names should match case-sensitively")
	}
}

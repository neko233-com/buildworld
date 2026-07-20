package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWatchServiceStreamsAndCancelsWithoutStoppingService(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "service.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "service.log"), []byte("before monitor\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lines := make(chan string, 16)
	done := make(chan error, 1)
	go func() {
		done <- WatchService(ctx, dir, map[string]string{
			"pid_file": "service.pid", "log_file": "service.log", "initial_lines": "0",
		}, func(line string) { lines <- line })
	}()
	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "service.log"), []byte("before monitor\nlive line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(650 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchService() error = %v, want cancellation", err)
	}
	close(lines)
	joined := ""
	for line := range lines {
		joined += line + "\n"
	}
	if !strings.Contains(joined, "live line") {
		t.Fatalf("forwarded lines = %q", joined)
	}
}

func TestWatchServiceRejectsMissingPIDFile(t *testing.T) {
	err := WatchService(context.Background(), t.TempDir(), map[string]string{"pid_file": "missing", "log_file": "service.log"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "PID file") {
		t.Fatalf("WatchService() error = %v", err)
	}
}

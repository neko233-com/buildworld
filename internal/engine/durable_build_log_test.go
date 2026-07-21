package engine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestDurableBuildLogTimerAndTerminalFlushDoNotLoseTail(t *testing.T) {
	data, build := newDurableLogTestStore(t)
	runner := NewBuildRunner(data, nil, filepath.Join(t.TempDir(), "workspace"), nil)

	runner.log(build.ID, "watch", "first")
	runner.log(build.ID, "watch", "timer-tail")
	waitForDurableLog(t, data, build.ID, "timer-tail")

	runner.log(build.ID, "watch", "terminal-tail")
	runner.broadcastStatus(build.ID, "success", "", 1)
	stored, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.Log, "terminal-tail") {
		t.Fatalf("terminal flush lost tail:\n%s", stored.Log)
	}
	runner.durableLogMu.Lock()
	_, retained := runner.durableLogs[build.ID]
	runner.durableLogMu.Unlock()
	if retained {
		t.Fatal("terminal flush retained per-build timer state")
	}
}

func TestDurableBuildLogAppendFailureRetainsRetriesAndCleansTerminalState(t *testing.T) {
	data, build := newDurableLogTestStore(t)
	runner := NewBuildRunner(data, nil, filepath.Join(t.TempDir(), "workspace"), nil)

	injected := errors.New("injected SQLite append failure")
	var attempts atomic.Int32
	reported := make(chan error, 4)
	runner.durableLogAppend = func(buildID int64, content string) error {
		if attempts.Add(1) <= 2 {
			return injected
		}
		return data.AppendBuildLog(buildID, content)
	}
	runner.durableLogErrorReporter = func(_ int64, err error) {
		reported <- err
	}

	if err := runner.log(build.ID, "watch", "retained-after-failure"); !errors.Is(err, injected) {
		t.Fatalf("first append error was not observable to caller: %v", err)
	}
	if err := runner.log(build.ID, "watch", "buffered-during-backoff"); err != nil {
		t.Fatalf("new output should join retained retry batch: %v", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("live output bypassed durable-log retry backoff: %d attempts", got)
	}
	runner.broadcastStatus(build.ID, "success", "", 1)

	waitForDurableLog(t, data, build.ID, "retained-after-failure")
	waitForDurableLogStateCount(t, runner, 0)
	if attempts.Load() < 3 {
		t.Fatalf("durable log did not retry terminal batch: %d attempt(s)", attempts.Load())
	}
	select {
	case err := <-reported:
		if !errors.Is(err, injected) {
			t.Fatalf("reported wrong persistence error: %v", err)
		}
	default:
		t.Fatal("durable log append failure was not reported")
	}

	stored, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(stored.Log, "retained-after-failure"); count != 1 {
		t.Fatalf("retried durable batch persisted %d copies:\n%s", count, stored.Log)
	}
	if count := strings.Count(stored.Log, "buffered-during-backoff"); count != 1 {
		t.Fatalf("output buffered during retry persisted %d copies:\n%s", count, stored.Log)
	}
}

func TestAutomaticRetryDiagnosticsDoNotRecreateTerminalDurableLogState(t *testing.T) {
	data, build := newDurableLogTestStore(t)
	runner := NewBuildRunner(data, nil, filepath.Join(t.TempDir(), "workspace"), nil)
	project, err := data.GetProject(build.ProjectID)
	if err != nil {
		t.Fatal(err)
	}

	runner.fail(build.ID, time.Now(), "injected build failure", project)

	runner.durableLogMu.Lock()
	_, retained := runner.durableLogs[build.ID]
	runner.durableLogMu.Unlock()
	if retained {
		t.Fatal("automatic retry diagnostics recreated terminal durable log state")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		builds, listErr := data.ListBuildsByProject(project.ID)
		if listErr == nil && len(builds) == 2 && builds[0].Status == "success" && !runner.isBuildRunning(builds[0].ID) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	builds, err := data.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 2 || builds[0].Status != "success" {
		t.Fatalf("automatic retry did not complete: %+v", builds)
	}
	waitForDurableLogStateCount(t, runner, 0)

	failed, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(failed.Log, "Automatic retry scheduled as build #2") {
		t.Fatalf("failed build lacks automatic retry diagnostics:\n%s", failed.Log)
	}
}

func TestDurableBuildLogConcurrentPressureStaysBounded(t *testing.T) {
	data, build := newDurableLogTestStore(t)
	runner := NewBuildRunner(data, nil, filepath.Join(t.TempDir(), "workspace"), nil)

	const writers = 8
	const linesPerWriter = 2_000
	var wait sync.WaitGroup
	for writer := range writers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for line := range linesPerWriter {
				runner.log(build.ID, "watch", fmt.Sprintf("writer=%d line=%d %s", writer, line, strings.Repeat("x", 64)))
			}
		}()
	}
	wait.Wait()
	for writer := range writers {
		runner.log(build.ID, "watch", fmt.Sprintf("final-sentinel-writer=%d", writer))
	}
	runner.broadcastStatus(build.ID, "cancelled", "", 1)

	stored, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if utf8.RuneCountInString(stored.Log) > store.BuildLogRetentionCharacters {
		t.Fatalf("durable log exceeded retention: %d bytes", len(stored.Log))
	}
	if !store.IsBuildLogTruncated(stored.Log) {
		t.Fatal("pressure log did not report durable-history truncation")
	}
	for writer := range writers {
		last := fmt.Sprintf("final-sentinel-writer=%d", writer)
		if !strings.Contains(stored.Log, last) {
			t.Fatalf("terminal retained tail does not include %q", last)
		}
	}
}

func TestBoundLiveBuildLogLineIsUTF8AndSizeBounded(t *testing.T) {
	line := strings.Repeat("月", liveBuildLogLineMaxBytes) + "newest"
	bounded := boundLiveBuildLogLine(line)
	if !utf8.ValidString(bounded) || len(bounded) > liveBuildLogLineMaxBytes+len(store.BuildLogOversizedMarker) {
		t.Fatalf("bounded live line is invalid or too large: %d bytes", len(bounded))
	}
	if !strings.HasPrefix(bounded, store.BuildLogOversizedMarker) || !strings.HasSuffix(bounded, "newest") {
		t.Fatalf("bounded live line lacks marker or newest tail")
	}
}

func newDurableLogTestStore(t *testing.T) (*store.Store, *store.Build) {
	t.Helper()
	data, err := store.New(filepath.Join(t.TempDir(), "build-log.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	project, err := data.CreateProject("durable-log", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return data, build
}

func waitForDurableLog(t *testing.T, data *store.Store, buildID int64, expected string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		build, err := data.GetBuild(buildID)
		if err == nil && strings.Contains(build.Log, expected) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for durable log %q", expected)
}

func waitForDurableLogStateCount(t *testing.T, runner *BuildRunner, expected int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runner.durableLogMu.Lock()
		count := len(runner.durableLogs)
		runner.durableLogMu.Unlock()
		if count == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	runner.durableLogMu.Lock()
	count := len(runner.durableLogs)
	runner.durableLogMu.Unlock()
	t.Fatalf("durable log state count = %d, want %d", count, expected)
}

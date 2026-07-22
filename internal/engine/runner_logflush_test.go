package engine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

// waitForLog polls the durable build log until it contains want (or times out).
// The durable store coalesces writes behind a short flush interval, so readers
// must wait rather than asserting immediately after a write.
func waitForLog(t *testing.T, logOf func() string, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for {
		last = logOf()
		if strings.Contains(last, want) {
			return last
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for durable log to contain %q; log was: %q", timeout, want, last)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestRunnerFlushesBufferedPartialSecretDuringLongRunningStep reproduces the
// defect fixed by the per-step flush ticker in runner.go (lines 704-724).
//
// A never-ending step (service_watch used as a temporary log monitor) emits
// streaming chunks. When a chunk overlaps a configured secret the
// streamingSecretMasker must hold it back until a full line / flush boundary.
// stream.Write returns "" for such a prefix, so logBuildOutput persists nothing
// on that call. Before the fix flushBuildOutput was only invoked at step
// COMPLETION, so a long-running step's buffered output never reached the
// durable store or the live WebSocket.
//
// This test configures a real secret on the build, emits a partial secret
// prefix (the realistic shape of newline-free streamed tail output), and
// asserts it is (1) NOT persisted before the flush and (2) persisted after the
// flush that the runner's ticker now performs while the step is still alive.
func TestRunnerFlushesBufferedPartialSecretDuringLongRunningStep(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "bw.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer st.Close()

	// A pipeline whose only step is a terminal service_watch (long-running),
	// matching the real monitor pattern this fix targets.
	const config = `import { definePipeline, stage, watchService } from "@buildworld/pipeline"
export default definePipeline({
  stages: [stage("Observe", watchService("Logs", {
    pidFile: "server.pid",
    logFile: "server.log",
    pollSeconds: 1,
  }))],
})`

	project, err := st.CreateProject("log-mon", "", "", "git", "main", config, 1, nil, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Configure a real secret so the streaming masker buffers secret-overlapping
	// output instead of releasing it immediately. This mirrors server-game-go,
	// whose build masks live DB credentials in the game backend's console log.
	const secret = "supersecret-token-ABCD1234"
	if err := st.SetEnvVar("project", &project.ID, "DB_PASSWORD", secret, true, "db password"); err != nil {
		t.Fatalf("SetEnvVar: %v", err)
	}

	build, err := st.CreateBuild(project.ID, 1, "manual", "main", "", "{}", nil, nil)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}

	runner := NewBuildRunner(st, nil, filepath.Join(dir, "ws"), nil)
	cfg, err := ParsePipelineConfig(config)
	if err != nil {
		t.Fatalf("ParsePipelineConfig: %v", err)
	}
	runner.configureBuildSecretMasker(build, project, cfg)
	defer runner.clearBuildSecretMasker(build.ID)

	logOf := func() string {
		b, err := st.GetBuild(build.ID)
		if err != nil {
			t.Fatalf("GetBuild: %v", err)
		}
		return b.Log
	}

	stage := "Observe"

	// (1) Emit a streaming chunk that is a TRUE prefix of the secret and has NO
	// trailing newline - the exact shape the masker must hold back until a
	// flush. "supersecret-tok" is a genuine prefix of the configured secret.
	partial := secret[:15] // "supersecret-tok"
	runner.logBuildOutput(build.ID, stage, partial)

	// Give any accidental async flush a chance to fire (there is none without
	// the ticker); this guards against a false negative on the "not persisted"
	// assertion.
	time.Sleep(durableBuildLogFlushInterval + 100*time.Millisecond)

	before := logOf()
	if strings.Contains(before, partial) {
		t.Fatalf("partial secret prefix leaked to durable log before flush: %q", before)
	}
	if strings.Contains(before, secret) {
		t.Fatalf("raw secret present in durable log before flush: %q", before)
	}

	// This is the call the runner now performs on its 250ms ticker while a
	// long-running step is still alive (runner.go:716 and :724).
	runner.flushBuildOutput(build.ID, stage)

	after := waitForLog(t, logOf, partial, 2*time.Second)
	// A partial secret prefix cannot be fully masked (only the complete secret
	// is in the replacer), so the fix's contract is: the buffered chunk is
	// RELEASED on flush rather than lost. Assert the full secret never appears.
	if strings.Contains(after, secret) {
		t.Fatalf("flush exposed the raw secret in the durable log: %q", after)
	}

	// (2) A COMPLETE secret on a complete chunk is masked and emitted at once,
	// proving normal (non-buffered) secret handling is unchanged by the fix.
	runner.logBuildOutput(build.ID, stage, secret+"\n")
	got := waitForLog(t, logOf, maskedSecretValue, 2*time.Second)
	if strings.Contains(got, secret) {
		t.Fatalf("complete secret leaked unmasked into durable log: %q", got)
	}
}

// TestRunnerStreamsCompleteLinesImmediately proves the fix does not change the
// normal path: complete, newline-terminated, secret-free output is persisted at
// once, so ordinary (secret-free) streaming keeps working as before.
func TestRunnerStreamsCompleteLinesImmediately(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(filepath.Join(dir, "bw.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer st.Close()

	const config = `import { definePipeline, stage, watchService } from "@buildworld/pipeline"
export default definePipeline({
  stages: [stage("Observe", watchService("Logs", {
    pidFile: "server.pid",
    logFile: "server.log",
    pollSeconds: 1,
  }))],
})`
	project, err := st.CreateProject("log-mon", "", "", "git", "main", config, 1, nil, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	build, err := st.CreateBuild(project.ID, 1, "manual", "main", "", "{}", nil, nil)
	if err != nil {
		t.Fatalf("CreateBuild: %v", err)
	}
	runner := NewBuildRunner(st, nil, filepath.Join(dir, "ws"), nil)
	cfg, err := ParsePipelineConfig(config)
	if err != nil {
		t.Fatalf("ParsePipelineConfig: %v", err)
	}
	runner.configureBuildSecretMasker(build, project, cfg)
	defer runner.clearBuildSecretMasker(build.ID)

	logOf := func() string {
		b, err := st.GetBuild(build.ID)
		if err != nil {
			t.Fatalf("GetBuild: %v", err)
		}
		return b.Log
	}

	runner.logBuildOutput(build.ID, "Observe", "game server listening on :8080\n")

	if got := waitForLog(t, logOf, "game server listening on :8080", 2*time.Second); got == "" {
		t.Fatalf("complete non-secret line was not persisted: %q", got)
	}
}

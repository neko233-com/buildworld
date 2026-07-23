package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWatchServiceStreamsNewLogLinesUntilCanceled(t *testing.T) {
	target := t.TempDir()
	pidFile := filepath.Join(target, "server.pid")
	logFile := filepath.Join(target, "server.log")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprint(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logFile, []byte("old line\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := make(chan string, 32)
	result := make(chan error, 1)
	go func() {
		result <- WatchService(ctx, target, map[string]string{
			"pid_file":      "server.pid",
			"log_file":      "server.log",
			"initial_lines": "1",
		}, func(line string) {
			output <- line
		})
	}()

	waitForWatchOutput(t, output, "old line")
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("live line\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	waitForWatchOutput(t, output, "live line")

	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WatchService() error = %v, want context canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WatchService did not stop after cancellation")
	}
}

func TestWatchServiceLogFollowIsIndependentFromProcessPoll(t *testing.T) {
	target := t.TempDir()
	logFile := filepath.Join(target, "server.log")
	if err := os.WriteFile(filepath.Join(target, "server.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logFile, []byte("follow-ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	output := make(chan string, 32)
	result := make(chan error, 1)
	go func() {
		result <- WatchService(ctx, target, map[string]string{
			"pid_file":      "server.pid",
			"log_file":      "server.log",
			"initial_lines": "1",
			"poll_seconds":  "5",
		}, func(line string) {
			output <- line
		})
	}()
	waitForWatchOutput(t, output, "follow-ready")

	appendLog(t, logFile, "low-latency-line\n")
	waitForWatchOutputWithin(t, output, "low-latency-line", 1500*time.Millisecond)

	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WatchService() error = %v, want context canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WatchService did not stop after cancellation")
	}
}

func TestWatchServiceCatchesUpBurstLargerThanSingleRead(t *testing.T) {
	target := t.TempDir()
	logFile := filepath.Join(target, "server.log")
	if err := os.WriteFile(filepath.Join(target, "server.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logFile, []byte("burst-ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 1)
	caughtUp := make(chan struct{}, 1)
	result := make(chan error, 1)
	const marker = "burst-caught-up"
	go func() {
		result <- WatchService(ctx, target, map[string]string{
			"pid_file":      "server.pid",
			"log_file":      "server.log",
			"initial_lines": "1",
			"poll_seconds":  "5",
		}, func(line string) {
			if line == "burst-ready" {
				select {
				case started <- struct{}{}:
				default:
				}
			}
			if line == marker {
				select {
				case caughtUp <- struct{}{}:
				default:
				}
			}
		})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("WatchService did not start")
	}

	logLine := strings.Repeat("x", 4095) + "\n"
	payload := strings.Repeat(logLine, serviceLogReadLimit/4096*4) + marker + "\n"
	if len(payload) <= serviceLogReadLimit {
		t.Fatalf("test burst = %d bytes, want more than %d", len(payload), serviceLogReadLimit)
	}
	appendLog(t, logFile, payload)
	select {
	case <-caughtUp:
	case <-time.After(3 * time.Second):
		t.Fatalf("WatchService did not catch up %d-byte burst", len(payload))
	}

	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WatchService() error = %v, want context canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WatchService did not stop after burst cancellation")
	}
}

func TestWatchServiceRejectsPathsOutsideTarget(t *testing.T) {
	target := t.TempDir()
	err := WatchService(context.Background(), target, map[string]string{
		"pid_file": "../outside.pid",
		"log_file": "server.log",
	}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "inside target_dir") {
		t.Fatalf("WatchService() error = %v, want target_dir boundary error", err)
	}
}

func TestWatchServiceReportsFinalTailWhenPIDIsAlreadyDead(t *testing.T) {
	target := t.TempDir()
	var log strings.Builder
	for line := 1; line <= 60; line++ {
		fmt.Fprintf(&log, "line-%02d\n", line)
	}
	if err := os.WriteFile(filepath.Join(target, "server.pid"), []byte("2147483647"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "server.log"), []byte(log.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	var output []string
	err := WatchService(context.Background(), target, map[string]string{
		"pid_file": "server.pid",
		"log_file": "server.log",
	}, func(line string) {
		output = append(output, line)
	})
	joined := strings.Join(output, "\n")
	if err == nil || !strings.Contains(err.Error(), "service process exited") {
		t.Fatalf("WatchService() error = %v, want exited process", err)
	}
	if !strings.Contains(joined, "Last 50") || !strings.Contains(joined, "line-11") || !strings.Contains(joined, "line-60") {
		t.Fatalf("initial dead PID output did not contain final 50 lines:\n%s", joined)
	}
	if strings.Contains(joined, "line-10\n") {
		t.Fatalf("initial dead PID output included more than 50 lines:\n%s", joined)
	}
}

func TestWatchServiceReportsFinalTailWhenRunningPIDExits(t *testing.T) {
	target := t.TempDir()
	pidFile := filepath.Join(target, "server.pid")
	logFile := filepath.Join(target, "server.log")
	t.Setenv("BUILDWORLD_SERVICE_WATCH_HELPER", "1")
	process := exec.Command(os.Args[0], "-test.run=^TestWatchServiceProcessHelper$")
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	processReaped := false
	defer func() {
		if processReaped {
			return
		}
		_ = process.Process.Kill()
		_ = process.Wait()
	}()
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(process.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	for line := 1; line <= 60; line++ {
		fmt.Fprintf(&log, "dynamic-%02d\n", line)
	}
	if err := os.WriteFile(logFile, []byte(log.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output := make(chan string, 256)
	result := make(chan error, 1)
	go func() {
		result <- WatchService(ctx, target, map[string]string{
			"pid_file":      "server.pid",
			"log_file":      "server.log",
			"initial_lines": "0",
			"poll_seconds":  "1",
		}, func(line string) {
			output <- line
		})
	}()
	waitForWatchOutput(t, output, "continuously following")

	if err := process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := process.Wait(); err == nil {
		t.Fatal("helper process unexpectedly exited successfully after Kill")
	}
	processReaped = true

	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "service process exited") {
			t.Fatalf("WatchService() error = %v, want exited process", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("WatchService did not detect the running PID exit")
	}
	close(output)
	var lines []string
	for line := range output {
		lines = append(lines, line)
	}
	joined := strings.Join(lines, "\n")
	finalTailAt := strings.LastIndex(joined, "Last 50 service log lines:")
	if finalTailAt < 0 {
		t.Fatalf("dynamic PID exit output did not include final tail:\n%s", joined)
	}
	finalTail := joined[finalTailAt:]
	if !strings.Contains(finalTail, "dynamic-11") || !strings.Contains(finalTail, "dynamic-60") {
		t.Fatalf("dynamic PID exit final tail is incomplete:\n%s", finalTail)
	}
	if strings.Contains(finalTail, "dynamic-10\n") {
		t.Fatalf("dynamic PID exit included more than 50 final lines:\n%s", finalTail)
	}
}

func TestWatchServiceEmitsHeartbeatWhilePIDIsRunning(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "server.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "server.log"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	output := make(chan string, 32)
	result := make(chan error, 1)
	go func() {
		result <- WatchService(ctx, target, map[string]string{
			"pid_file":          "server.pid",
			"log_file":          "server.log",
			"heartbeat_seconds": "5",
			"poll_seconds":      "1",
			"port":              "8700",
		}, func(line string) {
			output <- line
		})
	}()
	waitForWatchOutputWithin(t, output, "heartbeat", 7*time.Second)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WatchService() error = %v, want context canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WatchService did not stop after heartbeat cancellation")
	}
}

func TestWatchServiceHandoverMakesExpectedReplacementSuccessful(t *testing.T) {
	target := t.TempDir()
	t.Setenv("BUILDWORLD_SERVICE_WATCH_HELPER", "1")
	process := exec.Command(os.Args[0], "-test.run=^TestWatchServiceProcessHelper$")
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if serviceProcessRunning(process.Process.Pid) {
			_ = process.Process.Kill()
		}
		_ = process.Wait()
	}()
	if err := os.WriteFile(filepath.Join(target, "server.pid"), []byte(strconv.Itoa(process.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "server.log"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	output := make(chan string, 32)
	go func() {
		result <- WatchService(context.Background(), target, map[string]string{
			"pid_file": "server.pid", "log_file": "server.log", "poll_seconds": "1",
			"owner_file": ".jenkins-monitor-owner", "handover_file": ".jenkins-monitor-handover", "owner_id": "41",
		}, func(line string) { output <- line })
	}()
	waitForWatchOutput(t, output, "continuously following")
	if contents, err := os.ReadFile(filepath.Join(target, ".jenkins-monitor-owner")); err != nil || strings.TrimSpace(string(contents)) != "41" {
		t.Fatalf("monitor owner = %q, %v", contents, err)
	}
	if err := os.WriteFile(filepath.Join(target, ".jenkins-monitor-handover"), []byte("42"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = process.Wait()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("WatchService() error = %v, want successful handover", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("WatchService did not complete after handover")
	}
	if _, err := os.Stat(filepath.Join(target, ".jenkins-monitor-owner")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("monitor owner was not released: %v", err)
	}
}

func TestWatchServiceStopsMigratedServiceOnCancellation(t *testing.T) {
	target := t.TempDir()
	pidFile := filepath.Join(target, "server.pid")
	logFile := filepath.Join(target, "server.log")
	t.Setenv("BUILDWORLD_SERVICE_WATCH_HELPER", "1")
	process := exec.Command(os.Args[0], "-test.run=^TestWatchServiceProcessHelper$")
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := make(chan error, 1)
	go func() { reaped <- process.Wait() }()
	defer func() {
		if serviceProcessRunning(process.Process.Pid) {
			_ = process.Process.Kill()
		}
		select {
		case <-reaped:
		case <-time.After(3 * time.Second):
		}
	}()
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(process.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := make(chan string, 64)
	result := make(chan error, 1)
	go func() {
		result <- WatchService(ctx, target, map[string]string{
			"pid_file": "server.pid", "log_file": "server.log",
			"stop_service_on_cancel": "true", "shutdown_timeout_seconds": "3",
		}, func(line string) { output <- line })
	}()
	waitForWatchOutput(t, output, "continuously following")
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WatchService() error = %v, want context canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WatchService did not stop the service after cancellation")
	}
	if serviceProcessRunning(process.Process.Pid) {
		t.Fatalf("service PID %d survived cancellation", process.Process.Pid)
	}
	select {
	case <-reaped:
	case <-time.After(2 * time.Second):
		t.Fatal("terminated service process was not reaped")
	}
}

func TestWatchServiceProcessHelper(t *testing.T) {
	if os.Getenv("BUILDWORLD_SERVICE_WATCH_HELPER") != "1" {
		t.Skip("helper process")
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestServiceLogFollowerInitialZeroSkipsHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	if err := os.WriteFile(path, []byte("history\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	follower := serviceLogFollower{initialLines: 0}
	var output []string
	follower.forward(path, func(line string) { output = append(output, line) })
	if len(output) != 0 {
		t.Fatalf("initial output = %v, want history skipped", output)
	}
	appendLog(t, path, "live\n")
	follower.forward(path, func(line string) { output = append(output, line) })
	if strings.Join(output, "\n") != "live" {
		t.Fatalf("output = %v, want only live line", output)
	}
}

func TestServiceLogFollowerInitialTailUsesOneOffsetSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	if err := os.WriteFile(path, []byte("history\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	follower := serviceLogFollower{initialLines: 1}
	var output []string
	follower.forward(path, func(line string) {
		output = append(output, line)
		if line == "history" {
			appendLog(t, path, "written-during-tail\n")
		}
	})
	follower.forward(path, func(line string) { output = append(output, line) })
	if strings.Join(output, "\n") != "history\nwritten-during-tail" {
		t.Fatalf("output = %v, append between tail and follow offset was lost", output)
	}
}

func TestServiceLogFollowerRecoversMissingLogWithoutStatusSpam(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	follower := serviceLogFollower{initialLines: 0}
	var output []string
	emit := func(line string) { output = append(output, line) }
	follower.forward(path, emit)
	follower.forward(path, emit)
	if countContaining(output, "unavailable") != 1 {
		t.Fatalf("missing status = %v, want exactly one unavailable message", output)
	}
	if err := os.WriteFile(path, []byte("history\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	follower.forward(path, emit)
	follower.forward(path, emit)
	if countContaining(output, "available") != 2 {
		// The single unavailable line and the single available line both
		// contain the word "available".
		t.Fatalf("recovery status = %v, want one recovery message", output)
	}
	appendLog(t, path, "live\n")
	follower.forward(path, emit)
	if output[len(output)-1] != "live" || strings.Contains(strings.Join(output, "\n"), "history") {
		t.Fatalf("recovered output = %v, want new data only", output)
	}
}

func TestServiceLogFollowerResumesSameFileAfterTemporaryDisappearance(t *testing.T) {
	target := t.TempDir()
	path := filepath.Join(target, "server.log")
	hidden := filepath.Join(target, "server.log.hidden")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	follower := serviceLogFollower{initialLines: 0}
	var output []string
	emit := func(line string) { output = append(output, line) }
	follower.forward(path, emit)

	if err := os.Rename(path, hidden); err != nil {
		t.Fatal(err)
	}
	follower.forward(path, emit)
	follower.forward(path, emit)
	if countContaining(output, "unavailable") != 1 {
		t.Fatalf("temporary missing status = %v", output)
	}
	if err := os.Rename(hidden, path); err != nil {
		t.Fatal(err)
	}
	appendLog(t, path, "after-recovery\n")
	follower.forward(path, emit)
	follower.forward(path, emit)
	if countContaining(output, "available again") != 1 || output[len(output)-1] != "after-recovery" {
		t.Fatalf("same-file recovery output = %v", output)
	}
}

func TestServiceLogFollowerRecoversFromTruncateAndRecreate(t *testing.T) {
	target := t.TempDir()
	path := filepath.Join(target, "server.log")
	if err := os.WriteFile(path, []byte("original-long-line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	follower := serviceLogFollower{initialLines: 0}
	var output []string
	emit := func(line string) { output = append(output, line) }
	follower.forward(path, emit)

	if err := os.WriteFile(path, []byte("truncated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	follower.forward(path, emit)
	if countContaining(output, "truncated") != 2 || output[len(output)-1] != "truncated" {
		// One status line and one log line contain "truncated".
		t.Fatalf("truncate output = %v", output)
	}

	rotated := filepath.Join(target, "server.log.1")
	if err := os.Rename(path, rotated); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("recreated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	follower.forward(path, emit)
	if countContaining(output, "rotated or recreated") != 1 || output[len(output)-1] != "recreated" {
		t.Fatalf("recreate output = %v", output)
	}
	follower.forward(path, emit)
	if countContaining(output, "rotated or recreated") != 1 {
		t.Fatalf("recreate status repeated: %v", output)
	}
}

func TestServiceLogFollowerFlushesPartialLinesAndBoundsEachRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	follower := serviceLogFollower{initialLines: 0, partialFlushAfter: time.Millisecond}
	var output []string
	emit := func(line string) { output = append(output, line) }
	follower.forward(path, emit)
	appendLog(t, path, "partial")
	follower.forward(path, emit)
	if len(output) != 0 || follower.pendingSince.IsZero() {
		t.Fatalf("partial output = %v, pendingSince = %v", output, follower.pendingSince)
	}
	follower.flushStalePartial(follower.pendingSince.Add(time.Millisecond), emit)
	if len(output) != 1 || output[0] != "partial" {
		t.Fatalf("partial output = %v, want timely flush", output)
	}

	payload := strings.Repeat("x", serviceLogReadLimit+serviceLogPartialLimit)
	appendLog(t, path, payload)
	before := follower.offset
	follower.forward(path, emit)
	if consumed := follower.offset - before; consumed != serviceLogReadLimit {
		t.Fatalf("single poll consumed %d bytes, want bounded %d", consumed, serviceLogReadLimit)
	}
	if len(follower.pending) >= serviceLogPartialLimit {
		t.Fatalf("pending partial grew to %d bytes", len(follower.pending))
	}
}

func TestParsePipelineConfigValidatesServiceWatchAndInfersLongRunning(t *testing.T) {
	valid := `import { definePipeline, stage, watchService } from "@buildworld/pipeline"
export default definePipeline({
  timeoutSec: 17,
  stages: [stage("Observe", watchService("Logs", {
    pidFile: "server.pid",
    logFile: "server.log",
    heartbeatSeconds: 30,
    pollSeconds: 1,
    initialLines: 0,
  }))],
})`
	config, err := ParsePipelineConfig(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !config.AllowLongRunning || config.TimeoutSec != 17 {
		t.Fatalf("allowLongRunning=%v timeout=%d, want inferred long-running with explicit timeout preserved", config.AllowLongRunning, config.TimeoutSec)
	}

	tests := []struct {
		name   string
		config string
		want   string
	}{
		{"missing pid", `{"log_file":"server.log"}`, "pid_file is required"},
		{"missing log", `{"pid_file":"server.pid"}`, "log_file is required"},
		{"unknown", `{"pid_file":"server.pid","log_file":"server.log","retry":"yes"}`, "not supported"},
		{"heartbeat too low", `{"pid_file":"server.pid","log_file":"server.log","heartbeat_seconds":"4"}`, "heartbeat_seconds"},
		{"poll invalid", `{"pid_file":"server.pid","log_file":"server.log","poll_seconds":"0"}`, "poll_seconds"},
		{"initial invalid", `{"pid_file":"server.pid","log_file":"server.log","initial_lines":"-1"}`, "initial_lines"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := fmt.Sprintf(`import { definePipeline, stage, step } from "@buildworld/pipeline"
export default definePipeline({
  stages: [stage("Observe", step("Logs", "service_watch", "", { config: %s }))],
})`, test.config)
			_, err := ParsePipelineConfig(source)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParsePipelineConfig() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestServiceWatchRequiresTerminalPlacement(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "terminal step",
			source: `import { definePipeline, shell, stage, watchService } from "@buildworld/pipeline"
export default definePipeline({ stages: [stage("Observe", [
  watchService("Logs", { pidFile: "server.pid", logFile: "server.log" }),
  shell("After", "echo unreachable"),
])] })`,
			want: "terminal step",
		},
		{
			name: "terminal stage",
			source: `import { definePipeline, shell, stage, watchService } from "@buildworld/pipeline"
export default definePipeline({ stages: [
  stage("Observe", watchService("Logs", { pidFile: "server.pid", logFile: "server.log" })),
  stage("After", shell("After", "echo unreachable")),
] })`,
			want: "terminal stage",
		},
		{
			name: "post",
			source: `import { definePipeline, shell, stage, step } from "@buildworld/pipeline"
export default definePipeline({
  stages: [stage("Build", shell("Build", "echo ok"))],
  post: { always: [step("Logs", "service_watch", "", {
    config: { pid_file: "server.pid", log_file: "server.log" },
  })] },
})`,
			want: "not allowed in post steps",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParsePipelineConfig(test.source)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParsePipelineConfig() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestConditionalServiceWatchKeepsDefaultTimeoutEnabled(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "step condition", source: `import { definePipeline, stage, step } from "@buildworld/pipeline"
export default definePipeline({ stages: [stage("Observe", step("Logs", "service_watch", "", {
  if: "false", config: { pid_file: "server.pid", log_file: "server.log" },
}))] })`},
		{name: "stage condition", source: `import { definePipeline, stage, watchService } from "@buildworld/pipeline"
export default definePipeline({ stages: [stage("Observe", watchService("Logs", {
  pidFile: "server.pid", logFile: "server.log",
}), { if: "false" })] })`},
		{name: "branch filter", source: `import { definePipeline, stage, watchService } from "@buildworld/pipeline"
export default definePipeline({ stages: [stage("Observe", watchService("Logs", {
  pidFile: "server.pid", logFile: "server.log",
}), { branches: ["main"] })] })`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := ParsePipelineConfig(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if config.AllowLongRunning {
				t.Fatal("conditional service_watch disabled the finite default timeout")
			}
		})
	}
}

func appendLog(t *testing.T, path, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(text); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func countContaining(lines []string, fragment string) int {
	count := 0
	for _, line := range lines {
		if strings.Contains(line, fragment) {
			count++
		}
	}
	return count
}

func waitForWatchOutput(t *testing.T, output <-chan string, expected string) {
	t.Helper()
	waitForWatchOutputWithin(t, output, expected, 3*time.Second)
}

func waitForWatchOutputWithin(t *testing.T, output <-chan string, expected string, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line := <-output:
			if strings.Contains(line, expected) {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for service log %q", expected)
		}
	}
}

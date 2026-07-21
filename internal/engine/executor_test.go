package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExecutorRun(t *testing.T) {
	executor := NewExecutor()

	ctx := context.Background()
	output, err := executor.Run(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if output != "hello\n" {
		t.Errorf("output = %q, want %q", output, "hello\n")
	}
}

func TestExecutorRunError(t *testing.T) {
	executor := NewExecutor()

	ctx := context.Background()
	_, err := executor.Run(ctx, "false")
	if err == nil {
		t.Error("Run() should fail on 'false' command")
	}
}

func TestRunWithOutputCallback(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	var outputs []string
	err := executor.RunWithOutput(ctx, "echo", func(s string) {
		outputs = append(outputs, s)
	}, "hello")
	if err != nil {
		t.Fatalf("RunWithOutput() error = %v", err)
	}

	if len(outputs) == 0 {
		t.Fatal("RunWithOutput() callback was never called")
	}

	combined := strings.Join(outputs, "")
	if !strings.Contains(combined, "hello") {
		t.Errorf("callback output = %q, want it to contain %q", combined, "hello")
	}
}

func TestRunWithOutputContextCancellation(t *testing.T) {
	executor := NewExecutor()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- executor.RunWithOutput(ctx, slowCmd(), func(s string) {}, slowArgs()...)
	}()

	cancel()

	err := <-errCh
	if err == nil {
		t.Error("RunWithOutput() should fail when context is cancelled")
	}
}

func TestRunWithOutputTimeout(t *testing.T) {
	executor := NewExecutor()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := executor.RunWithOutput(ctx, slowCmd(), func(s string) {}, slowArgs()...)
	if err == nil {
		t.Error("RunWithOutput() should fail when context times out")
	}
}

func TestRunShellCancellationTerminatesChildProcess(t *testing.T) {
	executor := NewExecutor()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("BUILDWORLD_PROCESS_TREE_HELPER", "1")
	t.Setenv("BUILDWORLD_PROCESS_TREE_PID_FILE", pidFile)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	output := make(chan string, 32)
	command := processTreeShellCommand(t)
	go func() {
		result <- executor.RunShell(ctx, command, "", nil, func(line string) {
			output <- line
		})
	}()

	pid := waitForProcessTreePID(t, pidFile, result, output)
	if !serviceProcessRunning(pid) {
		t.Fatalf("child process %d was not running before cancellation", pid)
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("RunShell() should fail when its context is canceled")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunShell() did not return after cancellation")
	}
	waitForProcessTreeExit(t, pid)
}

func TestRunShellCancellationTerminatesChildAfterRootShellExits(t *testing.T) {
	executor := NewExecutor()
	pidFile := filepath.Join(t.TempDir(), "detached-child.pid")
	t.Setenv("BUILDWORLD_PROCESS_TREE_HELPER", "1")
	t.Setenv("BUILDWORLD_PROCESS_TREE_PID_FILE", pidFile)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	output := make(chan string, 32)
	command := detachedProcessTreeShellCommand(t)
	go func() {
		result <- executor.RunShell(ctx, command, "", nil, func(line string) {
			output <- line
		})
	}()

	pid := waitForProcessTreePID(t, pidFile, result, output)
	if !serviceProcessRunning(pid) {
		t.Fatalf("detached child process %d was not running before cancellation", pid)
	}
	// The shell command exits immediately; RunShell remains blocked only because
	// its detached child still owns the inherited output handles.
	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-result:
		t.Fatalf("RunShell returned before cancellation while child held its pipes: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("RunShell() should fail when its context is canceled")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunShell() did not return after detached-child cancellation")
	}
	waitForProcessTreeExit(t, pid)
}

func TestSuccessfulCommandPreservesIntentionalBackgroundChild(t *testing.T) {
	executor := NewExecutor()
	pidFile := filepath.Join(t.TempDir(), "successful-background.pid")
	t.Setenv("BUILDWORLD_PROCESS_TREE_HELPER", "1")
	t.Setenv("BUILDWORLD_PROCESS_TREE_PID_FILE", pidFile)
	t.Setenv("BUILDWORLD_PROCESS_TREE_SPAWNER", "1")
	t.Setenv("BUILDWORLD_PROCESS_TREE_SPAWNER_INHERIT_OUTPUT", "1")

	result := make(chan error, 1)
	executable, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, runErr := executor.Run(context.Background(), executable, "-test.run=^TestExecutorProcessTreeSpawner$")
		result <- runErr
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("successful background command failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("successful background command did not return")
	}

	pid := readProcessTreePID(t, pidFile)
	if !serviceProcessRunning(pid) {
		t.Fatalf("background child process %d did not survive successful command completion", pid)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Kill(); err != nil {
		t.Fatalf("clean background test child %d: %v", pid, err)
	}
	waitForProcessTreeExit(t, pid)
}

func TestFailedCommandTerminatesBackgroundChild(t *testing.T) {
	executor := NewExecutor()
	pidFile := filepath.Join(t.TempDir(), "failed-background.pid")
	t.Setenv("BUILDWORLD_PROCESS_TREE_HELPER", "1")
	t.Setenv("BUILDWORLD_PROCESS_TREE_PID_FILE", pidFile)
	t.Setenv("BUILDWORLD_PROCESS_TREE_SPAWNER", "1")
	t.Setenv("BUILDWORLD_PROCESS_TREE_SPAWNER_FAIL", "1")
	t.Setenv("BUILDWORLD_PROCESS_TREE_SPAWNER_INHERIT_OUTPUT", "1")

	executable, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Run(context.Background(), executable, "-test.run=^TestExecutorProcessTreeSpawner$"); err == nil {
		t.Fatal("failing spawner command unexpectedly succeeded")
	}
	pid := readProcessTreePID(t, pidFile)
	waitForProcessTreeExit(t, pid)
}

func TestExecutorProcessTreeHelper(t *testing.T) {
	if os.Getenv("BUILDWORLD_PROCESS_TREE_HELPER") != "1" {
		t.Skip("helper process")
	}
	pidFile := os.Getenv("BUILDWORLD_PROCESS_TREE_PID_FILE")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestExecutorProcessTreeSpawner(t *testing.T) {
	if os.Getenv("BUILDWORLD_PROCESS_TREE_SPAWNER") != "1" {
		t.Skip("helper process")
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	childFiles := []*os.File{devNull, devNull, devNull}
	if os.Getenv("BUILDWORLD_PROCESS_TREE_SPAWNER_INHERIT_OUTPUT") == "1" {
		// Keep the os/exec copy pipes open after this root exits. Cmd.Wait must
		// still return in bounded time, preserve this child on root success, and
		// terminate it on root failure.
		childFiles = []*os.File{devNull, os.Stdout, os.Stderr}
	}
	process, err := os.StartProcess(os.Args[0], []string{
		os.Args[0],
		"-test.run=^TestExecutorProcessTreeHelper$",
	}, &os.ProcAttr{
		Env:   os.Environ(),
		Files: childFiles,
	})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("BUILDWORLD_PROCESS_TREE_SPAWNER_FAIL") == "1" {
		pidFile := os.Getenv("BUILDWORLD_PROCESS_TREE_PID_FILE")
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(process.Pid)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := process.Release(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("BUILDWORLD_PROCESS_TREE_SPAWNER_FAIL") == "1" {
		os.Exit(7)
	}
}

func TestRunWithOutputEmptyCommand(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	err := executor.RunWithOutput(ctx, "", func(s string) {})
	if err == nil {
		t.Error("RunWithOutput() should fail with empty command")
	}
}

func processTreeShellCommand(t *testing.T) string {
	t.Helper()
	executable, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if strings.ContainsAny(executable, " \t\"") {
			t.Skip("Windows process-tree helper path requires quoting that cmd.exe rewrites")
		}
		return fmt.Sprintf(`%s -test.run=TestExecutorProcessTreeHelper`, executable)
	}
	quoted := "'" + strings.ReplaceAll(executable, "'", `'"'"'`) + "'"
	return quoted + ` -test.run='^TestExecutorProcessTreeHelper$' & child=$!; wait "$child"`
}

func detachedProcessTreeShellCommand(t *testing.T) string {
	t.Helper()
	executable, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if strings.ContainsAny(executable, " \t\"&|<>^") {
			t.Skip("Windows process-tree helper path cannot be represented safely by cmd.exe")
		}
		return fmt.Sprintf(`start /b %s -test.run=^TestExecutorProcessTreeHelper$`, executable)
	}
	quoted := "'" + strings.ReplaceAll(executable, "'", `'"'"'`) + "'"
	return quoted + ` -test.run='^TestExecutorProcessTreeHelper$' &`
}

func waitForProcessTreePID(t *testing.T, path string, result <-chan error, output <-chan string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var commandOutput strings.Builder
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				return pid
			}
		}
		select {
		case line := <-output:
			commandOutput.WriteString(line)
		case err := <-result:
			t.Fatalf("shell exited before writing child PID: %v; output: %s", err, commandOutput.String())
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatalf("timed out waiting for child PID file %s; output: %s", path, commandOutput.String())
	return 0
}

func readProcessTreePID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for child PID file %s", path)
	return 0
}

func waitForProcessTreeExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !serviceProcessRunning(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child process %d survived build cancellation", pid)
}

func TestRunWithOutputEnvVars(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	t.Setenv("BUILDWORLD_TEST_VAR", "hello123")

	name, args := echoEnvCmd("BUILDWORLD_TEST_VAR")
	var output string
	err := executor.RunWithOutput(ctx, name, func(s string) {
		output += s
	}, args...)
	if err != nil {
		t.Fatalf("RunWithOutput() error = %v", err)
	}

	if !strings.Contains(output, "hello123") {
		t.Errorf("output = %q, want it to contain env var value", output)
	}
}

func slowCmd() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "sleep"
}

func slowArgs() []string {
	if runtime.GOOS == "windows" {
		return []string{"/c", "ping", "-n", "6", "127.0.0.1"}
	}
	return []string{"5"}
}

func echoEnvCmd(key string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo", "%" + key + "%"}
	}
	return "sh", []string{"-c", "echo $" + key}
}

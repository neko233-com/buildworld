package engine

import (
	"context"
	"runtime"
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

func TestRunWithOutputEmptyCommand(t *testing.T) {
	executor := NewExecutor()
	ctx := context.Background()

	err := executor.RunWithOutput(ctx, "", func(s string) {})
	if err == nil {
		t.Error("RunWithOutput() should fail with empty command")
	}
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

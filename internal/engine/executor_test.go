package engine

import (
	"context"
	"testing"
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

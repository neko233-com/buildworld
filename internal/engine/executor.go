package engine

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type Executor struct {
	workspace string
}

func NewExecutor() *Executor {
	return &Executor{}
}

func (e *Executor) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

func (e *Executor) RunWithOutput(ctx context.Context, name string, onOutput func(string), args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)

	cmd.Stdout = &lineWriter{callback: onOutput}
	cmd.Stderr = &lineWriter{callback: onOutput}

	return cmd.Run()
}

type lineWriter struct {
	callback func(string)
}

func (w *lineWriter) Write(p []byte) (n int, err error) {
	w.callback(string(p))
	return len(p), nil
}

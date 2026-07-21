package processtree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

const inheritedPipeWaitDelay = time.Second

// Cmd wraps exec.Cmd so BuildWorld owns the complete command lifecycle. In
// particular, context cancellation can terminate a platform process group/job
// without relying on a reusable PID, while a normally completed command may
// deliberately leave a detached background service running.
type Cmd struct {
	*exec.Cmd

	ctx      context.Context
	platform platformState

	mu             sync.Mutex
	startAttempted bool
	started        bool
	waited         bool
	processDone    chan struct{}
	contextDone    chan commandContextResult
}

type commandContextResult struct {
	err            error
	terminationErr error
	terminated     bool
}

// CommandContext creates a command whose cancellation terminates the complete
// process tree rooted at the command. WaitDelay bounds inherited stdout/stderr
// pipes after the root exits, while Wait uses ProcessState to preserve a
// successful command's intentionally detached descendants.
func CommandContext(ctx context.Context, name string, args ...string) *Cmd {
	if ctx == nil {
		panic("nil Context")
	}
	return &Cmd{
		Cmd: func() *exec.Cmd {
			cmd := exec.Command(name, args...)
			cmd.WaitDelay = inheritedPipeWaitDelay
			return cmd
		}(),
		ctx: ctx,
	}
}

// Start starts the command inside its platform process-tree container.
func (c *Cmd) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.startAttempted {
		return errors.New("exec: already started")
	}
	c.startAttempted = true
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if err := startProcess(c.Cmd, &c.platform); err != nil {
		return err
	}
	c.started = true
	c.processDone = make(chan struct{})
	c.contextDone = make(chan commandContextResult, 1)
	go c.watchContext()
	return nil
}

// Wait waits for the command and applies failure cleanup to descendants. It
// must be called once after a successful Start.
func (c *Cmd) Wait() error {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return errors.New("exec: not started")
	}
	if c.waited {
		c.mu.Unlock()
		return errors.New("exec: Wait was already called")
	}
	c.waited = true
	processDone := c.processDone
	contextDone := c.contextDone
	c.mu.Unlock()

	processErr := c.Cmd.Wait()
	// If a successful root intentionally leaves a descendant holding inherited
	// output handles, os/exec closes those handles after WaitDelay and reports
	// ErrWaitDelay. The root's ProcessState remains the authoritative command
	// result: preserve the descendant and treat that bounded pipe cleanup as a
	// successful launch. A non-zero root still returns ExitError and takes the
	// failure cleanup path below.
	if errors.Is(processErr, exec.ErrWaitDelay) && c.Cmd.ProcessState != nil && c.Cmd.ProcessState.Success() {
		processErr = nil
	}
	close(processDone)
	ctxResult := <-contextDone

	var cleanupErr error
	if processErr != nil && !ctxResult.terminated {
		cleanupErr = normalizeTerminationError(terminateProcessTree(c.Cmd, &c.platform))
	}

	resultErr := processErr
	if resultErr == nil && ctxResult.err != nil {
		resultErr = ctxResult.err
	}
	if ctxResult.terminationErr != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("terminate process tree: %w", ctxResult.terminationErr))
	}
	if cleanupErr != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("clean failed process tree: %w", cleanupErr))
	}

	preserveDescendants := resultErr == nil
	if releaseErr := releaseProcessTree(&c.platform, preserveDescendants); releaseErr != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("release process tree: %w", releaseErr))
	}
	return resultErr
}

func (c *Cmd) watchContext() {
	select {
	case <-c.processDone:
		c.contextDone <- commandContextResult{}
	case <-c.ctx.Done():
		terminationErr := normalizeTerminationError(terminateProcessTree(c.Cmd, &c.platform))
		c.contextDone <- commandContextResult{
			err:            c.ctx.Err(),
			terminationErr: terminationErr,
			terminated:     true,
		}
	}
}

func normalizeTerminationError(err error) error {
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

// Run starts and waits for the command.
func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// Output runs the command and returns stdout while preserving ExitError.Stderr
// behavior used by callers of os/exec.Cmd.Output.
func (c *Cmd) Output() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	var stdout bytes.Buffer
	c.Stdout = &stdout

	captureStderr := c.Stderr == nil
	var stderr bytes.Buffer
	if captureStderr {
		c.Stderr = &stderr
	}
	err := c.Run()
	if captureStderr {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitErr.Stderr = append([]byte(nil), stderr.Bytes()...)
		}
	}
	return append([]byte(nil), stdout.Bytes()...), err
}

// CombinedOutput runs the command and returns its combined stdout and stderr.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if c.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var output lockedBuffer
	c.Stdout = &output
	c.Stderr = &output
	err := c.Run()
	return output.Bytes(), err
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

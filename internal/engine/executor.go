package engine

import (
	"bytes"
	"context"
	"debug/pe"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/neko233-com/buildworld/internal/processtree"
)

type Executor struct {
	workspace string
}

func NewExecutor() *Executor {
	return &Executor{}
}

func (e *Executor) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := processtree.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

func (e *Executor) RunWithOutput(ctx context.Context, name string, onOutput func(string), args ...string) error {
	if name == "" {
		return fmt.Errorf("empty command")
	}
	cmd := processtree.CommandContext(ctx, name, args...)

	return runWithLineWriters(cmd, onOutput)
}

func (e *Executor) RunShell(ctx context.Context, command, dir string, env []string, onOutput func(string)) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	var name string
	var args []string
	if runtime.GOOS == "windows" {
		name = "cmd"
		args = []string{"/c", command}
	} else {
		name = "sh"
		args = []string{"-c", command}
	}
	cmd := processtree.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	return runWithLineWriters(cmd, onOutput)
}

func (e *Executor) RunMultiShell(ctx context.Context, shell, command, dir string, env []string, onOutput func(string)) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	shell = strings.ToLower(strings.TrimSpace(shell))
	if shell == "" {
		return e.RunShell(ctx, command, dir, env, onOutput)
	}

	switch shell {
	case "powershell", "ps1", "pwsh":
		return e.runPowerShell(ctx, shell, command, dir, env, onOutput)
	case "bash", "sh":
		return e.runBash(ctx, shell, command, dir, env, onOutput)
	case "cmd":
		return e.runCmd(ctx, command, dir, env, onOutput)
	case "python", "python3":
		return e.runPython(ctx, shell, command, dir, env, onOutput)
	default:
		return e.RunShell(ctx, command, dir, env, onOutput)
	}
}

func (e *Executor) runPowerShell(ctx context.Context, shell, command, dir string, env []string, onOutput func(string)) error {
	isMultiLine := strings.Contains(command, "\n")
	var cmd *processtree.Cmd
	if runtime.GOOS == "windows" {
		name := "powershell"
		if shell == "pwsh" {
			var err error
			name, err = preferredWindowsPwsh()
			if err != nil {
				return fmt.Errorf("PowerShell Core (pwsh) is unavailable: %w", err)
			}
		}
		if isMultiLine {
			scriptPath, err := e.writeTempScript(dir, command, ".ps1")
			if err != nil {
				return err
			}
			defer os.Remove(scriptPath)
			cmd = processtree.CommandContext(ctx, name, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
		} else {
			cmd = processtree.CommandContext(ctx, name, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command)
		}
	} else {
		if isMultiLine {
			scriptPath, err := e.writeTempScript(dir, command, ".ps1")
			if err != nil {
				return err
			}
			defer os.Remove(scriptPath)
			cmd = processtree.CommandContext(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-File", scriptPath)
		} else {
			cmd = processtree.CommandContext(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-Command", command)
		}
	}
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	return runWithLineWriters(cmd, onOutput)
}

// preferredWindowsPwsh prefers a matching native PE executable in PATH while
// retaining the existing PATH choice when only a different architecture exists.
func preferredWindowsPwsh() (string, error) {
	fallback, err := exec.LookPath("pwsh")
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(filepath.Base(fallback), "pwsh.exe") {
		return fallback, nil
	}
	architecture := runtime.GOARCH
	switch strings.ToUpper(os.Getenv("PROCESSOR_ARCHITEW6432")) {
	case "AMD64":
		architecture = "amd64"
	case "ARM64":
		architecture = "arm64"
	}
	var machine uint16
	switch architecture {
	case "amd64":
		machine = pe.IMAGE_FILE_MACHINE_AMD64
	case "arm64":
		machine = pe.IMAGE_FILE_MACHINE_ARM64
	default:
		return fallback, nil
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}
		candidate := filepath.Join(dir, "pwsh.exe")
		binary, openErr := pe.Open(candidate)
		if openErr != nil {
			continue
		}
		matches := binary.Machine == machine
		_ = binary.Close()
		if matches {
			return candidate, nil
		}
	}
	return fallback, nil
}

func (e *Executor) runBash(ctx context.Context, shell, command, dir string, env []string, onOutput func(string)) error {
	if runtime.GOOS == "windows" {
		bashPath, err := exec.LookPath("bash")
		if err == nil {
			shPath, _ := exec.LookPath("sh")
			bashPath = preferredWindowsBash(bashPath, shPath, os.Getenv("SystemRoot"))
			cmd := processtree.CommandContext(ctx, bashPath, "-c", command)
			if dir != "" {
				cmd.Dir = dir
			}
			if len(env) > 0 {
				cmd.Env = append(cmd.Environ(), env...)
			}
			return runWithLineWriters(cmd, onOutput)
		}
		scriptPath, err := e.writeTempScript(dir, command, ".sh")
		if err != nil {
			return err
		}
		defer os.Remove(scriptPath)
		cmd := processtree.CommandContext(ctx, "sh", scriptPath)
		if dir != "" {
			cmd.Dir = dir
		}
		if len(env) > 0 {
			cmd.Env = append(cmd.Environ(), env...)
		}
		return runWithLineWriters(cmd, onOutput)
	}
	name := shell
	if name == "" {
		name = "sh"
	}
	cmd := processtree.CommandContext(ctx, name, "-c", command)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	return runWithLineWriters(cmd, onOutput)
}

// preferredWindowsBash avoids the Windows WSL launcher when a POSIX shell on
// PATH has its own Bash. Preserve explicitly selected non-system Bash paths.
func preferredWindowsBash(bashPath, shPath, windowsRoot string) string {
	if windowsRoot == "" || shPath == "" {
		return bashPath
	}
	systemBash := filepath.Join(windowsRoot, "System32", "bash.exe")
	if !strings.EqualFold(filepath.Clean(bashPath), filepath.Clean(systemBash)) {
		return bashPath
	}
	candidate := filepath.Join(filepath.Dir(shPath), "bash.exe")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}
	return bashPath
}

func (e *Executor) runCmd(ctx context.Context, command, dir string, env []string, onOutput func(string)) error {
	if runtime.GOOS == "windows" {
		cmd := processtree.CommandContext(ctx, "cmd", "/c", command)
		if dir != "" {
			cmd.Dir = dir
		}
		if len(env) > 0 {
			cmd.Env = append(cmd.Environ(), env...)
		}
		return runWithLineWriters(cmd, onOutput)
	}
	return e.RunShell(ctx, command, dir, env, onOutput)
}

func (e *Executor) runPython(ctx context.Context, shell, command, dir string, env []string, onOutput func(string)) error {
	scriptPath, err := e.writeTempScript(dir, command, ".py")
	if err != nil {
		return err
	}
	defer os.Remove(scriptPath)
	py := shell
	if py == "" {
		py = "python"
	}
	cmd := processtree.CommandContext(ctx, py, scriptPath)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	return runWithLineWriters(cmd, onOutput)
}

func (e *Executor) writeTempScript(dir, content, ext string) (string, error) {
	scriptRoot := filepath.Join(dir, ".buildworld", "scripts")
	if dir == "" {
		scriptRoot = filepath.Join(ResolveBuildTempRoot(""), "scripts")
	}
	if err := os.MkdirAll(scriptRoot, 0o755); err != nil {
		return "", fmt.Errorf("create isolated script directory: %w", err)
	}
	f, err := os.CreateTemp(scriptRoot, "bw_script_*"+ext)
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	f.Close()
	if runtime.GOOS != "windows" {
		os.Chmod(f.Name(), 0o755)
	}
	path, _ := filepath.Abs(f.Name())
	return path, nil
}

type lineWriter struct {
	mu       sync.Mutex
	callback func(string)
	pending  []byte
}

type synchronizedPipeWriter struct {
	mu     sync.Mutex
	writer *io.PipeWriter
}

func (w *synchronizedPipeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(p)
}

func (w *lineWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	w.pending = append(w.pending, p...)
	lines := make([]string, 0, bytes.Count(w.pending, []byte{'\n'}))
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		line := strings.TrimSuffix(string(w.pending[:index]), "\r")
		lines = append(lines, line)
		w.pending = w.pending[index+1:]
	}
	w.mu.Unlock()
	for _, line := range lines {
		w.callback(line)
	}
	return len(p), nil
}

func (w *lineWriter) Flush() {
	w.mu.Lock()
	line := strings.TrimSuffix(string(w.pending), "\r")
	w.pending = nil
	w.mu.Unlock()
	if line != "" {
		w.callback(line)
	}
}

func runWithLineWriters(cmd *processtree.Cmd, callback func(string)) error {
	output := &lineWriter{callback: callback}
	reader, writer := io.Pipe()
	combinedWriter := &synchronizedPipeWriter{writer: writer}
	cmd.Stdout = combinedWriter
	cmd.Stderr = combinedWriter

	readDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			count, readErr := reader.Read(buffer)
			if count > 0 {
				_, _ = output.Write(buffer[:count])
			}
			if readErr != nil {
				if readErr == io.EOF {
					readDone <- nil
				} else {
					readDone <- readErr
				}
				return
			}
		}
	}()

	err := cmd.Run()
	_ = writer.Close()
	readErr := <-readDone
	output.Flush()
	if err != nil {
		return err
	}
	return readErr
}

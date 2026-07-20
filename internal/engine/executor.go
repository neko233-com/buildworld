package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	if name == "" {
		return fmt.Errorf("empty command")
	}
	cmd := exec.CommandContext(ctx, name, args...)

	cmd.Stdout = &lineWriter{callback: onOutput}
	cmd.Stderr = &lineWriter{callback: onOutput}

	return cmd.Run()
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
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	cmd.Stdout = &lineWriter{callback: onOutput}
	cmd.Stderr = &lineWriter{callback: onOutput}
	return cmd.Run()
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
		return e.runPowerShell(ctx, command, dir, env, onOutput)
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

func (e *Executor) runPowerShell(ctx context.Context, command, dir string, env []string, onOutput func(string)) error {
	isMultiLine := strings.Contains(command, "\n")
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		if isMultiLine {
			scriptPath, err := e.writeTempScript(dir, command, ".ps1")
			if err != nil {
				return err
			}
			defer os.Remove(scriptPath)
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
		} else {
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command)
		}
	} else {
		if isMultiLine {
			scriptPath, err := e.writeTempScript(dir, command, ".ps1")
			if err != nil {
				return err
			}
			defer os.Remove(scriptPath)
			cmd = exec.CommandContext(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-File", scriptPath)
		} else {
			cmd = exec.CommandContext(ctx, "pwsh", "-NoProfile", "-NonInteractive", "-Command", command)
		}
	}
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	cmd.Stdout = &lineWriter{callback: onOutput}
	cmd.Stderr = &lineWriter{callback: onOutput}
	return cmd.Run()
}

func (e *Executor) runBash(ctx context.Context, shell, command, dir string, env []string, onOutput func(string)) error {
	if runtime.GOOS == "windows" {
		_, err := exec.LookPath("bash")
		if err == nil {
			cmd := exec.CommandContext(ctx, "bash", "-c", command)
			if dir != "" {
				cmd.Dir = dir
			}
			if len(env) > 0 {
				cmd.Env = append(cmd.Environ(), env...)
			}
			cmd.Stdout = &lineWriter{callback: onOutput}
			cmd.Stderr = &lineWriter{callback: onOutput}
			return cmd.Run()
		}
		scriptPath, err := e.writeTempScript(dir, command, ".sh")
		if err != nil {
			return err
		}
		defer os.Remove(scriptPath)
		cmd := exec.CommandContext(ctx, "sh", scriptPath)
		if dir != "" {
			cmd.Dir = dir
		}
		if len(env) > 0 {
			cmd.Env = append(cmd.Environ(), env...)
		}
		cmd.Stdout = &lineWriter{callback: onOutput}
		cmd.Stderr = &lineWriter{callback: onOutput}
		return cmd.Run()
	}
	name := shell
	if name == "" {
		name = "sh"
	}
	cmd := exec.CommandContext(ctx, name, "-c", command)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	cmd.Stdout = &lineWriter{callback: onOutput}
	cmd.Stderr = &lineWriter{callback: onOutput}
	return cmd.Run()
}

func (e *Executor) runCmd(ctx context.Context, command, dir string, env []string, onOutput func(string)) error {
	if runtime.GOOS == "windows" {
		cmd := exec.CommandContext(ctx, "cmd", "/c", command)
		if dir != "" {
			cmd.Dir = dir
		}
		if len(env) > 0 {
			cmd.Env = append(cmd.Environ(), env...)
		}
		cmd.Stdout = &lineWriter{callback: onOutput}
		cmd.Stderr = &lineWriter{callback: onOutput}
		return cmd.Run()
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
	cmd := exec.CommandContext(ctx, py, scriptPath)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}
	cmd.Stdout = &lineWriter{callback: onOutput}
	cmd.Stderr = &lineWriter{callback: onOutput}
	return cmd.Run()
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
	callback func(string)
}

func (w *lineWriter) Write(p []byte) (n int, err error) {
	w.callback(string(p))
	return len(p), nil
}

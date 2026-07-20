package cli

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/neko233-com/buildworld/internal/config"
)

func stateDir() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(directory, "buildworld"), nil
}

func resolvedConfigPath(requested string) (string, error) {
	if requested != "" {
		return filepath.Abs(requested)
	}
	directory, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "config.yaml"), nil
}

func ensureConfig(requested string) (string, *config.Config, error) {
	path, err := resolvedConfigPath(requested)
	if err != nil {
		return "", nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		directory := filepath.Dir(path)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return "", nil, fmt.Errorf("create BuildWorld state directory: %w", err)
		}
		toYAML := func(value string) string { return strings.ReplaceAll(filepath.ToSlash(value), "\\", "/") }
		contents := fmt.Sprintf("server:\n  host: 127.0.0.1\n  port: 8700\ndatabase:\n  path: %q\nplugins:\n  path: %q\nstorage:\n  workspace: %q\n  build_temp: %q\n  artifacts: %q\n  logs: %q\nworkers:\n  local:\n    enabled: true\n    max_concurrent_builds: 4\n    workspace: local\n    pool: default\n    labels: [go, nodejs, typescript]\n", toYAML(filepath.Join(directory, "buildworld.db")), toYAML(filepath.Join(directory, "plugins")), toYAML(filepath.Join(directory, "workspace")), toYAML(filepath.Join(directory, "build_temp")), toYAML(filepath.Join(directory, "artifacts")), toYAML(filepath.Join(directory, "logs")))
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			return "", nil, fmt.Errorf("write default BuildWorld config: %w", err)
		}
	} else if err != nil {
		return "", nil, fmt.Errorf("inspect config: %w", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return "", nil, err
	}
	return path, cfg, nil
}

func pidPath() string {
	directory, err := stateDir()
	if err != nil {
		return ""
	}
	return filepath.Join(directory, "server.pid")
}

func serverLogPath() string {
	directory, err := stateDir()
	if err != nil {
		return "buildworld-server.log"
	}
	return filepath.Join(directory, "server.log")
}

func openServerLog() (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(serverLogPath()), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(serverLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

func writePID(pid int) error {
	path := pidPath()
	if path == "" {
		return fmt.Errorf("resolve BuildWorld PID path")
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidPath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func healthy(port int) bool {
	client := http.Client{Timeout: 750 * time.Millisecond}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", port))
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func serverExecutable() (string, error) {
	current, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve BuildWorld executable: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(current); resolveErr == nil {
		current = resolved
	}
	name := "buildworld-server"
	if filepath.Ext(current) == ".exe" {
		name += ".exe"
	}
	candidate := filepath.Join(filepath.Dir(current), name)
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate, nil
	}
	if found, err := exec.LookPath(name); err == nil {
		return found, nil
	}
	return "", fmt.Errorf("cannot find %s beside %s; reinstall the complete BuildWorld release bundle", name, current)
}

func stopManagedServer() (bool, error) {
	// Service managers restart a child that is killed directly. Stop the
	// registered service first so pause/stop actually leaves BuildWorld down.
	stopped, err := stopAutostartService()
	if err != nil {
		return false, err
	}
	if stopped {
		_ = os.Remove(pidPath())
		return true, nil
	}
	pid, err := readPID()
	if err == nil && pid > 0 {
		process, findErr := os.FindProcess(pid)
		if findErr == nil {
			if err := stopProcess(process); err == nil {
				_ = os.Remove(pidPath())
				return true, nil
			}
		}
	}
	return false, nil
}

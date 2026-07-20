package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WatchService is the native counterpart to Jenkins' `tail -f` monitor. It
// owns no service process: cancelling the build stops observation only, while
// an exited PID makes the build fail after emitting the final log lines.
func WatchService(ctx context.Context, workspace string, config map[string]string, onOutput func(string)) error {
	targetDir := strings.TrimSpace(firstWatchConfig(config, "target_dir", "targetDir"))
	if targetDir == "" {
		targetDir = workspace
	}
	if !filepath.IsAbs(targetDir) {
		targetDir = filepath.Join(workspace, targetDir)
	}
	targetDir = filepath.Clean(targetDir)

	pidPath, err := watchPath(targetDir, firstWatchConfig(config, "pid_file", "pidFile"))
	if err != nil {
		return fmt.Errorf("service watch pid_file: %w", err)
	}
	logPath, err := watchPath(targetDir, firstWatchConfig(config, "log_file", "logFile"))
	if err != nil {
		return fmt.Errorf("service watch log_file: %w", err)
	}
	pid, err := readServicePID(pidPath)
	if err != nil {
		return err
	}
	if !serviceProcessRunning(pid) {
		return fmt.Errorf("service process is not running (PID: %d)", pid)
	}

	heartbeat := watchDuration(config, "heartbeat_seconds", "heartbeatSeconds", 30*time.Second, 5*time.Second)
	poll := watchDuration(config, "poll_seconds", "pollSeconds", 500*time.Millisecond, 200*time.Millisecond)
	port := strings.TrimSpace(firstWatchConfig(config, "port"))
	onOutput(fmt.Sprintf("BuildWorld is continuously following service logs (PID: %d)%s", pid, watchPortLabel(port)))

	follower := serviceLogFollower{}
	if initial := watchInt(config, "initial_lines", "initialLines", 10); initial > 0 {
		for _, line := range serviceLogTail(logPath, initial) {
			onOutput(line)
		}
		follower.skipExisting(logPath)
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	heartbeats := time.NewTicker(heartbeat)
	defer heartbeats.Stop()

	for {
		follower.forward(logPath, onOutput)
		if !serviceProcessRunning(pid) {
			onOutput(fmt.Sprintf("❌ Service process exited (PID: %d)", pid))
			onOutput("Last 50 service log lines:")
			for _, line := range serviceLogTail(logPath, 50) {
				onOutput(line)
			}
			return fmt.Errorf("service process exited (PID: %d)", pid)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeats.C:
			onOutput(fmt.Sprintf("[heartbeat %s] service is running (PID: %d%s)", time.Now().Format("2006-01-02 15:04:05"), pid, watchPortLabel(port)))
		case <-ticker.C:
		}
	}
}

func firstWatchConfig(config map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(config[name]); value != "" {
			return value
		}
	}
	return ""
}

func watchPath(targetDir, configured string) (string, error) {
	if strings.TrimSpace(configured) == "" {
		return "", fmt.Errorf("is required")
	}
	path := configured
	if !filepath.IsAbs(path) {
		path = filepath.Join(targetDir, path)
	}
	path = filepath.Clean(path)
	rel, err := filepath.Rel(targetDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("must remain inside target_dir")
	}
	return path, nil
}

func readServicePID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("service PID file %q: %w", path, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("service PID file %q is invalid", path)
	}
	return pid, nil
}

func watchDuration(config map[string]string, snake, camel string, fallback, minimum time.Duration) time.Duration {
	seconds, err := strconv.Atoi(firstWatchConfig(config, snake, camel))
	if err != nil || seconds <= 0 {
		return fallback
	}
	value := time.Duration(seconds) * time.Second
	if value < minimum {
		return minimum
	}
	return value
}

func watchInt(config map[string]string, snake, camel string, fallback int) int {
	value, err := strconv.Atoi(firstWatchConfig(config, snake, camel))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func watchPortLabel(port string) string {
	if port == "" {
		return ""
	}
	return fmt.Sprintf(", port: %s", port)
}

type serviceLogFollower struct {
	offset  int64
	pending string
}

func (f *serviceLogFollower) skipExisting(path string) {
	if info, err := os.Stat(path); err == nil {
		f.offset = info.Size()
	}
}

func (f *serviceLogFollower) forward(path string, onOutput func(string)) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}
	if info.Size() < f.offset {
		f.offset, f.pending = 0, ""
	}
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	if _, err := file.Seek(f.offset, 0); err != nil {
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return
	}
	chunk := string(data)
	f.offset += int64(len(data))
	parts := strings.Split(f.pending+chunk, "\n")
	f.pending = parts[len(parts)-1]
	for _, line := range parts[:len(parts)-1] {
		onOutput(strings.TrimSuffix(line, "\r"))
	}
}

func serviceLogTail(path string, count int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	for index := range lines {
		lines[index] = strings.TrimSuffix(lines[index], "\r")
	}
	return lines
}

package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	serviceWatchDefaultHeartbeat = 15 * time.Minute
	serviceWatchMinimumHeartbeat = 5 * time.Second
	serviceWatchMaximumHeartbeat = 24 * time.Hour
	serviceWatchDefaultShutdown  = 65 * time.Second
	serviceWatchMaximumShutdown  = 10 * time.Minute
	serviceWatchDefaultPoll      = 500 * time.Millisecond
	serviceWatchMinimumPoll      = time.Second
	serviceWatchMaximumPoll      = time.Hour
	serviceWatchMaximumInitial   = 10_000

	serviceLogFollowInterval  = 100 * time.Millisecond
	serviceLogCatchUpInterval = time.Millisecond
	serviceLogReadBufferSize  = 32 * 1024
	serviceLogReadLimit       = 256 * 1024
	serviceLogPartialLimit    = 64 * 1024
	serviceLogTailReadLimit   = 1024 * 1024
	serviceLogPartialDelay    = 500 * time.Millisecond
)

// WatchService is the native counterpart to Jenkins' `tail -f` monitor. It
// owns no service process: cancelling the build stops observation only, while
// an exited PID makes the build fail after emitting the final log lines.
func WatchService(ctx context.Context, workspace string, config map[string]string, onOutput func(string)) error {
	if err := ValidateServiceWatchConfig(config); err != nil {
		return err
	}
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
		return reportServiceExit(pid, logPath, onOutput)
	}
	ownership, err := newServiceWatchOwnership(targetDir, config)
	if err != nil {
		return err
	}
	if ownership != nil {
		if err := ownership.claim(); err != nil {
			return err
		}
		defer ownership.release()
	}

	heartbeat := watchDuration(config, "heartbeat_seconds", "heartbeatSeconds", serviceWatchDefaultHeartbeat)
	poll := watchDuration(config, "poll_seconds", "pollSeconds", serviceWatchDefaultPoll)
	port := strings.TrimSpace(firstWatchConfig(config, "port"))
	onOutput(fmt.Sprintf("BuildWorld is continuously following service logs (PID: %d)%s", pid, watchPortLabel(port)))

	follower := serviceLogFollower{
		initialLines:      watchInt(config, "initial_lines", "initialLines", 10),
		partialFlushAfter: serviceLogPartialDelay,
	}
	// The initial tail and its follow offset come from one open-file snapshot.
	// Appends after that snapshot remain beyond the captured offset and are
	// forwarded by the next independent log-follow pass.
	backlogged := follower.forward(logPath, onOutput)
	logTimer := time.NewTimer(serviceLogNextInterval(backlogged))
	defer logTimer.Stop()
	processChecks := time.NewTicker(poll)
	defer processChecks.Stop()
	heartbeats := time.NewTicker(heartbeat)
	defer heartbeats.Stop()

	for {
		select {
		case <-ctx.Done():
			if watchBool(config, "stop_service_on_cancel", "stopServiceOnCancel") {
				onOutput(fmt.Sprintf("Build cancellation requested; gracefully stopping service PID: %d", pid))
				shutdown := watchDuration(config, "shutdown_timeout_seconds", "shutdownTimeoutSeconds", serviceWatchDefaultShutdown)
				if err := terminateServiceProcess(pid, shutdown); err != nil {
					onOutput(fmt.Sprintf("❌ Service PID %d did not stop cleanly: %v", pid, err))
					return err
				}
				follower.forward(logPath, onOutput)
				onOutput(fmt.Sprintf("Service PID %d stopped after build cancellation.", pid))
			}
			return ctx.Err()
		case <-heartbeats.C:
			onOutput(fmt.Sprintf("[heartbeat %s] service is running (PID: %d%s)", time.Now().Format("2006-01-02 15:04:05"), pid, watchPortLabel(port)))
		case <-processChecks.C:
			if !serviceProcessRunning(pid) {
				follower.forward(logPath, onOutput)
				if ownership != nil && ownership.handoverRequested() {
					onOutput(fmt.Sprintf("Service PID %d was replaced by deployment %s; log monitor handed over.", pid, ownership.handoverID()))
					return nil
				}
				return reportServiceExit(pid, logPath, onOutput)
			}
		case <-logTimer.C:
			backlogged = follower.forward(logPath, onOutput)
			logTimer.Reset(serviceLogNextInterval(backlogged))
		}
	}
}

// serviceWatchOwnership preserves Jenkins' monitor-handover contract. The
// retiring monitor succeeds only when a newer deployment explicitly requested
// it; an unexpected service exit remains a failed build.
type serviceWatchOwnership struct {
	ownerPath    string
	handoverPath string
	ownerID      string
}

func newServiceWatchOwnership(targetDir string, config map[string]string) (*serviceWatchOwnership, error) {
	ownerFile := firstWatchConfig(config, "owner_file", "ownerFile")
	handoverFile := firstWatchConfig(config, "handover_file", "handoverFile")
	ownerID := firstWatchConfig(config, "owner_id", "ownerId")
	if ownerFile == "" && handoverFile == "" && ownerID == "" {
		return nil, nil
	}
	if ownerFile == "" || handoverFile == "" || ownerID == "" {
		return nil, fmt.Errorf("service_watch owner_file, handover_file and owner_id must be configured together")
	}
	ownerPath, err := watchPath(targetDir, ownerFile)
	if err != nil {
		return nil, fmt.Errorf("service watch owner_file: %w", err)
	}
	handoverPath, err := watchPath(targetDir, handoverFile)
	if err != nil {
		return nil, fmt.Errorf("service watch handover_file: %w", err)
	}
	return &serviceWatchOwnership{ownerPath: ownerPath, handoverPath: handoverPath, ownerID: ownerID}, nil
}

func (o *serviceWatchOwnership) claim() error {
	if err := os.WriteFile(o.ownerPath, []byte(o.ownerID), 0o600); err != nil {
		return fmt.Errorf("claim service log monitor: %w", err)
	}
	return nil
}

func (o *serviceWatchOwnership) handoverID() string {
	contents, err := os.ReadFile(o.handoverPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(contents))
}

func (o *serviceWatchOwnership) handoverRequested() bool {
	requested := o.handoverID()
	return requested != "" && requested != o.ownerID
}

func (o *serviceWatchOwnership) release() {
	contents, err := os.ReadFile(o.ownerPath)
	if err == nil && strings.TrimSpace(string(contents)) == o.ownerID {
		_ = os.Remove(o.ownerPath)
	}
}

func serviceLogNextInterval(backlogged bool) time.Duration {
	if backlogged {
		return serviceLogCatchUpInterval
	}
	return serviceLogFollowInterval
}

// ValidateServiceWatchConfig validates the deliberately small native watcher
// surface before a build is queued. Runtime-only env.* and working-directory
// entries are accepted because the generic step runner adds them to Config.
func ValidateServiceWatchConfig(config map[string]string) error {
	allowed := map[string]struct{}{
		"target_dir": {}, "targetDir": {},
		"pid_file": {}, "pidFile": {},
		"log_file": {}, "logFile": {},
		"port":              {},
		"heartbeat_seconds": {}, "heartbeatSeconds": {},
		"poll_seconds": {}, "pollSeconds": {},
		"initial_lines": {}, "initialLines": {},
		"stop_service_on_cancel": {}, "stopServiceOnCancel": {},
		"shutdown_timeout_seconds": {}, "shutdownTimeoutSeconds": {},
		"owner_file": {}, "ownerFile": {},
		"handover_file": {}, "handoverFile": {},
		"owner_id": {}, "ownerId": {},
		"working-directory": {},
	}
	for key := range config {
		if _, ok := allowed[key]; ok || strings.HasPrefix(key, "env.") {
			continue
		}
		return fmt.Errorf("service_watch option %q is not supported", key)
	}
	for _, aliases := range [][2]string{
		{"target_dir", "targetDir"},
		{"pid_file", "pidFile"},
		{"log_file", "logFile"},
		{"heartbeat_seconds", "heartbeatSeconds"},
		{"poll_seconds", "pollSeconds"},
		{"initial_lines", "initialLines"},
		{"stop_service_on_cancel", "stopServiceOnCancel"},
		{"shutdown_timeout_seconds", "shutdownTimeoutSeconds"},
		{"owner_file", "ownerFile"},
		{"handover_file", "handoverFile"},
		{"owner_id", "ownerId"},
	} {
		if err := validateWatchAliases(config, aliases[0], aliases[1]); err != nil {
			return err
		}
	}
	if firstWatchConfig(config, "pid_file", "pidFile") == "" {
		return fmt.Errorf("service_watch pid_file is required")
	}
	if firstWatchConfig(config, "log_file", "logFile") == "" {
		return fmt.Errorf("service_watch log_file is required")
	}
	if err := validateWatchInteger(config, "heartbeat_seconds", "heartbeatSeconds",
		int(serviceWatchMinimumHeartbeat/time.Second), int(serviceWatchMaximumHeartbeat/time.Second)); err != nil {
		return err
	}
	if err := validateWatchInteger(config, "poll_seconds", "pollSeconds",
		int(serviceWatchMinimumPoll/time.Second), int(serviceWatchMaximumPoll/time.Second)); err != nil {
		return err
	}
	if err := validateWatchInteger(config, "initial_lines", "initialLines", 0, serviceWatchMaximumInitial); err != nil {
		return err
	}
	if err := validateWatchBool(config, "stop_service_on_cancel", "stopServiceOnCancel"); err != nil {
		return err
	}
	if err := validateWatchInteger(config, "shutdown_timeout_seconds", "shutdownTimeoutSeconds", 1, int(serviceWatchMaximumShutdown/time.Second)); err != nil {
		return err
	}
	return nil
}

func validateWatchAliases(config map[string]string, snake, camel string) error {
	snakeValue, snakeSet := config[snake]
	camelValue, camelSet := config[camel]
	snakeValue = strings.TrimSpace(snakeValue)
	camelValue = strings.TrimSpace(camelValue)
	if snakeSet && camelSet && snakeValue != "" && camelValue != "" && snakeValue != camelValue {
		return fmt.Errorf("service_watch options %q and %q conflict", snake, camel)
	}
	return nil
}

func validateWatchInteger(config map[string]string, snake, camel string, minimum, maximum int) error {
	value := firstWatchConfig(config, snake, camel)
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return fmt.Errorf("service_watch %s must be an integer from %d to %d", snake, minimum, maximum)
	}
	return nil
}

func reportServiceExit(pid int, logPath string, onOutput func(string)) error {
	onOutput(fmt.Sprintf("❌ Service process exited (PID: %d)", pid))
	onOutput("Last 50 service log lines:")
	for _, line := range serviceLogTail(logPath, 50) {
		onOutput(line)
	}
	return fmt.Errorf("service process exited (PID: %d)", pid)
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

func watchDuration(config map[string]string, snake, camel string, fallback time.Duration) time.Duration {
	seconds, err := strconv.Atoi(firstWatchConfig(config, snake, camel))
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func watchBool(config map[string]string, snake, camel string) bool {
	value, err := strconv.ParseBool(firstWatchConfig(config, snake, camel))
	return err == nil && value
}

func validateWatchBool(config map[string]string, snake, camel string) error {
	value := firstWatchConfig(config, snake, camel)
	if value == "" {
		return nil
	}
	if _, err := strconv.ParseBool(value); err != nil {
		return fmt.Errorf("service_watch %s must be true or false", snake)
	}
	return nil
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
	offset            int64
	pending           []byte
	pendingSince      time.Time
	fileInfo          os.FileInfo
	attached          bool
	unavailable       bool
	initialLines      int
	partialFlushAfter time.Duration
}

// forward reads at most serviceLogReadLimit bytes. Its return value reports
// whether more bytes are already available, allowing the caller to catch up
// immediately without letting a single read monopolize cancellation handling.
func (f *serviceLogFollower) forward(path string, onOutput func(string)) bool {
	file, err := os.Open(path)
	if err != nil {
		f.markUnavailable(path, onOutput)
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		f.markUnavailable(path, onOutput)
		return false
	}

	if !f.attached {
		if f.unavailable {
			onOutput(fmt.Sprintf("✅ Service log is available; following %s", path))
		}
		for _, line := range serviceLogTailFromFile(file, info.Size(), f.initialLines) {
			onOutput(line)
		}
		f.offset = info.Size()
		f.fileInfo = info
		f.attached = true
		f.unavailable = false
		return serviceLogHasUnread(file, f.offset)
	}

	wasUnavailable := f.unavailable
	if !os.SameFile(f.fileInfo, info) {
		f.reset(0)
		if wasUnavailable {
			onOutput(fmt.Sprintf("✅ Service log was recreated; following %s", path))
		} else {
			onOutput(fmt.Sprintf("↻ Service log was rotated or recreated; following %s", path))
		}
	} else if wasUnavailable {
		onOutput(fmt.Sprintf("✅ Service log is available again; resuming %s", path))
	}
	f.unavailable = false
	f.fileInfo = info

	if info.Size() < f.offset {
		f.reset(0)
		onOutput(fmt.Sprintf("↻ Service log was truncated; restarting from the beginning of %s", path))
	}
	if _, err := file.Seek(f.offset, 0); err != nil {
		f.markUnavailable(path, onOutput)
		return false
	}

	buffer := make([]byte, serviceLogReadBufferSize)
	total := 0
	for total < serviceLogReadLimit {
		remaining := serviceLogReadLimit - total
		readBuffer := buffer
		if remaining < len(readBuffer) {
			readBuffer = readBuffer[:remaining]
		}
		count, readErr := file.Read(readBuffer)
		if count > 0 {
			now := time.Now()
			f.offset += int64(count)
			total += count
			f.consume(readBuffer[:count], now, onOutput)
		}
		if readErr != nil {
			if readErr != io.EOF {
				f.markUnavailable(path, onOutput)
			}
			break
		}
		if count == 0 {
			break
		}
	}
	f.flushStalePartial(time.Now(), onOutput)
	return serviceLogHasUnread(file, f.offset)
}

func serviceLogHasUnread(file *os.File, offset int64) bool {
	info, err := file.Stat()
	return err == nil && !info.IsDir() && info.Size() > offset
}

func (f *serviceLogFollower) consume(chunk []byte, now time.Time, onOutput func(string)) {
	if len(chunk) == 0 {
		return
	}
	if len(f.pending) == 0 {
		f.pendingSince = now
	}
	f.pending = append(f.pending, chunk...)
	for {
		if newline := bytes.IndexByte(f.pending, '\n'); newline >= 0 {
			f.emitPending(newline, onOutput)
			f.pending = f.pending[newline+1:]
			if len(f.pending) == 0 {
				f.pendingSince = time.Time{}
			} else {
				f.pendingSince = now
			}
			continue
		}
		if len(f.pending) >= serviceLogPartialLimit {
			f.emitPending(serviceLogPartialLimit, onOutput)
			f.pending = f.pending[serviceLogPartialLimit:]
			if len(f.pending) == 0 {
				f.pendingSince = time.Time{}
			} else {
				f.pendingSince = now
			}
			continue
		}
		break
	}
}

func (f *serviceLogFollower) flushStalePartial(now time.Time, onOutput func(string)) {
	delay := f.partialFlushAfter
	if delay <= 0 {
		delay = serviceLogPartialDelay
	}
	if len(f.pending) == 0 || f.pendingSince.IsZero() || now.Sub(f.pendingSince) < delay {
		return
	}
	f.emitPending(len(f.pending), onOutput)
	f.pending = nil
	f.pendingSince = time.Time{}
}

func (f *serviceLogFollower) emitPending(end int, onOutput func(string)) {
	line := f.pending[:end]
	line = bytes.TrimSuffix(line, []byte{'\r'})
	onOutput(string(line))
}

func (f *serviceLogFollower) reset(offset int64) {
	f.offset = offset
	f.pending = nil
	f.pendingSince = time.Time{}
}

func (f *serviceLogFollower) markUnavailable(path string, onOutput func(string)) {
	if !f.unavailable {
		onOutput(fmt.Sprintf("⚠️ Service log is unavailable; waiting for %s", path))
	}
	f.unavailable = true
}

func serviceLogTail(path string, count int) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return nil
	}
	return serviceLogTailFromFile(file, info.Size(), count)
}

func serviceLogTailFromFile(file *os.File, snapshotSize int64, count int) []string {
	if count <= 0 || snapshotSize <= 0 {
		return nil
	}
	readSize := snapshotSize
	if readSize > serviceLogTailReadLimit {
		readSize = serviceLogTailReadLimit
	}
	data := make([]byte, int(readSize))
	start := snapshotSize - readSize
	read, err := file.ReadAt(data, start)
	if err != nil && err != io.EOF {
		return nil
	}
	text := strings.TrimRight(string(data[:read]), "\r\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	for index := range lines {
		lines[index] = strings.TrimSuffix(lines[index], "\r")
	}
	return lines
}

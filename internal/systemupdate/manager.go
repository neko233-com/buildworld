package systemupdate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const MaxBundleBytes int64 = 512 << 20

var (
	ErrUpdateInProgress = errors.New("system update is already in progress")
	ErrAutomaticDisabled = errors.New("automatic system updates are disabled; an administrator must start the update")
	versionPattern      = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	checksumPattern     = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
)

type Request struct {
	Version   string
	SHA256    string
	Bundle    io.Reader
	Automatic bool
}

type Status struct {
	Status      string `json:"status"`
	OperationID string `json:"operation_id,omitempty"`
	Version     string `json:"version,omitempty"`
	Message     string `json:"message,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type Service interface {
	Status() Status
	Start(context.Context, Request) (Status, error)
}

type Options struct {
	CurrentVersion string
	ConfigPath     string
	InstallDir     string
	StateDir       string
	ScriptPath     string
	Launch         func(script string, args []string, logPath string) error
	Now            func() time.Time
}

type Manager struct {
	currentVersion string
	configPath     string
	installDir     string
	stateDir       string
	scriptPath     string
	launch         func(script string, args []string, logPath string) error
	now            func() time.Time
	mu             sync.Mutex
}

func NewManager(options Options) (*Manager, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && options.Launch == nil {
		return nil, fmt.Errorf("system update is unsupported on %s", runtime.GOOS)
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve server executable: %w", err)
	}
	if options.InstallDir == "" {
		options.InstallDir = filepath.Dir(executable)
	}
	if options.StateDir == "" {
		userConfig, configErr := os.UserConfigDir()
		if configErr != nil {
			return nil, fmt.Errorf("resolve update state directory: %w", configErr)
		}
		options.StateDir = filepath.Join(userConfig, "buildworld", "updates")
	}
	if options.ScriptPath == "" {
		options.ScriptPath = filepath.Join(options.InstallDir, "apply-update.sh")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Launch == nil {
		options.Launch = launchDetached
	}
	if err := validateInstallDir(options.InstallDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(options.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create update state directory: %w", err)
	}
	return &Manager{
		currentVersion: options.CurrentVersion,
		configPath:     options.ConfigPath,
		installDir:     filepath.Clean(options.InstallDir),
		stateDir:       filepath.Clean(options.StateDir),
		scriptPath:     filepath.Clean(options.ScriptPath),
		launch:         options.Launch,
		now:            options.Now,
	}, nil
}

func (m *Manager) Status() Status {
	if m == nil {
		return Status{Status: "unsupported", Message: "system update is unavailable"}
	}
	data, err := os.ReadFile(m.statusPath())
	if err == nil {
		var status Status
		if json.Unmarshal(data, &status) == nil && status.Status != "" {
			return status
		}
	}
	return Status{Status: "idle", Version: m.currentVersion}
}

func (m *Manager) Start(ctx context.Context, request Request) (Status, error) {
	if m == nil {
		return Status{}, errors.New("system update is unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if request.Automatic {
		return Status{}, ErrAutomaticDisabled
	}

	version := strings.TrimPrefix(strings.TrimSpace(request.Version), "v")
	checksum := strings.ToLower(strings.TrimSpace(request.SHA256))
	if !versionPattern.MatchString(version) {
		return Status{}, errors.New("version must use semantic form x.y.z")
	}
	if !checksumPattern.MatchString(checksum) {
		return Status{}, errors.New("sha256 must contain exactly 64 hexadecimal characters")
	}
	if request.Bundle == nil {
		return Status{}, errors.New("bundle is required")
	}
	if _, err := os.Stat(m.scriptPath); err != nil {
		return Status{}, fmt.Errorf("trusted update helper is unavailable: %w", err)
	}
	if err := m.acquireLock(); err != nil {
		return Status{}, err
	}
	locked := true
	defer func() {
		if locked {
			_ = os.Remove(m.lockPath())
		}
	}()

	operationID, err := randomOperationID()
	if err != nil {
		return Status{}, err
	}
	operationDir := filepath.Join(m.stateDir, "operations", operationID)
	if err := os.MkdirAll(operationDir, 0o700); err != nil {
		return Status{}, fmt.Errorf("create update operation: %w", err)
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = os.RemoveAll(operationDir)
		}
	}()
	bundlePath := filepath.Join(operationDir, "bundle.tar.gz")
	actualChecksum, size, err := writeBundle(ctx, bundlePath, request.Bundle)
	if err != nil {
		return Status{}, err
	}
	if size == 0 {
		return Status{}, errors.New("bundle is empty")
	}
	if actualChecksum != checksum {
		return Status{}, fmt.Errorf("bundle checksum mismatch: got %s", actualChecksum)
	}
	if err := validateBundle(bundlePath); err != nil {
		return Status{}, err
	}

	helperPath := filepath.Join(operationDir, "apply-update.sh")
	if err := copyFile(m.scriptPath, helperPath, 0o700); err != nil {
		return Status{}, fmt.Errorf("stage trusted update helper: %w", err)
	}
	now := m.now().UTC().Format(time.RFC3339)
	status := Status{
		Status:      "accepted",
		OperationID: operationID,
		Version:     version,
		Message:     "update bundle verified and queued",
		StartedAt:   now,
		UpdatedAt:   now,
	}
	if err := m.writeStatus(status); err != nil {
		return Status{}, err
	}
	args := []string{
		bundlePath,
		checksum,
		version,
		operationID,
		m.statusPath(),
		m.lockPath(),
		m.installDir,
		m.configPath,
	}
	if err := m.launch(helperPath, args, filepath.Join(operationDir, "update.log")); err != nil {
		status.Status = "failed"
		status.Message = "failed to launch update helper"
		status.UpdatedAt = m.now().UTC().Format(time.RFC3339)
		_ = m.writeStatus(status)
		return Status{}, fmt.Errorf("launch update helper: %w", err)
	}
	accepted = true
	locked = false
	return status, nil
}

func (m *Manager) acquireLock() error {
	lock, err := os.OpenFile(m.lockPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = fmt.Fprintf(lock, "%s\n", m.now().UTC().Format(time.RFC3339))
		return lock.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create update lock: %w", err)
	}
	info, statErr := os.Stat(m.lockPath())
	if statErr == nil && m.now().Sub(info.ModTime()) > time.Hour {
		if removeErr := os.Remove(m.lockPath()); removeErr == nil {
			return m.acquireLock()
		}
	}
	return ErrUpdateInProgress
}

func (m *Manager) statusPath() string { return filepath.Join(m.stateDir, "update-status.json") }
func (m *Manager) lockPath() string   { return filepath.Join(m.stateDir, "update.lock") }

func (m *Manager) writeStatus(status Status) error {
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("encode update status: %w", err)
	}
	temporary := m.statusPath() + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write update status: %w", err)
	}
	if err := os.Rename(temporary, m.statusPath()); err != nil {
		return fmt.Errorf("activate update status: %w", err)
	}
	return nil
}

func writeBundle(ctx context.Context, destination string, source io.Reader) (string, int64, error) {
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, fmt.Errorf("create staged bundle: %w", err)
	}
	defer target.Close()
	hash := sha256.New()
	limited := &io.LimitedReader{R: source, N: MaxBundleBytes + 1}
	written, err := io.Copy(io.MultiWriter(target, hash), &contextReader{ctx: ctx, reader: limited})
	if err != nil {
		return "", written, fmt.Errorf("stage update bundle: %w", err)
	}
	if written > MaxBundleBytes {
		return "", written, fmt.Errorf("bundle exceeds %d bytes", MaxBundleBytes)
	}
	if err := target.Sync(); err != nil {
		return "", written, fmt.Errorf("sync update bundle: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func validateBundle(bundlePath string) error {
	file, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("open staged bundle: %w", err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("bundle is not a valid gzip archive: %w", err)
	}
	defer compressed.Close()

	required := map[string]bool{
		"buildworld":                false,
		"buildworld-server":         false,
		"buildworld-worker":         false,
		"apply-update.sh":           false,
		"web/dist/index.html":       false,
		"sdk/pipeline/index.d.ts":   false,
		"sdk/pipeline/index.js":     false,
		"sdk/pipeline/package.json": false,
	}
	reader := tar.NewReader(compressed)
	var entries int
	var expanded int64
	for {
		header, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("read bundle archive: %w", nextErr)
		}
		entries++
		if entries > 20_000 {
			return errors.New("bundle contains too many entries")
		}
		name := path.Clean(strings.TrimPrefix(header.Name, "./"))
		if name == "." {
			continue
		}
		if path.IsAbs(header.Name) || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsRune(name, '\\') {
			return fmt.Errorf("bundle contains unsafe path %q", header.Name)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return fmt.Errorf("bundle contains unsupported entry %q", name)
		}
		if header.Size < 0 {
			return fmt.Errorf("bundle contains invalid size for %q", name)
		}
		expanded += header.Size
		if expanded > 2*MaxBundleBytes {
			return errors.New("expanded bundle is too large")
		}
		if _, exists := required[name]; exists {
			required[name] = true
			if name == "buildworld" || name == "buildworld-server" || name == "buildworld-worker" || name == "apply-update.sh" {
				if header.FileInfo().Mode()&0o111 == 0 {
					return fmt.Errorf("bundle executable %q is not executable", name)
				}
			}
		}
	}
	for name, present := range required {
		if !present {
			return fmt.Errorf("bundle is incomplete: missing %s", name)
		}
	}
	return nil
}

func validateInstallDir(directory string) error {
	directory = filepath.Clean(directory)
	home, _ := os.UserHomeDir()
	if directory == "" || directory == "." || directory == string(filepath.Separator) || directory == filepath.Clean(home) || filepath.Dir(directory) == directory {
		return fmt.Errorf("unsafe update installation directory %q", directory)
	}
	return nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func randomOperationID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("read system random source: %w", err)
	}
	return hex.EncodeToString(data), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(buffer)
	}
}

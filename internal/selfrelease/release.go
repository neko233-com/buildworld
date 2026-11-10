// Package selfrelease builds and verifies the controller's own local release.
package selfrelease

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var (
	versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	commitPattern  = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

type record struct {
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	BundleSHA   string `json:"bundle_sha256"`
	Phase       string `json:"phase"`
	OperationID string `json:"operation_id,omitempty"`
	SubmittedAt string `json:"submitted_at,omitempty"`
}

// Run packages a reviewed commit and reconciles its authenticated local update.
// The durable record survives the controller restart and never contains credentials.
func Run(ctx context.Context, getenv func(string) string) error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return errors.New("self-release requires a macOS or Linux builtin executor")
	}
	version := strings.TrimSpace(getenv("RELEASE_VERSION"))
	commit := strings.ToLower(strings.TrimSpace(getenv("SOURCE_COMMIT")))
	job := strings.TrimSpace(getenv("JOB_NAME"))
	buildNumber := strings.TrimSpace(getenv("BUILD_NUMBER"))
	if !versionPattern.MatchString(version) || !commitPattern.MatchString(commit) {
		return errors.New("RELEASE_VERSION and the exact hexadecimal SOURCE_COMMIT are required")
	}
	version = strings.TrimPrefix(version, "v")
	if job == "" || len(job) > 256 {
		return errors.New("JOB_NAME is required")
	}
	if n, err := strconv.ParseUint(buildNumber, 10, 64); err != nil || n == 0 {
		return errors.New("BUILD_NUMBER must be a positive integer")
	}
	if getenv("BUILDWORLD_UPDATE_TOKEN") == "" {
		return errors.New("BUILDWORLD_UPDATE_TOKEN with system:update scope is required")
	}
	git := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "HEAD^{commit}")
	output, err := git.Output()
	if err != nil || strings.TrimSpace(string(output)) != commit {
		return errors.New("workspace commit does not match SOURCE_COMMIT")
	}
	if value := strings.TrimSpace(getenv("GIT_COMMIT")); value != "" && !strings.EqualFold(value, commit) {
		return errors.New("BuildWorld GIT_COMMIT does not match SOURCE_COMMIT")
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	jobHash := sha256.Sum256([]byte(job))
	directory := filepath.Join(configRoot, "buildworld", "self-release", hex.EncodeToString(jobHash[:]), buildNumber)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	statePath := filepath.Join(directory, "state.json")
	bundlePath := filepath.Join(directory, "bundle.tar.gz")
	state, err := loadRecord(statePath)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("Running the complete local release gate and six-platform packaging.")
		command := exec.CommandContext(ctx, "pwsh", "-NoProfile", "-File", "scripts/release-local.ps1", "-Version", version)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("release packaging failed: %w", err)
		}
		platform := runtime.GOOS + "-" + runtime.GOARCH
		archive, hash, err := verifyArtifacts(filepath.Join("release", "v"+version), version, commit, platform)
		if err != nil {
			return err
		}
		if err := copyAtomic(archive, bundlePath); err != nil {
			return fmt.Errorf("retain verified local bundle: %w", err)
		}
		state = record{Version: version, Commit: commit, BundleSHA: hash, Phase: "prepared"}
		if err := saveRecord(statePath, state); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if state.Version != version || state.Commit != commit || len(state.BundleSHA) != 64 {
		return errors.New("existing self-release state does not match the requested build")
	}
	if hash, err := fileSHA256(bundlePath); err != nil || hash != state.BundleSHA {
		return errors.New("retained self-release bundle checksum mismatch")
	}
	endpoint := strings.TrimSpace(getenv("BUILDWORLD_SELFRELEASE_URL"))
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8080"
	}
	return updateController(ctx, endpoint, getenv("BUILDWORLD_UPDATE_TOKEN"), statePath, bundlePath, &state)
}

func loadRecord(filename string) (record, error) {
	var value record
	data, err := os.ReadFile(filename)
	if err != nil {
		return value, err
	}
	if len(data) > 64<<10 || json.Unmarshal(data, &value) != nil {
		return value, errors.New("invalid self-release state")
	}
	switch value.Phase {
	case "prepared", "submitting", "submitted", "succeeded":
		return value, nil
	default:
		return value, errors.New("invalid self-release state phase")
	}
}

func saveRecord(filename string, value record) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(filename), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), filename)
}

func copyAtomic(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(destination), ".bundle-*")
	if err != nil {
		return err
	}
	defer os.Remove(output.Name())
	if err := output.Chmod(0o600); err != nil {
		output.Close()
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	return os.Rename(output.Name(), destination)
}

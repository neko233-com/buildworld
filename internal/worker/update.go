package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/neko233-com/buildworld/internal/workercontrol"
	"github.com/neko233-com/buildworld/internal/workerupdate"
)

type updatePlan struct {
	OriginalPID      int      `json:"original_pid"`
	OperationID      string   `json:"operation_id"`
	Version          string   `json:"version"`
	BinarySHA256     string   `json:"binary_sha256"`
	Executable       string   `json:"executable"`
	Candidate        string   `json:"candidate"`
	StateRoot        string   `json:"state_root"`
	Directory        string   `json:"directory"`
	Arguments        []string `json:"arguments"`
	WorkingDirectory string   `json:"working_directory"`
}

func copyExclusive(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func fileDigest(file string) (string, error) {
	in, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer in.Close()
	hash := sha256.New()
	_, err = io.Copy(hash, in)
	return hex.EncodeToString(hash.Sum(nil)), err
}

func (d *Daemon) stageUpdate(op *workercontrol.Operation) error {
	if op.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		return d.failUpdate(op.ID, "Update platform does not match this worker")
	}
	snapshot := d.executor.Snapshot()
	_ = d.executor.Configure(snapshot.Capacity, true)
	if err := d.saveState(); err != nil {
		return err
	}
	if len(snapshot.Tasks) > 0 {
		return nil
	} // A later heartbeat retries only after all owners release.
	if d.state.Update != nil && d.state.Update.OperationID == op.ID {
		return nil
	}
	directory := filepath.Join(d.stateRoot, "updates", op.ID)
	if err := os.MkdirAll(filepath.Dir(directory), 0700); err != nil {
		return d.failUpdate(op.ID, "Cannot create private update storage")
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		return d.failUpdate(op.ID, "Update staging directory already exists or cannot be created; inspect retained material")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.server+"/api/agents/heartbeat/"+d.id+"/bundle/"+op.ID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Buildworld-Worker-Token", d.token)
	response, err := (&http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		return d.failUpdate(op.ID, "Unable to download the authenticated update bundle")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return d.failUpdate(op.ID, fmt.Sprintf("Worker bundle download HTTP %d", response.StatusCode))
	}
	archive := filepath.Join(directory, "bundle")
	if err = workerupdate.StageArchive(response.Body, archive, op.SHA256); err != nil {
		return d.failUpdate(op.ID, err.Error())
	}
	staged := filepath.Join(directory, "candidate")
	identity, err := workerupdate.Extract(archive, staged, op.Version, op.Platform)
	if err != nil {
		return d.failUpdate(op.ID, err.Error())
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	plan := &updatePlan{OriginalPID: os.Getpid(), OperationID: op.ID, Version: op.Version, BinarySHA256: identity.BinarySHA256, Executable: executable, Candidate: filepath.Join(staged, filepath.Base(executable)), StateRoot: d.stateRoot, Directory: directory, Arguments: os.Args[1:], WorkingDirectory: workingDirectory}
	// Release bundles use the canonical worker name, even for renamed installations.
	name := "buildworld-worker"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	plan.Candidate = filepath.Join(staged, name)
	command := exec.CommandContext(ctx, plan.Candidate, "--version")
	hideProcess(command)
	output, err := command.Output()
	if err != nil {
		return d.failUpdate(op.ID, "Candidate version probe failed")
	}
	var version struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(output, &version) != nil || strings.TrimPrefix(version.Version, "v") != strings.TrimPrefix(op.Version, "v") {
		return d.failUpdate(op.ID, "Candidate version probe does not match the release")
	}
	helper := filepath.Join(directory, "worker-helper")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	if err = copyExclusive(executable, helper); err != nil {
		return d.failUpdate(op.ID, "Cannot preserve the update helper")
	}
	d.state.Update = plan
	if err = d.saveState(); err != nil {
		return err
	}
	planPath := filepath.Join(directory, "plan.json")
	if err = atomicJSON(planPath, plan); err != nil {
		return err
	}
	command = exec.Command(helper, "--apply-worker-update", planPath)
	command.Dir = workingDirectory
	hideProcess(command)
	if err = command.Start(); err != nil {
		d.state.Update = nil
		return d.failUpdate(op.ID, "Cannot start the trusted update helper")
	}
	_ = command.Process.Release()
	d.shutdown()
	return nil
}

func (d *Daemon) failUpdate(id, message string) error {
	d.state.Completion = &workercontrol.Completion{ID: id, Status: "failed", Message: message}
	return d.saveState()
}

// ApplyUpdate runs only in a private copy of the original worker. A file lock
// proves the previous daemon has exited before replacing its executable.
func ApplyUpdate(planPath string) error {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	var plan updatePlan
	if len(data) > 64<<10 || json.Unmarshal(data, &plan) != nil || !workerupdate.DigestPattern.MatchString(plan.BinarySHA256) || !workerupdate.VersionPattern.MatchString(plan.Version) {
		return errors.New("invalid update plan")
	}
	helper, err := os.Executable()
	if err != nil {
		return err
	}
	if filepath.Dir(helper) != plan.Directory || filepath.Dir(planPath) != plan.Directory || filepath.Dir(plan.Directory) != filepath.Join(plan.StateRoot, "updates") || filepath.Dir(plan.Candidate) != filepath.Join(plan.Directory, "candidate") || !filepath.IsAbs(plan.Executable) || filepath.Clean(plan.Executable) == filepath.Clean(helper) {
		return errors.New("update plan paths are inconsistent")
	}
	lock, err := os.OpenFile(filepath.Join(plan.StateRoot, "daemon.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	deadline := time.Now().Add(45 * time.Second)
	for !tryFileLock(lock) {
		if time.Now().After(deadline) {
			return errors.New("original worker did not release ownership")
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer unlockFile(lock)
	digest, err := fileDigest(plan.Candidate)
	if err != nil || digest != plan.BinarySHA256 {
		return errors.New("staged worker digest changed")
	}
	backup := filepath.Join(plan.Directory, "original-worker")
	if err = os.Rename(plan.Executable, backup); err != nil {
		return fmt.Errorf("preserve original worker: %w", err)
	}
	if err = copyExclusive(plan.Candidate, plan.Executable); err != nil {
		_ = os.Rename(backup, plan.Executable)
		return err
	}
	metadataPath := filepath.Join(filepath.Dir(plan.Executable), "buildworld-worker.release-info.json")
	originalMetadata, metadataErr := os.ReadFile(metadataPath)
	metadata, err := os.ReadFile(filepath.Join(filepath.Dir(plan.Candidate), "release-info.json"))
	if err == nil {
		err = os.WriteFile(metadataPath, metadata, 0600)
	}
	if err != nil {
		_ = os.Remove(plan.Executable)
		_ = os.Rename(backup, plan.Executable)
		return err
	}
	// The child acquires the same ownership lock before registering. Release only
	// after the binary and metadata have been replaced as one local transaction.
	unlockFile(lock)
	command := exec.Command(plan.Executable, plan.Arguments...)
	command.Dir = plan.WorkingDirectory
	hideProcess(command)
	if err = command.Start(); err == nil {
		exited := make(chan error, 1)
		go func() { exited <- command.Wait() }()
		running := true
		ready := filepath.Join(plan.Directory, "ready.json")
		deadline = time.Now().Add(75 * time.Second)
		for running && time.Now().Before(deadline) {
			raw, e := os.ReadFile(ready)
			if e == nil {
				var receipt struct {
					PID     int    `json:"pid"`
					ID      string `json:"id"`
					Version string `json:"version"`
				}
				if json.Unmarshal(raw, &receipt) == nil && receipt.ID == plan.OperationID && receipt.Version == buildVersion(plan.Version) && receipt.PID == command.Process.Pid {
					return nil
				}
			}
			select {
			case <-exited:
				running = false
			case <-time.After(200 * time.Millisecond):
			}
		}
		if running {
			_ = command.Process.Kill()
			<-exited
		}
	}
	// Only the PID returned by Start belongs to this helper. The original
	// executable and metadata are retained; no workspace or runtime data is moved.
	deadline = time.Now().Add(15 * time.Second)
	for !tryFileLock(lock) {
		if time.Now().After(deadline) {
			return errors.New("updated worker still owns the process lock; rollback requires inspection")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err = os.Remove(plan.Executable); err != nil {
		return err
	}
	if err = os.Rename(backup, plan.Executable); err != nil {
		return err
	}
	if metadataErr == nil {
		_ = os.WriteFile(metadataPath, originalMetadata, 0600)
	} else {
		_ = os.Remove(metadataPath)
	}
	statePath := filepath.Join(plan.StateRoot, "state.json")
	raw, err := os.ReadFile(statePath)
	if err != nil {
		return err
	}
	var state daemonState
	if json.Unmarshal(raw, &state) != nil {
		return errors.New("cannot read rollback state")
	}
	state.Update = nil
	state.Completion = &workercontrol.Completion{ID: plan.OperationID, Status: "rolled_back", Message: "New worker did not acknowledge readiness; original binary restored; worker remains drained"}
	if err = atomicJSON(statePath, state); err != nil {
		return err
	}
	unlockFile(lock)
	command = exec.Command(plan.Executable, plan.Arguments...)
	command.Dir = plan.WorkingDirectory
	hideProcess(command)
	if err = command.Start(); err != nil {
		return fmt.Errorf("original worker restart failed: %w", err)
	}
	_ = command.Process.Release()
	return nil
}

func buildVersion(version string) string { return strings.TrimPrefix(version, "v") }

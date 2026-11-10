package selfrelease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type updateStatus struct {
	CurrentVersion string `json:"current_version"`
	Operation      struct {
		Status      string `json:"status"`
		OperationID string `json:"operation_id"`
		Version     string `json:"version"`
		UpdatedAt   string `json:"updated_at"`
	} `json:"operation"`
}

type runningVersion struct {
	Version      string `json:"version"`
	SourceCommit string `json:"source_commit"`
}

var errUpdateRejected = errors.New("system update rejected")

func localEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("self-release endpoint must be a local HTTP origin")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return "", errors.New("self-release endpoint must be loopback")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func updateController(ctx context.Context, endpoint, token, statePath, bundlePath string, state *record) error {
	base, err := localEndpoint(endpoint)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if state.Phase == "prepared" {
		version, err := readRunningVersion(ctx, client, base, token)
		if err != nil {
			return err
		}
		if releaseMatches(version, state) {
			if err := checkHealth(ctx, client, base, token); err != nil {
				return err
			}
			state.Phase = "succeeded"
			return saveRecord(statePath, *state)
		}
		status, err := readUpdateStatus(ctx, client, base, token)
		if err != nil {
			return err
		}
		if status.Operation.Status == "accepted" || status.Operation.Status == "applying" {
			return errors.New("another system update is already in progress")
		}
		state.Phase = "submitting"
		state.SubmittedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveRecord(statePath, *state); err != nil {
			return err
		}
		operationID, err := submitBundle(ctx, client, base, token, bundlePath, state)
		if err == nil {
			state.OperationID = operationID
			state.Phase = "submitted"
			if err := saveRecord(statePath, *state); err != nil {
				return err
			}
		} else if errors.Is(err, errUpdateRejected) {
			return err
		} else {
			// The controller can restart after accepting the upload but before
			// returning a response. Reconcile the durable operation; never resend.
			fmt.Println("Update response interrupted; checking the recorded operation without resubmitting.")
		}
	}
	if state.Phase == "succeeded" {
		version, err := readRunningVersion(ctx, client, base, token)
		if err != nil || !releaseMatches(version, state) {
			return errors.New("running release no longer matches the completed self-release")
		}
		return checkHealth(ctx, client, base, token)
	}
	return waitForOperation(ctx, client, base, token, statePath, state)
}

func waitForOperation(ctx context.Context, client *http.Client, base, token, statePath string, state *record) error {
	started, err := time.Parse(time.RFC3339Nano, state.SubmittedAt)
	if err != nil {
		return errors.New("self-release submission time is missing")
	}
	deadline := time.NewTimer(3 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		status, readErr := readUpdateStatus(ctx, client, base, token)
		if readErr == nil {
			op := status.Operation
			if state.OperationID == "" && op.OperationID != "" && strings.TrimPrefix(op.Version, "v") == state.Version {
				updated, parseErr := time.Parse(time.RFC3339Nano, op.UpdatedAt)
				if parseErr == nil && !updated.Before(started.Add(-2*time.Second)) {
					state.OperationID, state.Phase = op.OperationID, "submitted"
					if err := saveRecord(statePath, *state); err != nil {
						return err
					}
				}
			}
			if state.OperationID != "" && op.OperationID == state.OperationID {
				switch op.Status {
				case "failed", "rolled_back":
					return fmt.Errorf("system update %s; inspect the update audit and retry with a new build", op.Status)
				case "succeeded":
					version, err := readRunningVersion(ctx, client, base, token)
					if err != nil || !releaseMatches(version, state) {
						return errors.New("updated server version or source commit does not match the release")
					}
					if err := checkHealth(ctx, client, base, token); err != nil {
						return err
					}
					state.Phase = "succeeded"
					if err := saveRecord(statePath, *state); err != nil {
						return err
					}
					fmt.Printf("BuildWorld %s is healthy at source commit %s.\n", state.Version, state.Commit)
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("update outcome could not be verified; no duplicate request was sent")
		case <-ticker.C:
		}
	}
}

func releaseMatches(version runningVersion, state *record) bool {
	return strings.TrimPrefix(version.Version, "v") == state.Version && version.SourceCommit == state.Commit
}

func readUpdateStatus(ctx context.Context, client *http.Client, base, token string) (updateStatus, error) {
	var status updateStatus
	err := getJSON(ctx, client, base+"/api/system/update/", token, &status)
	return status, err
}

func readRunningVersion(ctx context.Context, client *http.Client, base, token string) (runningVersion, error) {
	var version runningVersion
	err := getJSON(ctx, client, base+"/api/version", token, &version)
	return version, err
}

func checkHealth(ctx context.Context, client *http.Client, base, token string) error {
	var health struct {
		Status string `json:"status"`
	}
	if err := getJSON(ctx, client, base+"/api/health", token, &health); err != nil || health.Status != "ok" {
		return errors.New("updated BuildWorld health check failed")
	}
	return nil
}

func getJSON(ctx context.Context, client *http.Client, endpoint, token string, target interface{}) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return errors.New("cannot reach the local BuildWorld controller")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("local BuildWorld API returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(target)
}

func submitBundle(ctx context.Context, client *http.Client, base, token, bundle string, state *record) (string, error) {
	file, err := os.Open(bundle)
	if err != nil {
		return "", err
	}
	defer file.Close()
	reader, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/system/update/", reader)
	if err != nil {
		reader.Close()
		writer.Close()
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", form.FormDataContentType())
	written := make(chan error, 1)
	go func() {
		err := form.WriteField("mode", "manual")
		if err == nil {
			err = form.WriteField("version", state.Version)
		}
		if err == nil {
			err = form.WriteField("sha256", state.BundleSHA)
		}
		if err == nil {
			var part io.Writer
			part, err = form.CreateFormFile("bundle", "buildworld.tar.gz")
			if err == nil {
				_, err = io.Copy(part, file)
			}
		}
		if err == nil {
			err = form.Close()
		}
		_ = writer.CloseWithError(err)
		written <- err
	}()
	response, err := client.Do(request)
	if err != nil {
		_ = reader.CloseWithError(err)
		<-written
		return "", errors.New("system update request did not return a response")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		_ = reader.Close()
		<-written
		var rejection struct {
			Code string `json:"code"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&rejection) == nil && rejection.Code == "unsupported_update_supervisor" {
			return "", fmt.Errorf("%w: BuildWorld is managed by a system LaunchDaemon; an administrator must migrate it to a per-user LaunchAgent", errUpdateRejected)
		}
		return "", fmt.Errorf("%w: HTTP %d", errUpdateRejected, response.StatusCode)
	}
	if err := <-written; err != nil {
		return "", errors.New("system update bundle upload was interrupted")
	}
	var accepted struct {
		OperationID string `json:"operation_id"`
		Version     string `json:"version"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&accepted); err != nil || accepted.Status != "accepted" || accepted.OperationID == "" || strings.TrimPrefix(accepted.Version, "v") != state.Version {
		return "", errors.New("system update returned invalid acceptance metadata")
	}
	return accepted.OperationID, nil
}

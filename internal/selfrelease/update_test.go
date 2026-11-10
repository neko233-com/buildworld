package selfrelease

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type updateFixture struct {
	mu           sync.Mutex
	state        string
	version      string
	commit       string
	posts        int
	dropResponse bool
	wrongCommit  bool
	hold         bool
	expectedSHA  string
	wantCommit   string
}

func (f *updateFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer test-token" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/version":
		_ = json.NewEncoder(w).Encode(map[string]string{"version": f.version, "source_commit": f.commit})
	case r.Method == http.MethodGet && r.URL.Path == "/api/health":
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	case r.Method == http.MethodGet && r.URL.Path == "/api/system/update/":
		operation := map[string]string{"status": f.state}
		if f.state != "idle" {
			operation["operation_id"] = "operation-1"
			operation["version"] = "1.2.3"
			operation["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"current_version": f.version, "operation": operation})
	case r.Method == http.MethodPost && r.URL.Path == "/api/system/update/":
		f.posts++
		wantSHA := f.expectedSHA
		if wantSHA == "" {
			wantSHA = strings.Repeat("b", 64)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil || r.FormValue("mode") != "manual" || r.FormValue("version") != "1.2.3" || r.FormValue("sha256") != wantSHA {
			http.Error(w, "invalid upload", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("bundle")
		if err != nil {
			http.Error(w, "missing bundle", http.StatusBadRequest)
			return
		}
		data, _ := io.ReadAll(file)
		file.Close()
		if string(data) != "bundle" {
			http.Error(w, "invalid bundle", http.StatusBadRequest)
			return
		}
		if f.hold {
			f.state = "applying"
		} else if f.state != "rolled_back" {
			f.state, f.version, f.commit = "succeeded", "v1.2.3", strings.Repeat("a", 40)
			if f.wantCommit != "" {
				f.commit = f.wantCommit
			}
			if f.wrongCommit {
				f.commit = strings.Repeat("c", 40)
			}
		}
		if f.dropResponse {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				connection.Close()
			}
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "operation_id": "operation-1", "version": "1.2.3"})
	default:
		http.NotFound(w, r)
	}
}

func TestUpdateControllerVerifiesActualCommitAndAvoidsDuplicateSubmission(t *testing.T) {
	for _, tc := range []struct {
		name, initial string
		drop, wrong   bool
		wantError     bool
	}{
		{"successful update", "idle", false, false, false},
		{"lost acceptance response", "idle", true, false, false},
		{"rollback", "rolled_back", false, false, true},
		{"wrong running commit", "idle", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &updateFixture{state: tc.initial, version: "v1.2.2", dropResponse: tc.drop, wrongCommit: tc.wrong}
			server := httptest.NewServer(fixture)
			defer server.Close()
			root := t.TempDir()
			bundle := filepath.Join(root, "bundle.tar.gz")
			if err := os.WriteFile(bundle, []byte("bundle"), 0o600); err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(root, "state.json")
			state := record{Version: "1.2.3", Commit: strings.Repeat("a", 40), BundleSHA: strings.Repeat("b", 64), Phase: "prepared"}
			err := updateController(context.Background(), server.URL, "test-token", statePath, bundle, &state)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v wantError=%t", err, tc.wantError)
			}
			if fixture.posts != 1 {
				t.Fatalf("uploads=%d, want 1", fixture.posts)
			}
			if !tc.wantError {
				persisted, err := loadRecord(statePath)
				if err != nil || persisted.Phase != "succeeded" || persisted.OperationID != "operation-1" {
					t.Fatalf("persisted=%+v error=%v", persisted, err)
				}
				if err := updateController(context.Background(), server.URL, "test-token", statePath, bundle, &persisted); err != nil || fixture.posts != 1 {
					t.Fatalf("recovery error=%v uploads=%d", err, fixture.posts)
				}
			}
		})
	}
}

func TestUpdateControllerResumesSubmittedOperation(t *testing.T) {
	fixture := &updateFixture{state: "idle", version: "v1.2.2", hold: true}
	server := httptest.NewServer(fixture)
	defer server.Close()
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.WriteFile(bundle, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, "state.json")
	state := record{Version: "1.2.3", Commit: strings.Repeat("a", 40), BundleSHA: strings.Repeat("b", 64), Phase: "prepared"}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := updateController(ctx, server.URL, "test-token", statePath, bundle, &state); err == nil {
		t.Fatal("an unfinished update was reported as successful")
	}
	persisted, err := loadRecord(statePath)
	if err != nil || persisted.Phase != "submitted" {
		t.Fatalf("persisted=%+v error=%v", persisted, err)
	}
	fixture.mu.Lock()
	fixture.state, fixture.version, fixture.commit = "succeeded", "v1.2.3", strings.Repeat("a", 40)
	fixture.mu.Unlock()
	if err := updateController(context.Background(), server.URL, "test-token", statePath, bundle, &persisted); err != nil {
		t.Fatal(err)
	}
	if fixture.posts != 1 {
		t.Fatalf("uploads after recovery=%d, want 1", fixture.posts)
	}
}

func TestSubmitBundleReportsUnsupportedSupervisor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"code":"unsupported_update_supervisor"}`)
	}))
	defer server.Close()
	bundle := filepath.Join(t.TempDir(), "bundle")
	if err := os.WriteFile(bundle, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := submitBundle(context.Background(), server.Client(), server.URL, "test-token", bundle, &record{Version: "1.2.3"})
	if err == nil || !strings.Contains(err.Error(), "LaunchDaemon") {
		t.Fatalf("rejection error = %v", err)
	}
}

func TestSelfReleaseRejectsNonLoopbackEndpoints(t *testing.T) {
	for _, endpoint := range []string{"http://192.168.1.2:8080", "https://127.0.0.1", "http://example.com", "http://user@localhost", "http://localhost/path"} {
		if _, err := localEndpoint(endpoint); err == nil {
			t.Fatalf("accepted endpoint %q", endpoint)
		}
	}
}

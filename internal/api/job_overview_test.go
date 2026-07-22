package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestListProjectBuildOverviewsReturnsEmptyArray(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "empty-job-overview.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })

	response := httptest.NewRecorder()
	(&handlers{d: Deps{Store: data}}).listProjectBuildOverviews(
		response,
		httptest.NewRequest(http.MethodGet, "/api/projects/job-overview", nil),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("body = %q, want []", response.Body.String())
	}
}

func TestJobOverviewRouteReturnsOnlyBoundedBuildData(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "job-overview-route.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	viewer, err := data.CreateUser("overview-viewer", "overview@example.test", "unused", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject("overview-project", "", "", "git", "main", `{}`, viewer.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(
		project.ID,
		1,
		"manual",
		"main",
		"secret-commit",
		`{"secret_parameter":"must-not-leak"}`,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.DB().Exec(
		"UPDATE builds SET status = 'success', log = ? WHERE id = ?",
		"secret-log-must-not-leak", build.ID,
	); err != nil {
		t.Fatal(err)
	}

	jwt := auth.NewJWT("job-overview-route-test-secret")
	token, err := jwt.Generate(viewer.ID, viewer.Role, viewer.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: data, JWT: jwt})
	request := httptest.NewRequest(http.MethodGet, "/api/projects/job-overview", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", contentType)
	}
	for _, forbidden := range []string{"must-not-leak", "secret-commit", `"parameters"`, `"log"`, `"commit_sha"`} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response leaks %q: %s", forbidden, response.Body.String())
		}
	}

	var payload []*store.ProjectBuildOverview
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload[0].ProjectID != project.ID {
		t.Fatalf("payload = %#v", payload)
	}
	if payload[0].Latest == nil || payload[0].Latest.ID != build.ID {
		t.Fatalf("latest = %#v", payload[0].Latest)
	}
	if payload[0].LastSuccess == nil || payload[0].LastSuccess.ID != build.ID {
		t.Fatalf("last success = %#v", payload[0].LastSuccess)
	}
	if payload[0].LastFailure != nil {
		t.Fatalf("last failure = %#v, want nil", payload[0].LastFailure)
	}

	var rawPayload []map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &rawPayload); err != nil {
		t.Fatal(err)
	}
	assertJSONKeys(t, rawPayload[0], []string{
		"project_id", "latest", "last_success", "last_failure", "recent_statuses",
	})
	var rawLatest map[string]json.RawMessage
	if err := json.Unmarshal(rawPayload[0]["latest"], &rawLatest); err != nil {
		t.Fatal(err)
	}
	assertJSONKeys(t, rawLatest, []string{
		"id", "number", "status", "started_at", "finished_at", "duration_ms",
	})
}

func assertJSONKeys(t *testing.T, value map[string]json.RawMessage, expected []string) {
	t.Helper()
	if len(value) != len(expected) {
		t.Fatalf("JSON keys = %v, want %v", mapKeys(value), expected)
	}
	for _, key := range expected {
		if _, ok := value[key]; !ok {
			t.Fatalf("JSON keys = %v, missing %q", mapKeys(value), key)
		}
	}
}

func mapKeys(value map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}

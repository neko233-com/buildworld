package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestProjectChangesRouteReturnsOnlyRecordedSCMFields(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "project-changes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })

	viewer, err := data.CreateUser("changes-viewer", "changes@example.test", "unused", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject("changes-project", "", "", "git", "main", `{}`, viewer.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 17, "manual", "release", "0123456789abcdef", `{"secret":"must-not-leak"}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	builtAt := time.Date(2026, time.July, 22, 12, 4, 50, 0, time.UTC)
	if _, err := data.DB().Exec(
		"UPDATE builds SET status = 'success', started_at = ?, log = ? WHERE id = ?",
		builtAt, "secret-log-must-not-leak", build.ID,
	); err != nil {
		t.Fatal(err)
	}

	jwt := auth.NewJWT("project-changes-route-test-secret")
	token, err := jwt.Generate(viewer.ID, viewer.Role, viewer.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: data, JWT: jwt})
	request := httptest.NewRequest(http.MethodGet, "/api/projects/"+strconv.FormatInt(project.ID, 10)+"/changes", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	for _, forbidden := range []string{"must-not-leak", `"parameters"`, `"log"`, `"author"`, `"message"`} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response leaks or invents %q: %s", forbidden, response.Body.String())
		}
	}

	var payload []store.ProjectChange
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	change := payload[0]
	if change.BuildID != build.ID || change.BuildNumber != 17 || change.Status != "success" || change.CommitSHA != "0123456789abcdef" || change.Branch != "release" {
		t.Fatalf("change = %#v", change)
	}
	if change.Timestamp == nil || !change.Timestamp.Equal(builtAt) {
		t.Fatalf("timestamp = %v, want %v", change.Timestamp, builtAt)
	}

	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	assertJSONKeys(t, raw[0], []string{"build_id", "build_number", "status", "commit_sha", "branch", "timestamp"})
}

func TestProjectChangesRouteRejectsInvalidAndMissingProjects(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "project-changes-errors.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	viewer, err := data.CreateUser("changes-errors", "errors@example.test", "unused", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWT("project-changes-errors-secret")
	token, err := jwt.Generate(viewer.ID, viewer.Role, viewer.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: data, JWT: jwt})

	for _, test := range []struct {
		path string
		want int
	}{
		{path: "/api/projects/not-an-id/changes", want: http.StatusBadRequest},
		{path: "/api/projects/999999/changes", want: http.StatusNotFound},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Fatalf("%s status = %d, want %d; body=%s", test.path, response.Code, test.want, response.Body.String())
		}
	}
}

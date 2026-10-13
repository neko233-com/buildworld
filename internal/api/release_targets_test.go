package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestReleaseTargetAPIKeepsSCMHistoryAvailableWithoutReturningParameters(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "targets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("release", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"test","TOKEN":"private-token"}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.FinishBuild(build.ID, "success", 0); err != nil {
		t.Fatal(err)
	}
	handler := &handlers{d: Deps{Store: data}}
	response := httptest.NewRecorder()
	handler.listReleaseTargets(response, requestWithRouteID("GET", "/api/projects/1/release-targets", bytes.NewReader(nil), project.ID))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"last_success"`) || !strings.Contains(response.Body.String(), `"wechat"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "private-token") || strings.Contains(response.Body.String(), "parameters") || strings.Contains(response.Body.String(), `"log"`) {
		t.Fatalf("target response exposed unrelated data: %s", response.Body.String())
	}
	missing := httptest.NewRecorder()
	handler.listReleaseTargets(missing, requestWithRouteID("GET", "/api/projects/999/release-targets", bytes.NewReader(nil), 999))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing project code = %d", missing.Code)
	}
}

func TestPublishLatestAPIGuardCannotBeBypassedByTriggersOrRetry(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "guard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("guard", "", "", "git", "main", `parameters:
  - {name: RELEASE_CHANNEL, type: choice, choices: [wechat, taptap]}
  - {name: RELEASE_ENVIRONMENT, type: string, default: test}
  - {name: RELEASE_ACTION, type: choice, choices: [restart-version, publish-latest]}
  - {name: RELEASE_VERSION, type: string}
  - {name: CONFIRM_PUBLISH_LATEST, type: boolean, default: false}
jobs:
  publish:
    steps:
      - {name: Publish, run: echo fixture}
`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := &handlers{d: Deps{Store: data}}
	for _, body := range []string{`{"parameters":{"RELEASE_ACTION":"publish-latest"}}`, `{"parameters":{"RELEASE_ACTION":"restart-version"}}`, `{"parameters":{"RELEASE_ACTION":"restart-version","RELEASE_VERSION":"0.1.234","RELEASE_ENVIRONMENT":"../release"}}`} {
		response := httptest.NewRecorder()
		handler.triggerBuild(response, requestWithRouteID("POST", "/api/projects/1/builds", bytes.NewBufferString(body), project.ID))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("guard = %d %s", response.Code, response.Body.String())
		}
	}
	confirmed := httptest.NewRecorder()
	handler.triggerBuild(confirmed, requestWithRouteID("POST", "/api/projects/1/builds", bytes.NewBufferString(`{"parameters":{"RELEASE_ACTION":"publish-latest","CONFIRM_PUBLISH_LATEST":true}}`), project.ID))
	if confirmed.Code != http.StatusCreated {
		t.Fatalf("confirmed = %d %s", confirmed.Code, confirmed.Body.String())
	}
	builds, err := data.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 1 {
		t.Fatalf("created builds = %d, want 1", len(builds))
	}
	retry := httptest.NewRecorder()
	handler.retryBuild(retry, requestWithRouteID("POST", "/api/builds/1/retry", bytes.NewReader(nil), builds[0].ID))
	if retry.Code != http.StatusBadRequest {
		t.Fatalf("retry = %d %s", retry.Code, retry.Body.String())
	}
	builds, err = data.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 1 {
		t.Fatalf("retry created another build: %v %v", builds, err)
	}
}

func TestHistoryAPIRejectsUnsafeEnvironmentFilter(t *testing.T) {
	handler := &handlers{}
	response := httptest.NewRecorder()
	handler.searchBuilds(response, httptest.NewRequest("GET", "/api/builds/search?release_environment=../release", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unsafe filter response = %d %s", response.Code, response.Body.String())
	}
}

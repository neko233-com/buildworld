package api

import (
	"bytes"
	"encoding/json"
	"fmt"
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

const sharedReleasePipelineFixture = `parameters:
  - {name: RELEASE_SHARED_SERVICE, type: string, default: sf-minigame-shared}
  - {name: RELEASE_ACTION, type: choice, choices: [restart-version, publish-latest]}
  - {name: RELEASE_VERSION, type: string}
  - {name: CONFIRM_PUBLISH_LATEST, type: boolean, default: false}
jobs:
  publish:
    steps:
      - {name: Publish, run: echo fixture}
`

func TestSharedReleaseRouterLatestRestartAndRetryGuards(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "shared-router.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	user, err := data.CreateUser("shared-admin", "shared-admin@example.test", "unused", "admin")
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWT("shared-release-router-fixture")
	session, err := jwt.Generate(user.ID, user.Role, user.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: data, JWT: jwt})
	project, err := data.CreateProject("shared-SDK", "", "", "git", "main", sharedReleasePipelineFixture, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf("/api/projects/%d/builds", project.ID)
	for _, body := range []string{
		`{"parameters":{"RELEASE_ACTION":"publish-latest"}}`,
		`{"parameters":{"RELEASE_ACTION":"publish-latest","CONFIRM_PUBLISH_LATEST":"true"}}`,
		`{"parameters":{"RELEASE_ACTION":"restart-version"}}`,
		`{"parameters":{"RELEASE_VERSION":"0.0.66","RELEASE_SHARED_SERVICE":""}}`,
		`{"parameters":{"RELEASE_VERSION":"0.0.66","RELEASE_SHARED_SERVICE":null}}`,
		`{"parameters":{"RELEASE_VERSION":"0.0.66","RELEASE_SHARED_SERVICE":"../private-invalid-service-value"}}`,
		`{"parameters":{"RELEASE_VERSION":"0.0.66","RELEASE_CHANNEL":"wechat"}}`,
		`{"parameters":{"RELEASE_VERSION":"0.0.66","RELEASE_ENVIRONMENT":"test"}}`,
	} {
		response := apiTokenRequest(router, http.MethodPost, trigger, session, body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("guard=%d %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "private-invalid-service-value") {
			t.Fatal("API error revealed service value")
		}
	}
	builds, err := data.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 0 {
		t.Fatal("rejected shared requests created a build", err)
	}
	latest := apiTokenRequest(router, http.MethodPost, trigger, session, `{"parameters":{"RELEASE_ACTION":"publish-latest","CONFIRM_PUBLISH_LATEST":true}}`)
	if latest.Code != http.StatusCreated {
		t.Fatalf("confirmed shared latest=%d %s", latest.Code, latest.Body.String())
	}
	builds, _ = data.ListBuildsByProject(project.ID)
	if len(builds) != 1 {
		t.Fatal("shared latest not queued")
	}
	latestRetry := apiTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/builds/%d/retry", builds[0].ID), session, `{"parameters":{"CONFIRM_PUBLISH_LATEST":true}}`)
	if latestRetry.Code != http.StatusBadRequest {
		t.Fatalf("shared latest replay=%d %s", latestRetry.Code, latestRetry.Body.String())
	}
	restart := apiTokenRequest(router, http.MethodPost, trigger, session, `{"parameters":{"RELEASE_ACTION":"restart-version","RELEASE_VERSION":"0.0.66"}}`)
	if restart.Code != http.StatusCreated {
		t.Fatalf("shared restart=%d %s", restart.Code, restart.Body.String())
	}
	builds, _ = data.ListBuildsByProject(project.ID)
	if len(builds) != 2 {
		t.Fatal("unexpected build count after shared requests")
	}
	var restartBuild *store.Build
	for _, build := range builds {
		var p map[string]interface{}
		_ = json.Unmarshal([]byte(build.Parameters), &p)
		if p["RELEASE_ACTION"] == "restart-version" {
			restartBuild = build
		}
	}
	if restartBuild == nil {
		t.Fatal("shared restart parameters missing")
	}
	restartRetry := apiTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/builds/%d/retry", restartBuild.ID), session, "")
	if restartRetry.Code != http.StatusCreated {
		t.Fatalf("fixed shared restart replay=%d %s", restartRetry.Code, restartRetry.Body.String())
	}
	builds, _ = data.ListBuildsByProject(project.ID)
	if len(builds) != 3 {
		t.Fatal("fixed restart did not retain a separate retry build")
	}
	for _, build := range builds {
		var p map[string]interface{}
		if json.Unmarshal([]byte(build.Parameters), &p) != nil || p["RELEASE_SHARED_SERVICE"] != "sf-minigame-shared" {
			t.Fatal("shared identity not persisted")
		}
		if _, ok := p["RELEASE_CHANNEL"]; ok {
			t.Fatal("shared build classified as channel release")
		}
		if _, ok := p["RELEASE_ENVIRONMENT"]; ok {
			t.Fatal("shared build classified as environment release")
		}
		if p["RELEASE_ACTION"] == "restart-version" && p["RELEASE_VERSION"] != "0.0.66" {
			t.Fatal("retry changed immutable shared version")
		}
	}
}

func TestSharedReleaseRouterRejectsSecretAndMixedDeclarations(t *testing.T) {
	for _, name := range []string{"secret-service", "password-service", "mixed-channel", "mixed-environment"} {
		t.Run(name, func(t *testing.T) {
			data, err := store.New(filepath.Join(t.TempDir(), "invalid-shared-router.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer data.Close()
			pipeline := sharedReleasePipelineFixture
			switch name {
			case "secret-service":
				pipeline = strings.Replace(pipeline, "type: string, default: sf-minigame-shared", "type: string, default: sf-minigame-shared, is_secret: true", 1)
			case "password-service":
				pipeline = strings.Replace(pipeline, "type: string, default: sf-minigame-shared", "type: password, default: sf-minigame-shared", 1)
			case "mixed-channel":
				pipeline = strings.Replace(pipeline, "jobs:", "  - {name: RELEASE_CHANNEL, type: choice, choices: [wechat, taptap]}\njobs:", 1)
			case "mixed-environment":
				pipeline = strings.Replace(pipeline, "jobs:", "  - {name: RELEASE_ENVIRONMENT, type: string}\njobs:", 1)
			}
			project, err := data.CreateProject("invalid-shared", "", "", "git", "main", pipeline, 0, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			user, err := data.CreateUser("guard-admin", "guard-admin@example.test", "unused", "admin")
			if err != nil {
				t.Fatal(err)
			}
			jwt := auth.NewJWT("invalid-shared-router-fixture")
			session, err := jwt.Generate(user.ID, user.Role, user.SessionVersion, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			router := NewRouter(Deps{Cfg: &config.Config{}, Store: data, JWT: jwt})
			response := apiTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/projects/%d/builds", project.ID), session, `{"parameters":{"RELEASE_ACTION":"publish-latest","CONFIRM_PUBLISH_LATEST":true}}`)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid schema accepted=%d %s", response.Code, response.Body.String())
			}
			builds, err := data.ListBuildsByProject(project.ID)
			if err != nil || len(builds) != 0 {
				t.Fatal("invalid definition queued a build")
			}
		})
	}
}

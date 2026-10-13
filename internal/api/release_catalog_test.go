package api

import (
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
	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

const releaseCatalogPipelineFixture = `parameters:
  - {name: RELEASE_CHANNEL, type: choice, choices: [obsolete], default: obsolete}
  - {name: RELEASE_ENVIRONMENT, type: string, default: release}
  - {name: RELEASE_ACTION, type: choice, choices: [restart-version, publish-latest]}
  - {name: RELEASE_VERSION, type: string}
  - {name: CONFIRM_PUBLISH_LATEST, type: boolean, default: false}
jobs:
  publish:
    steps:
      - {name: Publish, run: echo fixture}
`
const releaseCatalogAPIFixture = `{"schemaVersion":1,"targets":[{"channel":"alpha","environment":"test","channelLabel":"第一渠道"},{"channel":"alpha","environment":"release","channelLabel":"第一渠道"},{"channel":"third-store","environment":"audit-2","channelLabel":"第三渠道"}]}`

func TestConfiguredReleaseCatalogRouterUsesMatrixAcrossTriggerRetryAndMetadata(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "configured-release.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	user, err := data.CreateUser("catalog-admin", "catalog@example.test", "unused", "admin")
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWT("configured-release-fixture")
	session, err := jwt.Generate(user.ID, user.Role, user.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: data, JWT: jwt})
	project, err := data.CreateProject("configured-release", "", "", "git", "main", releaseCatalogPipelineFixture, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetEnvVar("project", &project.ID, "PRIVATE_TOKEN", "private-sentinel", true, "fixture"); err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf("/api/projects/%d/builds", project.ID)
	validBody := `{"parameters":{"RELEASE_CHANNEL":"third-store","RELEASE_ENVIRONMENT":"audit-2","RELEASE_VERSION":"0.1.21"}}`
	missing := apiTokenRequest(router, http.MethodPost, trigger, session, validBody)
	if missing.Code != http.StatusBadRequest {
		t.Fatal("missing public matrix did not fail closed")
	}
	if err := data.SetEnvVar("project", &project.ID, engine.ReleaseTargetsVariable, releaseCatalogAPIFixture, false, "fixture"); err != nil {
		t.Fatal(err)
	}
	metadata := apiTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/projects/%d/validate", project.ID), session, "")
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"release_targets_required":true`) || !strings.Contains(metadata.Body.String(), `"third-store"`) || strings.Contains(metadata.Body.String(), "obsolete") || strings.Contains(metadata.Body.String(), "private-sentinel") {
		t.Fatal("matrix metadata did not replace stale candidates or leaked unrelated variables")
	}
	for _, body := range []string{
		`{"parameters":{"RELEASE_CHANNEL":"third-store","RELEASE_ENVIRONMENT":"test","RELEASE_VERSION":"0.1.21"}}`,
		`{"parameters":{"RELEASE_CHANNEL":"alpha","RELEASE_ENVIRONMENT":"audit-2","RELEASE_VERSION":"0.1.21"}}`,
		`{"parameters":{"RELEASE_ACTION":"publish-latest"}}`,
		`{"parameters":{"RELEASE_ACTION":"publish-latest","CONFIRM_PUBLISH_LATEST":"true"}}`,
		`{"parameters":{"RELEASE_ACTION":"restart-version"}}`,
	} {
		if response := apiTokenRequest(router, http.MethodPost, trigger, session, body); response.Code != http.StatusBadRequest {
			t.Fatal("matrix/action guard was bypassed")
		}
	}
	created := apiTokenRequest(router, http.MethodPost, trigger, session, validBody)
	if created.Code != http.StatusCreated {
		t.Fatal("configured third channel was rejected")
	}
	var build store.Build
	if json.Unmarshal(created.Body.Bytes(), &build) != nil {
		t.Fatal("created build missing")
	}
	retryPath := fmt.Sprintf("/api/builds/%d/retry", build.ID)
	if response := apiTokenRequest(router, http.MethodPost, retryPath, session, `{}`); response.Code != http.StatusCreated {
		t.Fatal("configured fixed-version retry was rejected")
	}
	if err := data.SetProjectHTTPTrigger(project.ID, true, "fixture-http-token"); err != nil {
		t.Fatal(err)
	}
	httpPath := fmt.Sprintf("/api/trigger/projects/%d/fixture-http-token", project.ID)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, httpPath, strings.NewReader(validBody)))
	if response.Code != http.StatusCreated {
		t.Fatal("project HTTP trigger did not use configured matrix")
	}
	builds, err := data.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 3 {
		t.Fatal("invalid requests created a build")
	}
	if err := data.SetEnvVar("project", &project.ID, engine.ReleaseTargetsVariable, `{"schemaVersion":1,"targets":[{"channel":"alpha","environment":"test"}]}`, false, "fixture"); err != nil {
		t.Fatal(err)
	}
	if response := apiTokenRequest(router, http.MethodPost, retryPath, session, `{}`); response.Code != http.StatusBadRequest {
		t.Fatal("removed historical target was retried")
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, httpPath, strings.NewReader(validBody)))
	if response.Code != http.StatusBadRequest {
		t.Fatal("HTTP trigger accepted removed target")
	}
	builds, err = data.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 3 {
		t.Fatal("removed-target request wrote a build")
	}
}

func TestConfiguredReleaseCatalogRejectsSecretOrMalformedProjectMetadata(t *testing.T) {
	for _, fault := range []string{"secret", "malformed", "global-only"} {
		t.Run(fault, func(t *testing.T) {
			data, err := store.New(filepath.Join(t.TempDir(), "invalid-catalog.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer data.Close()
			project, err := data.CreateProject("invalid", "", "", "git", "main", releaseCatalogPipelineFixture, 0, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			value := releaseCatalogAPIFixture
			if fault == "malformed" {
				value = `{"schemaVersion":1,"targets":[{"channel":"private-sentinel/invalid","environment":"test"}]}`
			}
			scope, projectID := "project", &project.ID
			if fault == "global-only" {
				scope, projectID = "global", nil
			}
			if err := data.SetEnvVar(scope, projectID, engine.ReleaseTargetsVariable, value, fault == "secret", "fixture"); err != nil {
				t.Fatal(err)
			}
			handler := &handlers{d: Deps{Store: data}}
			if _, err := handler.resolveProjectBuildParameters(project, map[string]interface{}{"RELEASE_VERSION": "0.1.21"}); err == nil || strings.Contains(err.Error(), "private-sentinel") {
				t.Fatal("bad declaration was accepted or raw value disclosed")
			}
		})
	}
}

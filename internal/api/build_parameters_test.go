package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/neko233-com/buildworld/internal/store"
)

func requestWithRouteID(method, path string, body io.Reader, id int64) *http.Request {
	request := httptest.NewRequest(method, path, body)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", strconv.FormatInt(id, 10))
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}

func TestTriggerBuildValidatesDefaultsAndRedactsSecrets(t *testing.T) {
	dataStore, err := store.New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dataStore.Close()

	project, err := dataStore.CreateProject(
		"parameter-api",
		"",
		"",
		"git",
		"main",
		`parameters:
  - {name: target, type: choice, choices: [staging, production], default: staging, required: true}
  - {name: release, type: boolean, default: false}
  - {name: api_token, type: password, required: true, is_secret: true}
jobs:
  build:
    name: Build
    steps:
      - {name: Package, run: echo package}
`,
		0,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := &handlers{d: Deps{Store: dataStore}}

	missingSecret := httptest.NewRecorder()
	handler.triggerBuild(
		missingSecret,
		requestWithRouteID("POST", "/api/projects/1/builds", bytes.NewBufferString(`{"parameters":{"target":"production"}}`), project.ID),
	)
	if missingSecret.Code != http.StatusBadRequest || !strings.Contains(missingSecret.Body.String(), "api_token") {
		t.Fatalf("missing secret response = %d %s, want parameter validation error", missingSecret.Code, missingSecret.Body.String())
	}
	builds, err := dataStore.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 0 {
		t.Fatalf("invalid request created %d builds, want 0", len(builds))
	}

	const secret = "browser-secret-qa"
	valid := httptest.NewRecorder()
	handler.triggerBuild(
		valid,
		requestWithRouteID("POST", "/api/projects/1/builds", bytes.NewBufferString(`{
			"branch":"release/browser-qa",
			"parameters":{"target":"production","api_token":"`+secret+`"}
		}`), project.ID),
	)
	if valid.Code != http.StatusCreated {
		t.Fatalf("valid response = %d %s, want 201", valid.Code, valid.Body.String())
	}
	if strings.Contains(valid.Body.String(), secret) || !strings.Contains(valid.Body.String(), "********") {
		t.Fatalf("trigger response did not redact secret: %s", valid.Body.String())
	}

	builds, err = dataStore.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 1 {
		t.Fatalf("created builds = %d, want 1", len(builds))
	}
	if builds[0].Branch != "release/browser-qa" {
		t.Fatalf("branch = %q, want release/browser-qa", builds[0].Branch)
	}
	var storedParameters map[string]interface{}
	if err := json.Unmarshal([]byte(builds[0].Parameters), &storedParameters); err != nil {
		t.Fatal(err)
	}
	if storedParameters["api_token"] != secret {
		t.Fatalf("stored secret = %#v, runner must receive original value", storedParameters["api_token"])
	}
	if release, ok := storedParameters["release"].(bool); !ok || release {
		t.Fatalf("release default = %#v, want false", storedParameters["release"])
	}

	publicBuild := httptest.NewRecorder()
	handler.getBuild(
		publicBuild,
		requestWithRouteID("GET", "/api/builds/1", bytes.NewReader(nil), builds[0].ID),
	)
	if publicBuild.Code != http.StatusOK || strings.Contains(publicBuild.Body.String(), secret) {
		t.Fatalf("get build response exposed secret: %d %s", publicBuild.Code, publicBuild.Body.String())
	}
}

func TestTriggerBuildRejectsMalformedBodyAndInvalidChoice(t *testing.T) {
	dataStore, err := store.New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dataStore.Close()

	project, err := dataStore.CreateProject(
		"parameter-errors",
		"",
		"",
		"git",
		"main",
		`parameters:
  - {name: target, type: choice, choices: [staging, production], required: true}
jobs:
  build:
    steps:
      - {name: Package, run: echo package}
`,
		0,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := &handlers{d: Deps{Store: dataStore}}

	for name, body := range map[string]string{
		"malformed JSON": `{"parameters":`,
		"invalid choice": `{"parameters":{"target":"development"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.triggerBuild(
				response,
				requestWithRouteID("POST", "/api/projects/1/builds", bytes.NewBufferString(body), project.ID),
			)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("response = %d %s, want 400", response.Code, response.Body.String())
			}
		})
	}
}

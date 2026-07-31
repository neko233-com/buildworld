package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestProjectHTTPTriggerGeneratesCredentialAndStartsBuild(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "project-http-trigger.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	handler := &handlers{d: Deps{Store: database}}

	created := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "http://buildworld.test/api/projects", strings.NewReader(`{
		"name":"unity-game","repo_type":"git","default_branch":"release",
		"config":"jobs:\n  build:\n    steps:\n      - run: echo ok",
		"http_trigger_enabled":true
	}`)).WithContext(context.WithValue(context.Background(), auth.CtxRole, "admin"))
	handler.createProject(created, createRequest)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var project store.Project
	if err := json.Unmarshal(created.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	if !project.HTTPTriggerEnabled || project.HTTPTriggerURL == "" || strings.Contains(created.Body.String(), "http_trigger_token") {
		t.Fatalf("unsafe or incomplete create response: %s", created.Body.String())
	}
	persisted, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.HTTPTriggerToken == "" {
		t.Fatal("external HTTP trigger credential was not stored")
	}

	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: auth.NewJWT("project-http-trigger-test")})
	path := "/api/trigger/projects/" + strconv.FormatInt(project.ID, 10) + "/" + persisted.HTTPTriggerToken
	wrong := httptest.NewRecorder()
	router.ServeHTTP(wrong, httptest.NewRequest(http.MethodPost, path+"wrong", bytes.NewBufferString(`{}`)))
	if wrong.Code != http.StatusNotFound {
		t.Fatalf("wrong credential status = %d, want %d; body=%s", wrong.Code, http.StatusNotFound, wrong.Body.String())
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("trigger = %d %s", response.Code, response.Body.String())
	}
	builds, err := database.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 1 || builds[0].Trigger != "http" || builds[0].Branch != "release" {
		t.Fatalf("builds = %#v, want HTTP build on release", builds)
	}
}

func TestProjectHTTPTriggerURLIsHiddenFromViewer(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "project-http-trigger-viewer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject("private-http-trigger", "", "", "git", "main", "jobs:\n  build:\n    steps: []\n", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectHTTPTrigger(project.ID, true, "viewer-must-not-see-this"); err != nil {
		t.Fatal(err)
	}
	handler := &handlers{d: Deps{Store: database}}
	viewerRequest := requestWithRouteID(http.MethodGet, "http://buildworld.test/api/projects/1", nil, project.ID)
	viewerRequest = viewerRequest.WithContext(context.WithValue(viewerRequest.Context(), auth.CtxRole, "viewer"))
	viewerResponse := httptest.NewRecorder()
	handler.getProject(viewerResponse, viewerRequest)
	if viewerResponse.Code != http.StatusOK || strings.Contains(viewerResponse.Body.String(), "http_trigger_url") || strings.Contains(viewerResponse.Body.String(), "viewer-must-not-see-this") {
		t.Fatalf("viewer response exposed trigger: %d %s", viewerResponse.Code, viewerResponse.Body.String())
	}

	adminRequest := requestWithRouteID(http.MethodGet, "http://buildworld.test/api/projects/1", nil, project.ID)
	adminRequest = adminRequest.WithContext(context.WithValue(adminRequest.Context(), auth.CtxRole, "admin"))
	adminResponse := httptest.NewRecorder()
	handler.getProject(adminResponse, adminRequest)
	if adminResponse.Code != http.StatusOK || !strings.Contains(adminResponse.Body.String(), "http_trigger_url") || strings.Contains(adminResponse.Body.String(), "http_trigger_token") {
		t.Fatalf("admin response missing generated URL: %d %s", adminResponse.Code, adminResponse.Body.String())
	}
}

func TestProjectHTTPTriggerDisableRevokesCopiedURL(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "project-http-trigger-revoke.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject("revoke-http", "", "", "git", "main", "jobs:\n  build:\n    steps: []\n", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectHTTPTrigger(project.ID, true, "known-secret"); err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectHTTPTrigger(project.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: auth.NewJWT("project-http-trigger-revoke-test")})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/trigger/projects/"+strconv.FormatInt(project.ID, 10)+"/known-secret", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("revoked credential status = %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
	}
}

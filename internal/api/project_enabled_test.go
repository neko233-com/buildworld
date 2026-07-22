package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestProjectEnabledCreateUpdateAndDisabledTrigger(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "project-enabled-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	handler := &handlers{d: Deps{Store: data}}

	createdResponse := httptest.NewRecorder()
	handler.createProject(createdResponse, httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewBufferString(`{
		"name":"disabled-job","repo_type":"git","default_branch":"main","config":"jobs:\n  build:\n    steps:\n      - run: echo ok","enabled":false
	}`)))
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created store.Project
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Enabled {
		t.Fatal("created project enabled = true, want false")
	}

	triggerResponse := httptest.NewRecorder()
	handler.triggerBuild(triggerResponse, requestWithRouteID(http.MethodPost, "/api/projects/1/builds", strings.NewReader(`{}`), created.ID))
	if triggerResponse.Code != http.StatusConflict || !strings.Contains(triggerResponse.Body.String(), `"code":"project_disabled"`) {
		t.Fatalf("disabled trigger = %d %s", triggerResponse.Code, triggerResponse.Body.String())
	}
	builds, err := data.ListBuildsByProject(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 0 {
		t.Fatalf("disabled trigger created %d builds", len(builds))
	}

	updatedResponse := httptest.NewRecorder()
	handler.updateProject(updatedResponse, requestWithRouteID(http.MethodPut, "/api/projects/1", bytes.NewBufferString(`{
		"name":"disabled-job","repo_type":"git","default_branch":"main","config":"jobs:\n  build:\n    steps:\n      - run: echo ok","enabled":true
	}`), created.ID))
	if updatedResponse.Code != http.StatusOK || !strings.Contains(updatedResponse.Body.String(), `"enabled":true`) {
		t.Fatalf("update = %d %s", updatedResponse.Code, updatedResponse.Body.String())
	}
}

func TestProjectEnabledDefaultsTrueWhenCreateFieldOmitted(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "project-enabled-default-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	handler := &handlers{d: Deps{Store: data}}
	response := httptest.NewRecorder()
	handler.createProject(response, httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewBufferString(`{
		"name":"default-job","repo_type":"git","default_branch":"main","config":"jobs:\n  build:\n    steps:\n      - run: echo ok"
	}`)))
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"enabled":true`) {
		t.Fatalf("create default = %d %s", response.Code, response.Body.String())
	}
}

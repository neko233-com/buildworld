package api

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/neko233-com/buildworld/internal/store"
)

const validTemplateProjectBody = `{
  "name": "template-project",
  "repo_type": "git",
  "default_branch": "main",
  "config": "jobs:\n  build:\n    steps:\n      - run: echo ok\n",
  "template_id": 999
}`

func TestProjectWritesRejectUnknownBuildTemplate(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "unknown-template.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	handler := &handlers{d: Deps{Store: database}}

	createResponse := httptest.NewRecorder()
	handler.createProject(createResponse, httptest.NewRequest(http.MethodPost, "/api/projects/", bytes.NewBufferString(validTemplateProjectBody)))
	if createResponse.Code != http.StatusBadRequest || !strings.Contains(createResponse.Body.String(), "build template not found") {
		t.Fatalf("create with unknown template = %d %s", createResponse.Code, createResponse.Body.String())
	}
	if _, err := database.GetProjectByName("template-project"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("invalid project persisted: %v", err)
	}

	project, err := database.CreateProject("template-project", "", "", "git", "main", "jobs:\n  build:\n    steps:\n      - run: echo ok\n", 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/projects/"+strconv.FormatInt(project.ID, 10), bytes.NewBufferString(validTemplateProjectBody))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", strconv.FormatInt(project.ID, 10))
	updateRequest = updateRequest.WithContext(context.WithValue(updateRequest.Context(), chi.RouteCtxKey, routeContext))
	updateResponse := httptest.NewRecorder()
	handler.updateProject(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusBadRequest || !strings.Contains(updateResponse.Body.String(), "build template not found") {
		t.Fatalf("update with unknown template = %d %s", updateResponse.Code, updateResponse.Body.String())
	}
	reloaded, err := database.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.TemplateID != nil {
		t.Fatalf("unknown template was linked: %#v", reloaded.TemplateID)
	}
}

func TestDeleteBuildTemplateReturnsConflictWhileReferenced(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "template-in-use.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	template, err := database.CreateBuildTemplate("shared", "jobs:\n  build:\n    steps: []\n", "legacy compatibility")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateProject("linked", "", "", "git", "main", "jobs:\n  build:\n    steps: []\n", 1, nil, &template.ID); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodDelete, "/api/templates/"+strconv.FormatInt(template.ID, 10), nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", strconv.FormatInt(template.ID, 10))
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	response := httptest.NewRecorder()
	(&handlers{d: Deps{Store: database}}).deleteTemplate(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"build_template_in_use"`) {
		t.Fatalf("delete referenced template = %d %s", response.Code, response.Body.String())
	}
	if _, err := database.GetBuildTemplate(template.ID); err != nil {
		t.Fatalf("referenced template was deleted: %v", err)
	}
}

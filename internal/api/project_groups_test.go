package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestCreateProjectGroupReturnsStableConflictCode(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "project-groups.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	if _, err := data.CreateProjectGroup("games", ""); err != nil {
		t.Fatal(err)
	}
	handler := &handlers{d: Deps{Store: data}}
	request := httptest.NewRequest(http.MethodPost, "/api/project-groups/", bytes.NewBufferString(`{"name":"games"}`))
	response := httptest.NewRecorder()

	handler.createProjectGroup(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "project_group_name_exists" || payload["error"] != store.ErrProjectGroupNameExists.Error() {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestProjectGroupColorAPIValidatesPersistsAndLists(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "project-group-colors.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	handler := &handlers{d: Deps{Store: data}}

	createRequest := httptest.NewRequest(http.MethodPost, "/api/project-groups/", bytes.NewBufferString(`{"name":"game servers","color":"cyan"}`))
	createResponse := httptest.NewRecorder()
	handler.createProjectGroup(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	var created store.ProjectGroup
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Color != "cyan" {
		t.Fatalf("created color = %q, want cyan", created.Color)
	}

	listResponse := httptest.NewRecorder()
	handler.listProjectGroups(listResponse, httptest.NewRequest(http.MethodGet, "/api/project-groups/", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}
	var listed []store.ProjectGroup
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Color != "cyan" {
		t.Fatalf("listed groups = %#v", listed)
	}

	updateRequest := projectGroupRequestWithID(t, http.MethodPut, created.ID, `{"name":"game servers","description":"keeps omitted color"}`)
	updateResponse := httptest.NewRecorder()
	handler.updateProjectGroup(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateResponse.Code, updateResponse.Body.String())
	}
	var updated store.ProjectGroup
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Color != "cyan" {
		t.Fatalf("color after omitted update = %q, want cyan", updated.Color)
	}

	updateRequest = projectGroupRequestWithID(t, http.MethodPut, created.ID, `{"name":"game servers","color":"mint"}`)
	updateResponse = httptest.NewRecorder()
	handler.updateProjectGroup(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("color update status = %d, body = %s", updateResponse.Code, updateResponse.Body.String())
	}
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Color != "mint" {
		t.Fatalf("updated color = %q, want mint", updated.Color)
	}
}

func TestProjectGroupAPIRejectsInvalidColors(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "project-group-validation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	handler := &handlers{d: Deps{Store: data}}
	existing, err := data.CreateProjectGroupWithColor("servers", "", "blue")
	if err != nil {
		t.Fatal(err)
	}

	for _, body := range []string{
		`{"name":"empty color","color":""}`,
		`{"name":"case color","color":"BLUE"}`,
		`{"name":"raw color","color":"#007aff"}`,
	} {
		response := httptest.NewRecorder()
		handler.createProjectGroup(response, httptest.NewRequest(http.MethodPost, "/api/project-groups/", bytes.NewBufferString(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("create body %s status = %d, response = %s", body, response.Code, response.Body.String())
		}
	}

	for _, body := range []string{
		`{"name":"servers","color":"red"}`,
	} {
		response := httptest.NewRecorder()
		handler.updateProjectGroup(response, projectGroupRequestWithID(t, http.MethodPut, existing.ID, body))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("update body %s status = %d, response = %s", body, response.Code, response.Body.String())
		}
	}

	unchanged, err := data.GetProjectGroup(existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Color != "blue" {
		t.Fatalf("rejected updates changed group: %#v", unchanged)
	}
}

func projectGroupRequestWithID(t *testing.T, method string, id int64, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, "/api/project-groups/"+strconv.FormatInt(id, 10), bytes.NewBufferString(body))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", strconv.FormatInt(id, 10))
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}

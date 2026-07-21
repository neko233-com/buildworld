package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/neko233-com/buildworld/internal/store"
)

const validRepositoryTypeTestPipeline = `jobs:
  build:
    name: Build
    steps:
      - name: Compile
        run: echo ok
`

func requestWithID(method, target, id, body string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", id)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}

func TestRepositoryWritesRejectNonGitTypes(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*httptest.ResponseRecorder)
	}{
		{
			name: "create project",
			invoke: func(response *httptest.ResponseRecorder) {
				body := `{"name":"svn-project","repo_type":"svn","config":` + jsonQuote(validRepositoryTypeTestPipeline) + `}`
				(&handlers{}).createProject(response, httptest.NewRequest(http.MethodPost, "/api/projects/", bytes.NewBufferString(body)))
			},
		},
		{
			name: "update project",
			invoke: func(response *httptest.ResponseRecorder) {
				body := `{"name":"hg-project","repo_type":"hg","config":` + jsonQuote(validRepositoryTypeTestPipeline) + `}`
				(&handlers{}).updateProject(response, requestWithID(http.MethodPut, "/api/projects/9", "9", body))
			},
		},
		{
			name: "create VCS repository template",
			invoke: func(response *httptest.ResponseRecorder) {
				(&handlers{}).createVCSRoot(response, httptest.NewRequest(http.MethodPost, "/api/vcs-roots/", strings.NewReader(`{"name":"svn","type":"svn"}`)))
			},
		},
		{
			name: "update VCS repository template",
			invoke: func(response *httptest.ResponseRecorder) {
				(&handlers{}).updateVCSRoot(response, requestWithID(http.MethodPut, "/api/vcs-roots/9", "9", `{"name":"hg","type":"hg"}`))
			},
		},
		{
			name: "create unsupported credential",
			invoke: func(response *httptest.ResponseRecorder) {
				(&handlers{}).createCredential(response, httptest.NewRequest(http.MethodPost, "/api/credentials/", strings.NewReader(`{"name":"svn","type":"svn"}`)))
			},
		},
		{
			name: "update unsupported credential",
			invoke: func(response *httptest.ResponseRecorder) {
				(&handlers{}).updateCredential(response, requestWithID(http.MethodPut, "/api/credentials/9", "9", `{"name":"hg","type":"hg"}`))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			test.invoke(response)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, body = %s, want 422", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "only git is supported") && !strings.Contains(response.Body.String(), "use git or ssh_key") {
				t.Fatalf("body = %s, want a clear Git-only error", response.Body.String())
			}
		})
	}
}

func TestRepositoryWritesDefaultEmptyTypesToGit(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "git-defaults.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	handler := &handlers{d: Deps{Store: database}}

	projectBody := `{"name":"default-project","repo_url":"https://example.test/repository.git","default_branch":"main","config":` + jsonQuote(validRepositoryTypeTestPipeline) + `}`
	projectResponse := httptest.NewRecorder()
	handler.createProject(projectResponse, httptest.NewRequest(http.MethodPost, "/api/projects/", strings.NewReader(projectBody)))
	if projectResponse.Code != http.StatusCreated {
		t.Fatalf("create project = %d %s", projectResponse.Code, projectResponse.Body.String())
	}
	var project store.Project
	if err := json.NewDecoder(projectResponse.Body).Decode(&project); err != nil {
		t.Fatal(err)
	}
	if project.RepoType != store.RepositoryTypeGit {
		t.Fatalf("project repo type = %q, want git", project.RepoType)
	}

	rootResponse := httptest.NewRecorder()
	handler.createVCSRoot(rootResponse, httptest.NewRequest(http.MethodPost, "/api/vcs-roots/", strings.NewReader(`{"name":"default-root","url":"https://example.test/repository.git","branch":"main"}`)))
	if rootResponse.Code != http.StatusCreated {
		t.Fatalf("create repository template = %d %s", rootResponse.Code, rootResponse.Body.String())
	}
	var root store.VCSRoot
	if err := json.NewDecoder(rootResponse.Body).Decode(&root); err != nil {
		t.Fatal(err)
	}
	if root.Type != store.RepositoryTypeGit {
		t.Fatalf("repository template type = %q, want git", root.Type)
	}
}

func jsonQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

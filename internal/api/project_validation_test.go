package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestValidateProjectConfigParsesTypeScriptWithoutStartingBuild(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "project-validation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	_, err = database.CreateProject("typed", "", "", "git", "main", `import { definePipeline, shell, stage } from '@buildworld/pipeline'
export default definePipeline({ stages: [stage('Build', shell('Compile', 'echo ok'))] })`, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := &handlers{d: Deps{Store: database}}
	request := httptest.NewRequest(http.MethodPost, "/api/projects/1/validate", nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", "1")
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	response := httptest.NewRecorder()
	handler.validateProjectConfig(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"format":"typescript"`) || !strings.Contains(response.Body.String(), `"steps":1`) {
		t.Fatalf("validation = %d %s", response.Code, response.Body.String())
	}
}

func TestValidatePipelineSourceParsesUnsavedDraft(t *testing.T) {
	handler := &handlers{}
	request := httptest.NewRequest(http.MethodPost, "/api/pipeline-validation", bytes.NewBufferString(`{
		"source": "import { definePipeline, shell, stage } from '@buildworld/pipeline'\nexport default definePipeline({ stages: [stage('Build', shell('Compile', 'echo ok'))] })"
	}`))
	response := httptest.NewRecorder()

	handler.validatePipelineSource(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"format":"typescript"`) || !strings.Contains(response.Body.String(), `"steps":1`) {
		t.Fatalf("draft validation = %d %s", response.Code, response.Body.String())
	}
}

func TestValidatePipelineSourceRejectsInvalidDraft(t *testing.T) {
	handler := &handlers{}
	request := httptest.NewRequest(http.MethodPost, "/api/pipeline-validation", bytes.NewBufferString(`{"source":"import { definePipeline } from '@buildworld/pipeline'\nconst pipeline = {}"}`))
	response := httptest.NewRecorder()

	handler.validatePipelineSource(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("draft validation = %d %s, want 422", response.Code, response.Body.String())
	}
}

func TestValidatePipelineSourceRejectsUnboundTypeScriptHelper(t *testing.T) {
	handler := &handlers{}
	source := `import { definePipeline, stage } from "@buildworld/pipeline"
export default definePipeline({ stages: [stage("Build", shell("run", "echo ok"))] })`
	request := httptest.NewRequest(http.MethodPost, "/api/pipeline-validation", bytes.NewBufferString(`{"source":`+strconv.Quote(source)+`}`))
	response := httptest.NewRecorder()

	handler.validatePipelineSource(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `helper \"shell\" must be imported as a value`) {
		t.Fatalf("draft validation = %d %s, want unbound helper error", response.Code, response.Body.String())
	}
}

func TestValidatePipelineSourceRejectsInvalidServiceWatchOptions(t *testing.T) {
	handler := &handlers{}
	source := `jobs:
  observe:
    steps:
      - name: Logs
        type: service_watch
        with:
          pid_file: server.pid
          log_file: server.log
          unknown: value
`
	request := httptest.NewRequest(http.MethodPost, "/api/pipeline-validation", bytes.NewBufferString(`{"source":`+strconv.Quote(source)+`}`))
	response := httptest.NewRecorder()

	handler.validatePipelineSource(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "not supported") {
		t.Fatalf("draft validation = %d %s, want service_watch option error", response.Code, response.Body.String())
	}
}

func TestValidatePipelineSourceRejectsInvalidDependencyGraphs(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "yaml unknown dependency",
			source: `jobs:
  deploy:
    needs: missing
    steps: [{run: echo deploy}]
`,
			want: "unknown job",
		},
		{
			name: "typescript cycle",
			source: `import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({ stages: [
  stage("First", shell("first", "echo first"), { dependsOn: ["Second"] }),
  stage("Second", shell("second", "echo second"), { dependsOn: ["First"] }),
] })`,
			want: "dependency cycle",
		},
		{
			name: "yaml duplicate stage name",
			source: `jobs:
  first:
    name: Build
    steps: [{run: echo first}]
  second:
    name: Build
    steps: [{run: echo second}]
`,
			want: "declared more than once",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &handlers{}
			body := `{"source":` + strconv.Quote(test.source) + `}`
			request := httptest.NewRequest(http.MethodPost, "/api/pipeline-validation", bytes.NewBufferString(body))
			response := httptest.NewRecorder()

			handler.validatePipelineSource(response, request)

			if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), test.want) {
				t.Fatalf("draft validation = %d %s, want 422 containing %q", response.Code, response.Body.String(), test.want)
			}
		})
	}
}

func TestValidatePipelineSourceRejectsRemovedFormats(t *testing.T) {
	tests := map[string]string{
		"json":        `{"stages":[]}`,
		"markdown":    "# Build\n\n## Pipeline\n\n### Compile\n```default shell\necho ok\n```",
		"stages yaml": "stages:\n  - name: Build\n    steps:\n      - run: echo ok\n",
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			body := `{"source":` + strconv.Quote(source) + `}`
			request := httptest.NewRequest(http.MethodPost, "/api/pipeline-validation", bytes.NewBufferString(body))
			response := httptest.NewRecorder()
			(&handlers{}).validatePipelineSource(response, request)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("validation = %d %s, want 422", response.Code, response.Body.String())
			}
		})
	}
}

func TestProjectWritesRejectInvalidPipelineBeforePersistence(t *testing.T) {
	handler := &handlers{}
	body := `{"name":"unsafe","repo_type":"git","default_branch":"main","config":"not: [valid"}`

	createResponse := httptest.NewRecorder()
	handler.createProject(createResponse, httptest.NewRequest(http.MethodPost, "/api/projects/", bytes.NewBufferString(body)))
	if createResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create invalid pipeline = %d %s, want 422", createResponse.Code, createResponse.Body.String())
	}

	updateRequest := httptest.NewRequest(http.MethodPut, "/api/projects/42", bytes.NewBufferString(body))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", "42")
	updateRequest = updateRequest.WithContext(context.WithValue(updateRequest.Context(), chi.RouteCtxKey, routeContext))
	updateResponse := httptest.NewRecorder()
	handler.updateProject(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("update invalid pipeline = %d %s, want 422", updateResponse.Code, updateResponse.Body.String())
	}
}

func TestTemplateWritesRejectInvalidPipelineBeforePersistence(t *testing.T) {
	handler := &handlers{}
	body := `{"name":"unsafe-template","config":"not: [valid"}`

	createResponse := httptest.NewRecorder()
	handler.createTemplate(createResponse, httptest.NewRequest(http.MethodPost, "/api/templates/", bytes.NewBufferString(body)))
	if createResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("create invalid template = %d %s, want 422", createResponse.Code, createResponse.Body.String())
	}

	updateRequest := httptest.NewRequest(http.MethodPut, "/api/templates/42", bytes.NewBufferString(body))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", "42")
	updateRequest = updateRequest.WithContext(context.WithValue(updateRequest.Context(), chi.RouteCtxKey, routeContext))
	updateResponse := httptest.NewRecorder()
	handler.updateTemplate(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("update invalid template = %d %s, want 422", updateResponse.Code, updateResponse.Body.String())
	}
}

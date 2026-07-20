package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

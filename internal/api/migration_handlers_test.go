package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/neko233-com/buildworld/internal/migration"
)

func TestMigratePipelineEndpointReturnsVersionedNativeConfig(t *testing.T) {
	handler := &handlers{migrations: migration.NewDefaultRegistry()}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/pipeline-migrations/jenkinsfile",
		strings.NewReader(`{"name":"api-migration","source":"pipeline { agent any; stages { stage('Build') { steps { sh 'cd /Users/gamer/Desktop/app && go build ./...' } } } }"}`),
	)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("format", "jenkinsfile")
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	response := httptest.NewRecorder()

	handler.migratePipeline(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), migration.ResultVersion) || !strings.Contains(response.Body.String(), `"stage_count":1`) {
		t.Fatalf("response = %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"macos_protected_directory"`) {
		t.Fatalf("response does not expose the protected-directory review warning: %s", response.Body.String())
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestProjectSummaryViewKeepsTheLegacyFullViewAndExcludesConfig(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "project-summary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()

	largeConfig := `{"stages":[],"metadata":{"padding":"` + strings.Repeat("x", 512*1024) + `"}}`
	if _, err := data.CreateProject(
		"summary-wire-test",
		"large pipeline definition",
		"",
		"git",
		"main",
		largeConfig,
		0,
		nil,
		nil,
		[]string{"qa"},
	); err != nil {
		t.Fatal(err)
	}
	handler := &handlers{d: Deps{Store: data}}

	summaryResponse := httptest.NewRecorder()
	handler.listProjects(summaryResponse, httptest.NewRequest(http.MethodGet, "/api/projects/?view=summary", nil))
	if summaryResponse.Code != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", summaryResponse.Code, summaryResponse.Body.String())
	}
	if version := summaryResponse.Header().Get("X-Buildworld-View-Version"); version != "1" {
		t.Fatalf("summary version = %q, want 1", version)
	}
	if strings.Contains(summaryResponse.Body.String(), `"config"`) {
		t.Fatal("summary response unexpectedly contains pipeline config")
	}

	fullResponse := httptest.NewRecorder()
	handler.listProjects(fullResponse, httptest.NewRequest(http.MethodGet, "/api/projects/", nil))
	if fullResponse.Code != http.StatusOK {
		t.Fatalf("full status = %d, body = %s", fullResponse.Code, fullResponse.Body.String())
	}
	if !strings.Contains(fullResponse.Body.String(), strings.Repeat("x", 256)) {
		t.Fatal("legacy full response no longer contains the pipeline config")
	}
	if summaryResponse.Body.Len()*100 >= fullResponse.Body.Len() {
		t.Fatalf("summary response is not materially smaller: summary=%d full=%d", summaryResponse.Body.Len(), fullResponse.Body.Len())
	}
}

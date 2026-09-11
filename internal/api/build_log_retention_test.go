package api

import (
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

func TestBuildLogResponsesExposeDurableRetention(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "log-retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("logs", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	retained := store.BuildLogTruncationMarker + "[12:00:00] [watch] newest\n"
	if err := database.SetBuildLog(build.ID, retained); err != nil {
		t.Fatal(err)
	}
	handler := &handlers{d: Deps{Store: database}}

	logResponse := httptest.NewRecorder()
	handler.getBuildLogs(logResponse, buildLogRequest(build.ID, "/logs"))
	if logResponse.Code != http.StatusOK ||
		!strings.Contains(logResponse.Body.String(), `"truncated":true`) ||
		!strings.Contains(logResponse.Body.String(), `"retention_characters":1000000`) {
		t.Fatalf("log response = %d %s", logResponse.Code, logResponse.Body.String())
	}

	downloadResponse := httptest.NewRecorder()
	handler.downloadBuildLogs(downloadResponse, buildLogRequest(build.ID, "/logs/download?format=txt"))
	if downloadResponse.Code != http.StatusOK ||
		downloadResponse.Header().Get("X-BuildWorld-Log-Truncated") != "true" ||
		downloadResponse.Header().Get("X-BuildWorld-Log-Retention-Characters") != strconv.Itoa(store.BuildLogRetentionCharacters) ||
		!strings.HasPrefix(downloadResponse.Body.String(), store.BuildLogTruncationMarker) {
		t.Fatalf("download response = %d headers=%v body=%.160q", downloadResponse.Code, downloadResponse.Header(), downloadResponse.Body.String())
	}
}

func TestBuildLogResponseSupportsNewestTailWindow(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "log-window.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("logs", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetBuildLog(build.ID, "oldest\nmiddle\nnewest\n"); err != nil {
		t.Fatal(err)
	}

	handler := &handlers{d: Deps{Store: database}}
	response := httptest.NewRecorder()
	handler.getBuildLogs(response, buildLogRequest(build.ID, "/logs?tail_lines=2&tail_characters=100"))
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"windowed":true`) ||
		!strings.Contains(body, `"window_truncated":true`) || strings.Contains(body, "oldest\\n") ||
		!strings.Contains(body, "middle\\nnewest\\n") {
		t.Fatalf("tail response = %d %s", response.Code, body)
	}

	metadataResponse := httptest.NewRecorder()
	handler.getBuild(metadataResponse, buildLogRequest(build.ID, "/?include_log=false"))
	if metadataResponse.Code != http.StatusOK || strings.Contains(metadataResponse.Body.String(), "oldest") {
		t.Fatalf("metadata response still contains the durable log: %d %s", metadataResponse.Code, metadataResponse.Body.String())
	}
}

func buildLogRequest(buildID int64, suffix string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/api/builds/"+strconv.FormatInt(buildID, 10)+suffix, nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", strconv.FormatInt(buildID, 10))
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}

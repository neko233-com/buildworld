package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestReorderBuildQueueUsesVersionedOperation(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "queue-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("queue-api", "", "", "git", "main", `{"stages":[]}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var last *store.BuildQueueItem
	for number := 1; number <= 3; number++ {
		build, err := data.CreateBuild(project.ID, number, "manual", "main", "", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		last, err = data.CreateBuildQueueItem(build.ID, project.ID, project.Name, 0, build.Trigger, build.Branch)
		if err != nil {
			t.Fatal(err)
		}
	}
	handler := &handlers{d: Deps{Store: data}}
	response := httptest.NewRecorder()
	handler.reorderBuildQueue(
		response,
		requestWithRouteID(http.MethodPut, "/api/build-queue/3", strings.NewReader(`{"operation":"move_top"}`), last.ID),
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result engine.BuildQueueReorderResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Version != engine.BuildQueueReorderVersion || result.Position != 1 || result.Items[0].ID != last.ID {
		t.Fatalf("result = %#v", result)
	}
}

func TestReorderBuildQueueRejectsUnknownOperation(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "queue-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	handler := &handlers{d: Deps{Store: data}}
	response := httptest.NewRecorder()
	handler.reorderBuildQueue(
		response,
		requestWithRouteID(http.MethodPut, "/api/build-queue/1", strings.NewReader(`{"operation":"teleport"}`), 1),
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestReorderBuildQueueRequiresNamedOperation(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "queue-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	handler := &handlers{d: Deps{Store: data}}

	for _, body := range []string{`{}`, `{"priority":42}`} {
		response := httptest.NewRecorder()
		handler.reorderBuildQueue(
			response,
			requestWithRouteID(http.MethodPut, "/api/build-queue/1", strings.NewReader(body), 1),
		)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "operation is required") {
			t.Fatalf("body %s: status = %d, response = %s", body, response.Code, response.Body.String())
		}
	}
}

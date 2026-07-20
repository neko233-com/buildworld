package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestCreateProjectGroupReturnsStableConflictCode(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "project-groups.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	if _, err := data.CreateProjectGroup("games", "", nil); err != nil {
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

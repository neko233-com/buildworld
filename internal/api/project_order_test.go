package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestReorderProjectsValidatesAndPersistsOrder(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "project-order.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	first, err := data.CreateProject("first", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := data.CreateProject("second", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/projects/order", strings.NewReader(
		`{"ordered_ids":[`+strconv.FormatInt(first.ID, 10)+`,`+strconv.FormatInt(second.ID, 10)+`]}`,
	))
	(&handlers{d: Deps{Store: data}}).reorderProjects(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	projects, err := data.ListProjectSummaries()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 || projects[0].ID != first.ID || projects[1].ID != second.ID {
		t.Fatalf("unexpected projects: %#v", projects)
	}
}

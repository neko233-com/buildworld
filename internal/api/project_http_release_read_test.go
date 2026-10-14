package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestProjectHTTPReleaseObserverScopeAndRevocation(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "release-observer.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project, err := database.CreateProject("release-observer", "", "", "git", "main", "jobs:\n  build:\n    steps: []\n", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := database.CreateProject("unrelated-project", "", "", "git", "main", "jobs:\n  build:\n    steps: []\n", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectHTTPTrigger(project.ID, true, "fixture-project-access"); err != nil {
		t.Fatal(err)
	}
	parameters := `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"release","RELEASE_ACTION":"publish-latest","PRIVATE":"parameter-sentinel"}`
	build, err := database.CreateBuild(project.ID, 1, "http", "main", "", parameters, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherBuild, err := database.CreateBuild(other.ID, 1, "http", "main", "", parameters, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	marker := store.ReleaseResultMarker + `{"channel":"wechat","environment":"release","action":"publish-latest","component":"game-server","version":"0.1.133","branch":"release/wechat/release/game-server/0.1.133","sourceCommit":"` + strings.Repeat("a", 40) + `","configCommit":"` + strings.Repeat("b", 40) + `","configSnapshotId":"` + strings.Repeat("c", 64) + `","configVersion":"355"}`
	if err := database.AppendBuildLog(build.ID, "log-sentinel\n"+marker+"\n"); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: auth.NewJWT("release-observer-fixture")})
	base := fmt.Sprintf("/api/trigger/projects/%d/fixture-project-access", project.ID)
	get := func(path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		return response
	}
	response := get(base + fmt.Sprintf("/builds/%d", build.ID))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("release status = %d", response.Code)
	}
	for _, forbidden := range []string{"parameter-sentinel", "log-sentinel", "fixture-project-access", `"parameters"`, `"log"`} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("release status exposed %s", forbidden)
		}
	}
	metadata, err := database.GetProjectBuildReleaseStatus(project.ID, build.ID)
	if err != nil || metadata.Log != "" || metadata.Parameters != "" {
		t.Fatal("observer query loaded unrelated log or parameter data")
	}
	for _, path := range []string{fmt.Sprintf("/builds/%d", otherBuild.ID), "/builds/-1", "/builds/not-an-id"} {
		if response := get(base + path); response.Code != http.StatusNotFound {
			t.Fatalf("cross-project or malformed read = %d", response.Code)
		}
	}
	if response := get(base + "/release-status"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"schemaVersion":1`) {
		t.Fatal("project release observer capability missing")
	}
	var records []*store.BuildReleaseResult
	response = get(base + "/release-records")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &records) != nil || len(records) != 0 {
		t.Fatal("unfinished release appeared in successful record list")
	}
	if err := database.FinishBuild(build.ID, "success", 100); err != nil {
		t.Fatal(err)
	}
	response = get(base + "/release-records")
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &records) != nil || len(records) != 1 || records[0].ConfigVersion != "355" {
		t.Fatal("completed release record missing fixed identity")
	}
	for _, suffix := range []string{"/release-status", "/release-records", fmt.Sprintf("/builds/%d", build.ID)} {
		if response := get(strings.Replace(base, "fixture-project-access", "wrong-access", 1) + suffix); response.Code != http.StatusNotFound {
			t.Fatalf("wrong credential accepted: %d", response.Code)
		}
	}
	if err := database.SetProjectEnabled(project.ID, false); err != nil {
		t.Fatal(err)
	}
	if response := get(base + "/release-status"); response.Code != http.StatusConflict {
		t.Fatalf("disabled project = %d", response.Code)
	}
	if err := database.SetProjectEnabled(project.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectHTTPTrigger(project.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"/release-status", "/release-records", fmt.Sprintf("/builds/%d", build.ID)} {
		if response := get(base + suffix); response.Code != http.StatusNotFound {
			t.Fatalf("revoked credential accepted: %d", response.Code)
		}
	}
}

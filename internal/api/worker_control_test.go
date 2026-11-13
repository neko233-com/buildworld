package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/workercontrol"
)

func TestWorkerManagementRequiresAdminAndWorkerScopedHeartbeat(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "workers.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	jwt := auth.NewJWT("test-worker-control-secret")
	tokens := map[string]string{}
	for _, role := range []string{"admin", "viewer"} {
		user, e := s.CreateUser(role, role+"@example.test", "unused", role)
		if e != nil {
			t.Fatal(e)
		}
		tokens[role], _ = jwt.Generate(user.ID, role, user.SessionVersion, time.Hour)
	}
	hash, _ := auth.HashPassword("worker-test-only")
	_, _ = s.CreateWorker("worker", "worker", "localhost:1", hash, nil, 2, "")
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: s, JWT: jwt})
	call := func(method, path, role, body, workerToken string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if role != "" {
			request.Header.Set("Authorization", "Bearer "+tokens[role])
		}
		if workerToken != "" {
			request.Header.Set("X-Buildworld-Worker-Token", workerToken)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	for _, path := range []string{"/api/agents/worker/runtime", "/api/agents/worker/operations", "/api/agents/worker/update"} {
		method := "POST"
		if path == "/api/agents/worker/runtime" {
			method = "GET"
		}
		if response := call(method, path, "viewer", "{}", ""); response.Code != 403 {
			t.Fatalf("viewer admitted: %s %d", path, response.Code)
		}
		if response := call(method, path, "", "{}", ""); response.Code != 401 {
			t.Fatalf("anonymous admitted: %s %d", path, response.Code)
		}
	}
	queued := call("POST", "/api/agents/worker/operations", "admin", `{"kind":"drain","request_id":"fixed-request"}`, "")
	if queued.Code != 202 {
		t.Fatal(queued.Code, queued.Body.String())
	}
	var operation workercontrol.Operation
	_ = json.Unmarshal(queued.Body.Bytes(), &operation)
	if response := call("POST", "/api/agents/heartbeat/worker", "", `{}`, "wrong"); response.Code != 401 {
		t.Fatal("worker authentication bypass", response.Code)
	}
	if response := call("POST", "/api/agents/heartbeat/worker", "", `{}`, "worker-test-only"); response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	body, _ := json.Marshal(workercontrol.Heartbeat{Runtime: &workercontrol.Runtime{Version: "1.17.0", Capacity: 2, Tasks: []workercontrol.Task{}}, Completion: &workercontrol.Completion{ID: operation.ID, Status: "success"}})
	if response := call("POST", "/api/agents/heartbeat/worker", "", string(body), "worker-test-only"); response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	response := call("GET", "/api/agents/worker/runtime", "admin", "", "")
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"draining":true`)) || bytes.Contains(response.Body.Bytes(), []byte("worker-test-only")) {
		t.Fatal("runtime evidence or credential boundary", response.Code, response.Body.String())
	}
	if response := call("POST", "/api/agents/worker/operations", "admin", `{"kind":"update","request_id":"unsafe","version":"1.17.0"}`, ""); response.Code != 400 {
		t.Fatal("unverified update admitted", response.Code)
	}
	if response := call("GET", "/api/agents/heartbeat/worker/bundle/"+operation.ID, "", "", ""); response.Code != 401 {
		t.Fatal("bundle authentication bypass", response.Code)
	}
}

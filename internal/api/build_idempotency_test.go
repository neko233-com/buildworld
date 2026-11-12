package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestBuildTriggerIdempotencyReturnsOriginalAndRejectsChanges(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	user, err := database.CreateUser("builder", "builder@example.test", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject("demo", "", "", "git", "main", "jobs:\n  build:\n    steps:\n      - run: echo fixture\n", user.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWT("fixture-idempotency-secret")
	token, err := jwt.Generate(user.ID, user.Role, user.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: jwt})
	trigger := func(body, key string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/projects/%d/builds", project.ID), strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", key)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	key := "stable-request-0001"
	guarded := trigger(`{"branch":"main","prestart_contract":"v1","expected_pipeline_repo":"https://git.example/game.git","parameters":{"OPENING_PROBE_SECRET":"fixture"}}`, "stable-prestart-0001")
	if guarded.Code != http.StatusBadRequest || strings.Contains(guarded.Body.String(), "fixture") {
		t.Fatalf("non-prestart project accepted or exposed a secret: %d", guarded.Code)
	}
	first := trigger(`{"branch":"main"}`, key)
	if first.Code != http.StatusCreated || first.Header().Get("Idempotency-Supported") != "v1" {
		t.Fatalf("first request: %d %s", first.Code, first.Body.String())
	}
	var original store.Build
	if err := json.Unmarshal(first.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	replay := trigger(`{"branch":"main"}`, key)
	var replayed store.Build
	if err := json.Unmarshal(replay.Body.Bytes(), &replayed); err != nil {
		t.Fatal(err)
	}
	if replay.Code != http.StatusOK || replay.Header().Get("Idempotency-Supported") != "v1" || replay.Header().Get("Idempotency-Replayed") != "true" || replayed.ID != original.ID {
		t.Fatalf("replay: %d %s", replay.Code, replay.Body.String())
	}
	if changed := trigger(`{"branch":"other"}`, key); changed.Code != http.StatusConflict {
		t.Fatalf("changed request status=%d", changed.Code)
	}
	if bad := trigger(`{}`, "short"); bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid key status=%d", bad.Code)
	}
	builds, err := database.ListBuildsByProject(project.ID)
	if err != nil || len(builds) != 1 {
		t.Fatalf("build count=%d err=%v", len(builds), err)
	}
}

func TestIdempotentBuildRetainsNotificationPolicyAndRechecksRole(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	user, err := database.CreateUser("policy-admin", "policy@example.test", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject("demo", "", "", "git", "main", "jobs:\n  build:\n    steps:\n      - run: echo fixture\n", user.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWT("fixture-idempotency-secret")
	token, err := jwt.Generate(user.ID, user.Role, user.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: jwt})
	trigger := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/projects/%d/builds", project.ID), strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", "stable-policy-request-0001")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	body := `{"notification_mode":"in_app_only","notification_reason":"testing"}`
	first := trigger(body)
	var original store.Build
	if first.Code != http.StatusCreated || json.Unmarshal(first.Body.Bytes(), &original) != nil || original.NotificationMode != "in_app_only" {
		t.Fatalf("first=%d %s", first.Code, first.Body.String())
	}
	replay := trigger(body)
	var repeated store.Build
	if replay.Code != http.StatusOK || json.Unmarshal(replay.Body.Bytes(), &repeated) != nil || repeated.ID != original.ID || repeated.NotificationMode != "in_app_only" {
		t.Fatalf("replay=%d %s", replay.Code, replay.Body.String())
	}
	if changed := trigger(`{"notification_mode":"in_app_only","notification_reason":"maintenance"}`); changed.Code != http.StatusConflict {
		t.Fatalf("changed policy=%d", changed.Code)
	}
	if err := database.UpdateUserRole(user.ID, "developer"); err != nil {
		t.Fatal(err)
	}
	if denied := trigger(body); denied.Code != http.StatusForbidden {
		t.Fatalf("demoted replay=%d", denied.Code)
	}
}

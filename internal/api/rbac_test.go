package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestRouterRoleAuthorization(t *testing.T) {
	dataStore, err := store.New(filepath.Join(t.TempDir(), "rbac.db"))
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = dataStore.Close() })

	jwt := auth.NewJWT("rbac-integration-test-secret")
	users := map[string]*store.User{}
	for _, role := range []string{"admin", "developer", "viewer"} {
		user, createErr := dataStore.CreateUser(role, role+"@example.test", "unused", role)
		if createErr != nil {
			t.Fatalf("CreateUser(%s) error = %v", role, createErr)
		}
		users[role] = user
	}

	tokens := map[string]string{}
	for role, user := range users {
		token, generateErr := jwt.Generate(user.ID, role, user.SessionVersion, time.Hour)
		if generateErr != nil {
			t.Fatalf("Generate(%s) error = %v", role, generateErr)
		}
		tokens[role] = token
	}

	router := NewRouter(Deps{
		Cfg:   &config.Config{Server: config.ServerConfig{Port: 7777}},
		Store: dataStore,
		JWT:   jwt,
	})

	request := func(method, path, role, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if role != "" {
			req.Header.Set("Authorization", "Bearer "+tokens[role])
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder
	}

	if got := request(http.MethodPost, "/api/projects/", "viewer", `{"name":"denied"}`).Code; got != http.StatusForbidden {
		t.Fatalf("viewer create project status = %d, want %d", got, http.StatusForbidden)
	}
	if got := request(http.MethodPost, "/api/projects/", "developer", `{"name":"allowed","config":"jobs:\n  build:\n    steps:\n      - run: echo ok\n"}`).Code; got != http.StatusCreated {
		t.Fatalf("developer create project status = %d, want %d", got, http.StatusCreated)
	}
	if got := request(http.MethodGet, "/api/users/", "developer", "").Code; got != http.StatusForbidden {
		t.Fatalf("developer list users status = %d, want %d", got, http.StatusForbidden)
	}
	if got := request(http.MethodGet, "/api/users/", "admin", "").Code; got != http.StatusOK {
		t.Fatalf("admin list users status = %d, want %d", got, http.StatusOK)
	}
	if got := request(http.MethodGet, "/api/settings", "viewer", "").Code; got != http.StatusForbidden {
		t.Fatalf("viewer settings status = %d, want %d", got, http.StatusForbidden)
	}
	if got := request(http.MethodGet, "/api/system/storage", "viewer", "").Code; got != http.StatusOK {
		t.Fatalf("viewer system storage status = %d, want %d", got, http.StatusOK)
	}
	if got := request(http.MethodGet, "/api/system/update/", "developer", "").Code; got != http.StatusForbidden {
		t.Fatalf("developer system update status = %d, want %d", got, http.StatusForbidden)
	}
	if got := request(http.MethodGet, "/api/system/update/", "admin", "").Code; got != http.StatusOK {
		t.Fatalf("admin system update status = %d, want %d", got, http.StatusOK)
	}
	if got := request(http.MethodGet, "/api/notifications/in-app", "viewer", "").Code; got != http.StatusOK {
		t.Fatalf("viewer in-app notifications status = %d, want %d", got, http.StatusOK)
	}
	if got := request(http.MethodGet, "/api/notifications/channels/", "developer", "").Code; got != http.StatusForbidden {
		t.Fatalf("developer notification channels status = %d, want %d", got, http.StatusForbidden)
	}
	if got := request(http.MethodGet, "/api/notifications/channels/", "admin", "").Code; got != http.StatusOK {
		t.Fatalf("admin notification channels status = %d, want %d", got, http.StatusOK)
	}
	if got := request(http.MethodPost, "/api/auth/register", "", `{"username":"public","password":"blocked"}`).Code; got != http.StatusUnauthorized {
		t.Fatalf("public register status = %d, want %d", got, http.StatusUnauthorized)
	}
	if got := request(http.MethodGet, "/api/bigscreen", "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("public bigscreen status = %d, want %d", got, http.StatusUnauthorized)
	}
	if got := request(http.MethodGet, "/ws", "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("public websocket status = %d, want %d", got, http.StatusUnauthorized)
	}
}

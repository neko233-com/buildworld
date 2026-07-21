package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func persistAPIToken(t *testing.T, database *store.Store, userID int64, raw string, scopes string) *store.APIToken {
	t.Helper()
	hash := sha256.Sum256([]byte(raw))
	token, err := database.CreateAPIToken(userID, "security-test", hex.EncodeToString(hash[:]), raw[:11], scopes, nil)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func apiTokenRequest(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestViewerCannotCreateBuildTriggerToken(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "viewer-token.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	viewer, err := database.CreateUser("viewer", "viewer@example.test", "unused", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWT("viewer-token-test-secret")
	session, err := jwt.Generate(viewer.ID, viewer.Role, viewer.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: jwt})

	response := apiTokenRequest(router, http.MethodPost, "/api/api-tokens/", session, `{"name":"denied","scopes":["build:trigger"]}`)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusForbidden, response.Body.String())
	}
	tokens, err := database.ListAPITokens(viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 0 {
		t.Fatalf("viewer token count = %d, want 0", len(tokens))
	}
}

func TestBuildTriggerRequiresCurrentEditorRoleAndExplicitScope(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "trigger-token.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	developer, err := database.CreateUser("developer", "developer@example.test", "unused", "developer")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := database.CreateUser("viewer", "viewer-trigger@example.test", "unused", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	orphanOwner, err := database.CreateUser("orphan", "orphan@example.test", "unused", "developer")
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject(
		"token-trigger-project",
		"",
		"https://example.test/token-trigger.git",
		"git",
		"main",
		"jobs:\n  build:\n    steps:\n      - run: echo ok\n",
		developer.ID,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	noScopeRaw := "bw_44444444444444444444444444444444"
	viewerRaw := "bw_55555555555555555555555555555555"
	scopedRaw := "bw_66666666666666666666666666666666"
	orphanRaw := "bw_77777777777777777777777777777777"
	noScopeToken := persistAPIToken(t, database, developer.ID, noScopeRaw, `[]`)
	viewerToken := persistAPIToken(t, database, viewer.ID, viewerRaw, `["build:trigger"]`)
	scopedToken := persistAPIToken(t, database, developer.ID, scopedRaw, `["build:trigger"]`)
	persistAPIToken(t, database, orphanOwner.ID, orphanRaw, `["build:trigger"]`)
	if _, err := database.DB().Exec("DELETE FROM users WHERE id = ?", orphanOwner.ID); err != nil {
		t.Fatal(err)
	}

	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: auth.NewJWT("trigger-token-test-secret")})
	path := "/api/trigger/" + project.Name
	for _, test := range []struct {
		name   string
		token  string
		status int
	}{
		{name: "missing scope", token: noScopeRaw, status: http.StatusForbidden},
		{name: "viewer owner", token: viewerRaw, status: http.StatusForbidden},
		{name: "missing owner", token: orphanRaw, status: http.StatusUnauthorized},
		{name: "editor and scope", token: scopedRaw, status: http.StatusCreated},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := apiTokenRequest(router, http.MethodPost, path, test.token, `{}`)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
		})
	}

	builds, err := database.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 1 {
		t.Fatalf("build count = %d, want 1", len(builds))
	}
	for _, token := range []*store.APIToken{noScopeToken, viewerToken} {
		persisted, err := database.GetAPIToken(token.ID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.LastUsedAt != nil {
			t.Fatalf("denied token %d last_used_at = %v, want nil", token.ID, persisted.LastUsedAt)
		}
	}
	persistedScoped, err := database.GetAPIToken(scopedToken.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedScoped.LastUsedAt == nil {
		t.Fatal("accepted scoped token did not update last_used_at")
	}

	orphanResponse := apiTokenRequest(router, http.MethodGet, "/api/projects/", orphanRaw, "")
	if orphanResponse.Code != http.StatusUnauthorized {
		t.Fatalf("orphan middleware status = %d, want %d", orphanResponse.Code, http.StatusUnauthorized)
	}

	noScopeResponse := apiTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/projects/%d/builds", project.ID), noScopeRaw, `{}`)
	if noScopeResponse.Code != http.StatusForbidden {
		t.Fatalf("regular build endpoint without scope status = %d, want %d", noScopeResponse.Code, http.StatusForbidden)
	}
	scopedResponse := apiTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/projects/%d/builds", project.ID), scopedRaw, `{}`)
	if scopedResponse.Code != http.StatusCreated {
		t.Fatalf("regular build endpoint with scope status = %d, want %d; body=%s", scopedResponse.Code, http.StatusCreated, scopedResponse.Body.String())
	}
	for _, action := range []string{"retry", "approve"} {
		response := apiTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/builds/%d/%s", builds[0].ID, action), noScopeRaw, `{}`)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s without scope status = %d, want %d", action, response.Code, http.StatusForbidden)
		}
	}
}

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestJWTSessionUsesCurrentUserStateAndRevokesOnPasswordReset(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "jwt-session.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	actorHash, err := auth.HashPassword("actor-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateUser("actor", "actor@example.test", actorHash, "admin"); err != nil {
		t.Fatal(err)
	}
	targetHash, err := auth.HashPassword("target-password")
	if err != nil {
		t.Fatal(err)
	}
	target, err := database.CreateUser("target", "target@example.test", targetHash, "admin")
	if err != nil {
		t.Fatal(err)
	}

	router := NewRouter(Deps{
		Cfg:   &config.Config{},
		Store: database,
		JWT:   auth.NewJWT("jwt-session-api-test-secret"),
	})
	login := func(username, password string) (string, *http.Cookie) {
		t.Helper()
		response := jwtSessionRequest(router, http.MethodPost, "/api/auth/login", "", nil,
			fmt.Sprintf(`{"username":%q,"password":%q}`, username, password))
		if response.Code != http.StatusOK {
			t.Fatalf("login %s status = %d; body=%s", username, response.Code, response.Body.String())
		}
		var payload loginResp
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		var sessionCookie *http.Cookie
		for _, cookie := range response.Result().Cookies() {
			if cookie.Name == "bw_session" {
				sessionCookie = cookie
				break
			}
		}
		if payload.Token == "" || sessionCookie == nil {
			t.Fatal("login response missing token or bw_session cookie")
		}
		return payload.Token, sessionCookie
	}

	actorToken, _ := login("actor", "actor-password")
	oldTargetToken, oldTargetCookie := login("target", "target-password")

	demote := jwtSessionRequest(router, http.MethodPut, fmt.Sprintf("/api/users/%d/role", target.ID), actorToken, nil, `{"role":"viewer"}`)
	if demote.Code != http.StatusOK {
		t.Fatalf("demote status = %d; body=%s", demote.Code, demote.Body.String())
	}
	if response := jwtSessionRequest(router, http.MethodGet, "/api/users/", oldTargetToken, nil, ""); response.Code != http.StatusForbidden {
		t.Fatalf("demoted token admin status = %d, want %d", response.Code, http.StatusForbidden)
	}
	me := jwtSessionRequest(router, http.MethodGet, "/api/auth/me", oldTargetToken, nil, "")
	if me.Code != http.StatusOK {
		t.Fatalf("demoted token me status = %d; body=%s", me.Code, me.Body.String())
	}
	var currentUser store.User
	if err := json.Unmarshal(me.Body.Bytes(), &currentUser); err != nil {
		t.Fatal(err)
	}
	if currentUser.Role != "viewer" {
		t.Fatalf("demoted token current role = %q, want viewer", currentUser.Role)
	}

	reset := jwtSessionRequest(router, http.MethodPut, fmt.Sprintf("/api/users/%d/password", target.ID), actorToken, nil, `{"password":"new-target-password"}`)
	if reset.Code != http.StatusOK {
		t.Fatalf("password reset status = %d; body=%s", reset.Code, reset.Body.String())
	}
	if response := jwtSessionRequest(router, http.MethodGet, "/api/auth/me", oldTargetToken, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("old bearer after password reset status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if response := jwtSessionRequest(router, http.MethodGet, "/api/auth/me", "", oldTargetCookie, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("old cookie after password reset status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if response := jwtSessionRequest(router, http.MethodPost, "/api/auth/login", "", nil, `{"username":"target","password":"target-password"}`); response.Code != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	newTargetToken, _ := login("target", "new-target-password")
	if response := jwtSessionRequest(router, http.MethodGet, "/api/auth/me", newTargetToken, nil, ""); response.Code != http.StatusOK {
		t.Fatalf("new token status = %d; body=%s", response.Code, response.Body.String())
	}
	deleteResponse := jwtSessionRequest(router, http.MethodDelete, fmt.Sprintf("/api/users/%d/", target.ID), actorToken, nil, "")
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d; body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	if response := jwtSessionRequest(router, http.MethodGet, "/api/auth/me", newTargetToken, nil, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("deleted user token status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func jwtSessionRequest(router http.Handler, method, path, bearer string, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestMiddlewareAuthenticatesNestedChiRoute(t *testing.T) {
	jwt := NewJWT("middleware-test-secret")
	token, err := jwt.Generate(42, "admin", 3, time.Hour)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	router := chi.NewRouter()
	router.Route("/api", func(r chi.Router) {
		r.Use(Middleware(jwt, func(userID, sessionVersion int64) (string, bool) {
			return "admin", userID == 42 && sessionVersion == 3
		}, nil, "/api/auth/login"))
		r.Get("/auth/me", func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, UserIDFromContext(r.Context()))
		})
	})

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("request without token status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer "+token)
	authorized := httptest.NewRecorder()
	router.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("request with token status = %d, want %d", authorized.Code, http.StatusOK)
	}
	if authorized.Body.String() != "42" {
		t.Fatalf("authenticated user id = %q, want 42", authorized.Body.String())
	}
}

func TestMiddlewarePublicPathMatchingIsExactUnlessDirectory(t *testing.T) {
	jwt := NewJWT("middleware-test-secret")
	handler := Middleware(jwt, nil, nil, "/api/auth/login", "/api/webhooks/")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		path string
		want int
	}{
		{path: "/api/auth/login", want: http.StatusNoContent},
		{path: "/api/auth/login-extra", want: http.StatusUnauthorized},
		{path: "/api/webhooks/github", want: http.StatusNoContent},
		{path: "/api/webhooks", want: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}
}

func TestMiddlewareUsesCurrentRoleAndRejectsRevokedSession(t *testing.T) {
	jwt := NewJWT("middleware-current-user-secret")
	token, err := jwt.Generate(81, "admin", 4, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	currentRole := "viewer"
	currentVersion := int64(4)
	userExists := true
	validator := func(userID, sessionVersion int64) (string, bool) {
		if !userExists || userID != 81 || sessionVersion != currentVersion {
			return "", false
		}
		return currentRole, true
	}
	handler := Middleware(jwt, validator, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, RoleFromContext(r.Context()))
	}))

	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder
	}

	if response := request(); response.Code != http.StatusOK || response.Body.String() != "viewer" {
		t.Fatalf("current role response = %d %q, want 200 viewer", response.Code, response.Body.String())
	}
	currentVersion++
	if response := request(); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	currentVersion--
	userExists = false
	if response := request(); response.Code != http.StatusUnauthorized {
		t.Fatalf("deleted user status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestRequireRolesAllowsOnlyConfiguredRoles(t *testing.T) {
	handler := RequireRoles("admin", "developer")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, tt := range []struct {
		role string
		want int
	}{
		{role: "admin", want: http.StatusNoContent},
		{role: "developer", want: http.StatusNoContent},
		{role: "viewer", want: http.StatusForbidden},
		{role: "", want: http.StatusForbidden},
	} {
		t.Run(tt.role, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.role != "" {
				req = req.WithContext(context.WithValue(req.Context(), CtxRole, tt.role))
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}
}

func TestRequireAPITokenScope(t *testing.T) {
	handler := RequireAPITokenScope(ScopeBuildTrigger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	jwtRequest := httptest.NewRequest(http.MethodPost, "/", nil)
	jwtRecorder := httptest.NewRecorder()
	handler.ServeHTTP(jwtRecorder, jwtRequest)
	if jwtRecorder.Code != http.StatusNoContent {
		t.Fatalf("JWT request status = %d, want %d", jwtRecorder.Code, http.StatusNoContent)
	}

	missingScope := httptest.NewRequest(http.MethodPost, "/", nil)
	missingScope = missingScope.WithContext(context.WithValue(missingScope.Context(), CtxAPITokenScopes, map[string]struct{}{}))
	missingRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingRecorder, missingScope)
	if missingRecorder.Code != http.StatusForbidden {
		t.Fatalf("missing-scope API token status = %d, want %d", missingRecorder.Code, http.StatusForbidden)
	}

	withScope := httptest.NewRequest(http.MethodPost, "/", nil)
	withScope = withScope.WithContext(context.WithValue(withScope.Context(), CtxAPITokenScopes, map[string]struct{}{ScopeBuildTrigger: {}}))
	withRecorder := httptest.NewRecorder()
	handler.ServeHTTP(withRecorder, withScope)
	if withRecorder.Code != http.StatusNoContent {
		t.Fatalf("scoped API token status = %d, want %d", withRecorder.Code, http.StatusNoContent)
	}
}

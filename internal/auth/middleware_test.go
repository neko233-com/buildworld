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
	token, err := jwt.Generate(42, "admin", time.Hour)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	router := chi.NewRouter()
	router.Route("/api", func(r chi.Router) {
		r.Use(Middleware(jwt, nil, "/api/auth/login"))
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
	handler := Middleware(jwt, nil, "/api/auth/login", "/api/webhooks/")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

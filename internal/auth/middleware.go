package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	// CtxUserID is the authenticated user id in request context.
	CtxUserID contextKey = "user_id"
	// CtxRole is the authenticated user role in request context.
	CtxRole contextKey = "role"
)

// Middleware returns an HTTP middleware that validates a Bearer JWT token.
// Public paths (e.g. login, health, webhooks) bypass auth.
func Middleware(jwt *JWT, publicPrefixes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range publicPrefixes {
				if strings.HasPrefix(r.URL.Path, p) {
					next.ServeHTTP(w, r)
					return
				}
			}

			token := extractToken(r)
			if token == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			claims, err := jwt.Validate(token)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), CtxUserID, claims.UserID)
			ctx = context.WithValue(ctx, CtxRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if qt := r.URL.Query().Get("token"); qt != "" {
		return qt
	}
	return ""
}

// UserIDFromContext returns the authenticated user id, or 0 if absent.
func UserIDFromContext(ctx context.Context) int64 {
	if v, ok := ctx.Value(CtxUserID).(int64); ok {
		return v
	}
	return 0
}

// RoleFromContext returns the authenticated user role, or "" if absent.
func RoleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(CtxRole).(string); ok {
		return v
	}
	return ""
}

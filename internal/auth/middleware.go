package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/neko233-com/buildworld233/internal/store"
)

type contextKey string

const (
	// CtxUserID is the authenticated user id in request context.
	CtxUserID contextKey = "user_id"
	// CtxRole is the authenticated user role in request context.
	CtxRole contextKey = "role"
)

// APITokenValidator 验证 API Token 并返回 (userID, role, ok)。
type APITokenValidator func(token string) (userID int64, role string, ok bool)

// NewAPITokenValidator 基于 store 创建 API Token 验证器（bw_ 前缀 + SHA256 hash）。
func NewAPITokenValidator(s *store.Store) APITokenValidator {
	return func(token string) (int64, string, bool) {
		if !strings.HasPrefix(token, "bw_") {
			return 0, "", false
		}
		hash := sha256.Sum256([]byte(token))
		apiToken, err := s.GetAPITokenByHash(hex.EncodeToString(hash[:]))
		if err != nil || apiToken == nil {
			return 0, "", false
		}
		if apiToken.ExpiresAt != nil && apiToken.ExpiresAt.Before(time.Now()) {
			return 0, "", false
		}
		_ = s.UpdateAPITokenLastUsed(apiToken.ID)
		user, err := s.GetUser(apiToken.UserID)
		if err != nil {
			return apiToken.UserID, "developer", true
		}
		return user.ID, user.Role, true
	}
}

// Middleware returns an HTTP middleware that validates a Bearer JWT or API Token.
// Public paths (e.g. login, health, webhooks) bypass auth.
func Middleware(jwt *JWT, apiTokenValidator APITokenValidator, publicPrefixes ...string) func(http.Handler) http.Handler {
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

			// API Token (bw_ 前缀) 优先走 API Token 验证
			if apiTokenValidator != nil && strings.HasPrefix(token, "bw_") {
				if uid, role, ok := apiTokenValidator(token); ok {
					ctx := context.WithValue(r.Context(), CtxUserID, uid)
					ctx = context.WithValue(ctx, CtxRole, role)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				http.Error(w, `{"error":"invalid api token"}`, http.StatusUnauthorized)
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

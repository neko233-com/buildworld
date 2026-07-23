package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

type contextKey string

const (
	// CtxUserID is the authenticated user id in request context.
	CtxUserID contextKey = "user_id"
	// CtxRole is the authenticated user role in request context.
	CtxRole contextKey = "role"
	// CtxAPITokenScopes stores scopes only when API-token authentication was used.
	CtxAPITokenScopes contextKey = "api_token_scopes"
)

const (
	ScopeBuildTrigger = "build:trigger"
	ScopeSystemUpdate = "system:update"
)

// APITokenPrincipal is resolved from persisted token metadata and its current
// owner. Role is deliberately read from users on every request so deletion or
// demotion takes effect without issuing a new token.
type APITokenPrincipal struct {
	TokenID int64
	UserID  int64
	Name    string
	Role    string
	Scopes  map[string]struct{}
}

// APITokenValidator validates an API token and returns its current principal.
type APITokenValidator func(token string) (*APITokenPrincipal, bool)

// JWTSessionValidator resolves current persisted role and rejects revoked
// browser/agent sessions. Implementations must read current state per request.
type JWTSessionValidator func(userID, sessionVersion int64) (role string, ok bool)

// NewJWTSessionValidator creates a fail-closed validator backed by users.
// Role is deliberately not trusted from JWT claims.
func NewJWTSessionValidator(s *store.Store) JWTSessionValidator {
	return func(userID, sessionVersion int64) (string, bool) {
		if s == nil {
			return "", false
		}
		user, err := s.GetUser(userID)
		if err != nil || user == nil || user.SessionVersion != sessionVersion {
			return "", false
		}
		return user.Role, true
	}
}

// ResolveAPIToken validates token integrity, expiry, scopes, and ownership.
// It does not update last_used_at; callers should do that only after accepting
// the token for the requested operation.
func ResolveAPIToken(s *store.Store, token string) (*APITokenPrincipal, bool) {
	if s == nil || !strings.HasPrefix(token, "bw_") {
		return nil, false
	}
	hash := sha256.Sum256([]byte(token))
	apiToken, err := s.GetAPITokenByHash(hex.EncodeToString(hash[:]))
	if err != nil || apiToken == nil {
		return nil, false
	}
	if apiToken.ExpiresAt != nil && !apiToken.ExpiresAt.After(time.Now()) {
		return nil, false
	}

	scopes := make(map[string]struct{})
	if strings.TrimSpace(apiToken.Scopes) != "" {
		var persisted []string
		if err := json.Unmarshal([]byte(apiToken.Scopes), &persisted); err != nil {
			return nil, false
		}
		for _, scope := range persisted {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				return nil, false
			}
			scopes[scope] = struct{}{}
		}
	}

	user, err := s.GetUser(apiToken.UserID)
	if err != nil || user == nil {
		return nil, false
	}
	return &APITokenPrincipal{
		TokenID: apiToken.ID,
		UserID:  user.ID,
		Name:    apiToken.Name,
		Role:    user.Role,
		Scopes:  scopes,
	}, true
}

// NewAPITokenValidator creates a fail-closed API token validator.
func NewAPITokenValidator(s *store.Store) APITokenValidator {
	return func(token string) (*APITokenPrincipal, bool) {
		principal, ok := ResolveAPIToken(s, token)
		if !ok {
			return nil, false
		}
		_ = s.UpdateAPITokenLastUsed(principal.TokenID)
		return principal, true
	}
}

// Middleware returns an HTTP middleware that validates a Bearer JWT, the native
// bw_session cookie (for browser/AI agent flows), or an API Token.
// Public paths (e.g. login, health, webhooks) bypass auth.
func Middleware(jwt *JWT, sessionValidator JWTSessionValidator, apiTokenValidator APITokenValidator, publicPrefixes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range publicPrefixes {
				if publicPathMatches(r.URL.Path, p) {
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
				if principal, ok := apiTokenValidator(token); ok {
					ctx := context.WithValue(r.Context(), CtxUserID, principal.UserID)
					ctx = context.WithValue(ctx, CtxRole, principal.Role)
					ctx = context.WithValue(ctx, CtxAPITokenScopes, principal.Scopes)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				http.Error(w, `{"error":"invalid api token"}`, http.StatusUnauthorized)
				return
			}

			if jwt == nil || sessionValidator == nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			claims, err := jwt.Validate(token)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			role, ok := sessionValidator(claims.UserID, claims.SessionVersion)
			if !ok {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), CtxUserID, claims.UserID)
			ctx = context.WithValue(ctx, CtxRole, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func publicPathMatches(path, pattern string) bool {
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(path, pattern)
	}
	return path == pattern
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	if qt := r.URL.Query().Get("token"); qt != "" {
		return qt
	}
	if session, err := r.Cookie("bw_session"); err == nil {
		return session.Value
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

// IsAPITokenRequest reports whether middleware authenticated an API token.
func IsAPITokenRequest(ctx context.Context) bool {
	_, ok := ctx.Value(CtxAPITokenScopes).(map[string]struct{})
	return ok
}

// HasAPITokenScope checks an exact persisted scope.
func HasAPITokenScope(ctx context.Context, scope string) bool {
	scopes, ok := ctx.Value(CtxAPITokenScopes).(map[string]struct{})
	if !ok {
		return false
	}
	_, ok = scopes[scope]
	return ok
}

// RequireRoles authorizes requests whose authenticated role is in roles.
// It is intended to run after Middleware has populated the request context.
func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := allowed[RoleFromContext(r.Context())]; !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAPITokenScope applies only to API-token requests. Browser/session JWT
// requests continue through normal role authorization.
func RequireAPITokenScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IsAPITokenRequest(r.Context()) && !HasAPITokenScope(r.Context(), scope) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"insufficient api token scope"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

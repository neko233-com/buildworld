package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func persistTestAPIToken(t *testing.T, database *store.Store, userID int64, raw, scopes string) {
	t.Helper()
	hash := sha256.Sum256([]byte(raw))
	if _, err := database.CreateAPIToken(userID, "test-token", hex.EncodeToString(hash[:]), raw[:11], scopes, nil); err != nil {
		t.Fatal(err)
	}
}

func TestAPITokenResolutionUsesCurrentOwnerAndScopes(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "api-token-auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	owner, err := database.CreateUser("developer", "developer@example.test", "unused", "developer")
	if err != nil {
		t.Fatal(err)
	}
	raw := "bw_0123456789abcdef0123456789abcdef"
	persistTestAPIToken(t, database, owner.ID, raw, `["build:trigger"]`)

	principal, ok := ResolveAPIToken(database, raw)
	if !ok {
		t.Fatal("valid API token was rejected")
	}
	if principal.UserID != owner.ID || principal.Role != "developer" {
		t.Fatalf("principal = %+v", principal)
	}
	if _, ok := principal.Scopes[ScopeBuildTrigger]; !ok {
		t.Fatalf("scopes = %#v, want %q", principal.Scopes, ScopeBuildTrigger)
	}

	if err := database.UpdateUserRole(owner.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	principal, ok = ResolveAPIToken(database, raw)
	if !ok || principal.Role != "viewer" {
		t.Fatalf("demoted principal = %+v, ok=%t", principal, ok)
	}
}

func TestAPITokenResolutionRejectsMissingOwnerAndMalformedScopes(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "api-token-rejection.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	owner, err := database.CreateUser("owner", "owner@example.test", "unused", "developer")
	if err != nil {
		t.Fatal(err)
	}

	orphanRaw := "bw_11111111111111111111111111111111"
	persistTestAPIToken(t, database, owner.ID, orphanRaw, `["build:trigger"]`)
	if _, err := database.DB().Exec("DELETE FROM users WHERE id = ?", owner.ID); err != nil {
		t.Fatal(err)
	}
	if principal, ok := ResolveAPIToken(database, orphanRaw); ok || principal != nil {
		t.Fatalf("orphan token resolved as %+v", principal)
	}

	malformedOwner, err := database.CreateUser("malformed", "malformed@example.test", "unused", "developer")
	if err != nil {
		t.Fatal(err)
	}
	malformedRaw := "bw_22222222222222222222222222222222"
	persistTestAPIToken(t, database, malformedOwner.ID, malformedRaw, `{}`)
	if principal, ok := ResolveAPIToken(database, malformedRaw); ok || principal != nil {
		t.Fatalf("malformed-scope token resolved as %+v", principal)
	}
}

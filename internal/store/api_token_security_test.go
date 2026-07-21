package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
)

func TestDeleteUserExplicitlyRevokesAPITokens(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "delete-user-token.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	user, err := database.CreateUser("delete-me", "delete-me@example.test", "unused", "developer")
	if err != nil {
		t.Fatal(err)
	}
	raw := "bw_33333333333333333333333333333333"
	hash := sha256.Sum256([]byte(raw))
	hashString := hex.EncodeToString(hash[:])
	if _, err := database.CreateAPIToken(user.ID, "revoked", hashString, raw[:11], `["build:trigger"]`, nil); err != nil {
		t.Fatal(err)
	}

	if err := database.DeleteUser(user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetAPITokenByHash(hashString); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetAPITokenByHash() error = %v, want sql.ErrNoRows", err)
	}
}

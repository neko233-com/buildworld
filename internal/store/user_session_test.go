package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestUpdateUserPasswordRevokesExistingSessions(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "user-session.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	user, err := database.CreateUser("session-user", "session@example.test", "old-hash", "developer")
	if err != nil {
		t.Fatal(err)
	}
	if user.SessionVersion != 1 {
		t.Fatalf("initial session version = %d, want 1", user.SessionVersion)
	}
	if err := database.UpdateUserPassword(user.ID, "new-hash"); err != nil {
		t.Fatal(err)
	}
	updated, err := database.GetUser(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PasswordHash != "new-hash" || updated.SessionVersion != 2 {
		t.Fatalf("updated user hash/version = %q/%d, want new-hash/2", updated.PasswordHash, updated.SessionVersion)
	}
	if err := database.UpdateUserPassword(user.ID, "newer-hash"); err != nil {
		t.Fatal(err)
	}
	updated, err = database.GetUser(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SessionVersion != 3 {
		t.Fatalf("second password update session version = %d, want 3", updated.SessionVersion)
	}
	if err := database.UpdateUserPassword(999999, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing user password update error = %v, want sql.ErrNoRows", err)
	}
}

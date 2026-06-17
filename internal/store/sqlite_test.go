package store

import (
	"os"
	"testing"
)

func TestSQLiteStore(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := New(tmpFile.Name())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	// Test user creation
	user, err := store.CreateUser("testuser", "test@example.com", "hashedpassword", "admin")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("Username = %s, want testuser", user.Username)
	}

	// Test user retrieval
	fetched, err := store.GetUserByUsername("testuser")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}

	if fetched.ID != user.ID {
		t.Errorf("ID = %d, want %d", fetched.ID, user.ID)
	}
}

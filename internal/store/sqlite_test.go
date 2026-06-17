package store

import (
	"os"
	"testing"
)

func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	name := tmpFile.Name()
	tmpFile.Close()

	store, err := New(name)
	if err != nil {
		os.Remove(name)
		t.Fatalf("New() error = %v", err)
	}
	return store, func() {
		store.Close()
		os.Remove(name)
	}
}

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

	user, err := store.CreateUser("testuser", "test@example.com", "hashedpassword", "admin")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("Username = %s, want testuser", user.Username)
	}

	fetched, err := store.GetUserByUsername("testuser")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}

	if fetched.ID != user.ID {
		t.Errorf("ID = %d, want %d", fetched.ID, user.ID)
	}
}

func TestCreateMultipleUsers(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	users := []struct {
		username string
		email    string
		role     string
	}{
		{"alice", "alice@example.com", "admin"},
		{"bob", "bob@example.com", "developer"},
		{"charlie", "charlie@example.com", "viewer"},
	}

	var created []*User
	for _, u := range users {
		user, err := store.CreateUser(u.username, u.email, "hash-"+u.username, u.role)
		if err != nil {
			t.Fatalf("CreateUser(%q) error = %v", u.username, err)
		}
		created = append(created, user)
	}

	if len(created) != 3 {
		t.Fatalf("created %d users, want 3", len(created))
	}

	for i, u := range users {
		if created[i].Username != u.username {
			t.Errorf("user[%d].Username = %q, want %q", i, created[i].Username, u.username)
		}
		if created[i].Role != u.role {
			t.Errorf("user[%d].Role = %q, want %q", i, created[i].Role, u.role)
		}
	}

	for i, u := range users {
		fetched, err := store.GetUserByUsername(u.username)
		if err != nil {
			t.Fatalf("GetUserByUsername(%q) error = %v", u.username, err)
		}
		if fetched.ID != created[i].ID {
			t.Errorf("user[%d] ID mismatch: got %d, want %d", i, fetched.ID, created[i].ID)
		}
	}
}

func TestUserNotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, err := store.GetUser(9999)
	if err == nil {
		t.Fatal("GetUser(9999) expected error, got nil")
	}

	_, err = store.GetUserByUsername("nonexistent")
	if err == nil {
		t.Fatal("GetUserByUsername(\"nonexistent\") expected error, got nil")
	}
}

func TestDuplicateUsername(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, err := store.CreateUser("dupuser", "first@example.com", "hash1", "admin")
	if err != nil {
		t.Fatalf("first CreateUser() error = %v", err)
	}

	_, err = store.CreateUser("dupuser", "second@example.com", "hash2", "viewer")
	if err == nil {
		t.Fatal("CreateUser() with duplicate username expected error, got nil")
	}
}

func TestDatabaseMigration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-migrate-*.db")
	if err != nil {
		t.Fatal(err)
	}
	name := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(name)

	store, err := New(name)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tables := []string{"users", "ssh_keys", "projects", "builds", "artifacts", "workers", "plugins"}
	for _, table := range tables {
		var count int
		err := store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %q not queryable: %v", table, err)
		}
	}

	store.Close()

	store2, err := New(name)
	if err != nil {
		t.Fatalf("New() on existing db error = %v", err)
	}
	defer store2.Close()

	var count int
	err = store2.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		t.Errorf("users table not queryable after reopen: %v", err)
	}
}

func TestDatabaseClose(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-close-*.db")
	if err != nil {
		t.Fatal(err)
	}
	name := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(name)

	store, err := New(name)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	err = store.db.Ping()
	if err == nil {
		t.Fatal("db.Ping() after Close() expected error, got nil")
	}
}

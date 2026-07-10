package auth

import (
	"testing"

	"github.com/neko233-com/buildworld233/internal/store"
)

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("test")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "test" {
		t.Error("hash should not equal plaintext")
	}
	if !CheckPassword(hash, "test") {
		t.Error("CheckPassword should verify correct password")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("CheckPassword should reject wrong password")
	}
}

func TestSetupDefaultAdmin(t *testing.T) {
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer db.Close()

	err = SetupDefaultAdmin(db)
	if err != nil {
		t.Fatalf("SetupDefaultAdmin failed: %v", err)
	}

	admin, err := db.GetUserByUsername("root")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if admin.Role != "admin" {
		t.Errorf("expected admin role, got %s", admin.Role)
	}
	if !CheckPassword(admin.PasswordHash, "root") {
		t.Error("default admin password should be 'root'")
	}

	err = SetupDefaultAdmin(db)
	if err != nil {
		t.Fatalf("SetupDefaultAdmin should not fail on second call: %v", err)
	}
}

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
	if hash != "test_hashed" {
		t.Errorf("expected test_hashed, got %s", hash)
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

	err = SetupDefaultAdmin(db)
	if err != nil {
		t.Fatalf("SetupDefaultAdmin should not fail on second call: %v", err)
	}
}

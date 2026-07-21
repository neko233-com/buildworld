package auth

import (
	"testing"
	"time"
)

func TestJWTGenerate(t *testing.T) {
	jwt := NewJWT("secret")

	token, err := jwt.Generate(1, "admin", 7, 24*time.Hour)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if token == "" {
		t.Error("Token should not be empty")
	}
}

func TestJWTValidate(t *testing.T) {
	jwt := NewJWT("secret")

	token, _ := jwt.Generate(1, "admin", 7, 24*time.Hour)

	claims, err := jwt.Validate(token)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if claims.UserID != 1 {
		t.Errorf("UserID = %d, want 1", claims.UserID)
	}

	if claims.Role != "admin" {
		t.Errorf("Role = %s, want admin", claims.Role)
	}

	if claims.SessionVersion != 7 {
		t.Errorf("SessionVersion = %d, want 7", claims.SessionVersion)
	}
}

func TestJWTValidateInvalid(t *testing.T) {
	jwt := NewJWT("secret")

	_, err := jwt.Validate("invalid-token")
	if err == nil {
		t.Error("Validate() should fail on invalid token")
	}
}

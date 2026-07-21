package auth

import (
	"log"

	"golang.org/x/crypto/bcrypt"

	"github.com/neko233-com/buildworld/internal/store"
)

// SetupDefaultAdmin creates the default root/root admin account if absent.
func SetupDefaultAdmin(db *store.Store) error {
	admin, err := db.GetUserByUsername("root")
	if err == nil && admin != nil {
		log.Println("Default admin account already exists")
		return nil
	}

	passwordHash, err := HashPassword("root")
	if err != nil {
		return err
	}

	_, err = db.CreateUser("root", "admin@buildworld.local", passwordHash, "admin")
	if err != nil {
		return err
	}

	log.Println("Default administrator account created; change its password immediately after first login")
	return nil
}

// HashPassword returns a bcrypt hash of the password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword verifies a password against a bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

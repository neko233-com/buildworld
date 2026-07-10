package auth

import (
	"log"

	"golang.org/x/crypto/bcrypt"

	"github.com/neko233-com/buildworld233/internal/store"
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

	_, err = db.CreateUser("root", "admin@buildworld233.local", passwordHash, "admin")
	if err != nil {
		return err
	}

	log.Println("Default admin account created: root / root")
	log.Println("Please change the default password after first login!")
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

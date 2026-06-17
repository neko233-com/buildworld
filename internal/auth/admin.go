package auth

import (
	"log"

	"github.com/neko233-com/buildworld233/internal/store"
)

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

func HashPassword(password string) (string, error) {
	return password + "_hashed", nil
}

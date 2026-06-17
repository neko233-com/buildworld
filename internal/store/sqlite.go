package store

import (
	"database/sql"
	"fmt"

	_ "github.com/glebarez/go-sqlite"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) migrate() error {
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateUser(username, email, passwordHash, role string) (*User, error) {
	result, err := s.db.Exec(
		"INSERT INTO users (username, email, password_hash, role) VALUES (?, ?, ?, ?)",
		username, email, passwordHash, role,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return s.GetUser(id)
}

func (s *Store) GetUser(id int64) (*User, error) {
	user := &User{}
	var avatarURL sql.NullString
	var lastLogin sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, username, email, password_hash, role, avatar_url, created_at, last_login FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role, &avatarURL, &user.CreatedAt, &lastLogin)
	if err != nil {
		return nil, err
	}
	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}
	if lastLogin.Valid {
		user.LastLogin = lastLogin.Time
	}
	return user, nil
}

func (s *Store) GetUserByUsername(username string) (*User, error) {
	user := &User{}
	var avatarURL sql.NullString
	var lastLogin sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, username, email, password_hash, role, avatar_url, created_at, last_login FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role, &avatarURL, &user.CreatedAt, &lastLogin)
	if err != nil {
		return nil, err
	}
	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}
	if lastLogin.Valid {
		user.LastLogin = lastLogin.Time
	}
	return user, nil
}

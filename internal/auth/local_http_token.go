package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neko233-com/buildworld/internal/store"
)

// LocalHTTPTokenPath identifies a per-installation secret, never a shared key.
func LocalHTTPTokenPath(databasePath string) (string, error) {
	if strings.TrimSpace(databasePath) == "" || databasePath == ":memory:" || strings.HasPrefix(databasePath, "file:") {
		return "", fmt.Errorf("local HTTP token requires a persistent database file")
	}
	return filepath.Abs(databasePath + ".http-token")
}

// EnsureLocalHTTPToken enables authenticated automation without a browser setup
// step. Only the hash enters SQLite. Revoked tokens or demoted/deleted owners
// are never repaired into administrators on restart.
func EnsureLocalHTTPToken(db *store.Store, databasePath string) (path string, generated bool, err error) {
	path, err = LocalHTTPTokenPath(databasePath)
	if err != nil {
		return path, false, err
	}
	if db == nil {
		return path, false, fmt.Errorf("local HTTP token requires a database")
	}
	if info, statErr := os.Lstat(path); statErr == nil {
		if !info.Mode().IsRegular() || info.Size() > 128 {
			return path, false, fmt.Errorf("local HTTP token must be a bounded regular file")
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return path, false, fmt.Errorf("read local HTTP token: %w", readErr)
		}
		raw := strings.TrimSpace(string(body))
		if len(raw) != 67 || !strings.HasPrefix(raw, "bw_") {
			return path, false, fmt.Errorf("invalid local HTTP token file; refusing replacement")
		}
		if _, decodeErr := hex.DecodeString(raw[3:]); decodeErr != nil {
			return path, false, fmt.Errorf("invalid local HTTP token encoding")
		}
		principal, valid := ResolveAPIToken(db, raw)
		if !valid || principal.Role != "admin" || principal.Name != "local-http-api" {
			return path, false, fmt.Errorf("local HTTP token was revoked or its owner changed; refusing automatic reissue")
		}
		if info.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(path, 0o600); err != nil {
				return path, false, fmt.Errorf("protect local HTTP token: %w", err)
			}
		}
		return path, false, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return path, false, fmt.Errorf("inspect local HTTP token: %w", statErr)
	}
	owner, err := db.GetUserByUsername("root")
	if err != nil || owner == nil || owner.Role != "admin" {
		return path, false, fmt.Errorf("local HTTP token requires the existing root administrator")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return path, false, fmt.Errorf("generate local HTTP token: %w", err)
	}
	raw := "bw_" + hex.EncodeToString(random)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return path, false, fmt.Errorf("reserve local HTTP token: %w", err)
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.WriteString(raw + "\n"); err != nil {
		return path, false, fmt.Errorf("persist local HTTP token: %w", err)
	}
	if err := file.Sync(); err != nil {
		return path, false, fmt.Errorf("sync local HTTP token: %w", err)
	}
	if err := file.Close(); err != nil {
		return path, false, fmt.Errorf("close local HTTP token: %w", err)
	}
	hash := sha256.Sum256([]byte(raw))
	token, err := db.CreateAPIToken(owner.ID, "local-http-api", hex.EncodeToString(hash[:]), raw[:11], `["build:trigger"]`, nil)
	if err != nil {
		return path, false, fmt.Errorf("register local HTTP token: %w", err)
	}
	committed = true
	_ = db.CreateAuditLog(owner.ID, owner.Username, "create", "api_token", fmt.Sprint(token.ID), "local HTTP automation token provisioned", "local")
	return path, true, nil
}

package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const generatedJWTSecretSuffix = ".jwt-secret"

// LoadOrCreateJWTSecret returns the configured signing secret when one is
// present. Otherwise it persists a generated secret beside the database so
// browser and API sessions survive server restarts without requiring secret
// material in the main YAML configuration.
func LoadOrCreateJWTSecret(configured, databasePath string) (secret, persistedPath string, generated bool, err error) {
	if secret = strings.TrimSpace(configured); secret != "" {
		return secret, "", false, nil
	}

	persistedPath = jwtSecretPath(databasePath)
	if secret, err = readJWTSecret(persistedPath); err == nil {
		return secret, persistedPath, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", persistedPath, false, fmt.Errorf("read JWT secret: %w", err)
	}

	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", persistedPath, false, fmt.Errorf("generate JWT secret: %w", err)
	}
	secret = hex.EncodeToString(raw)

	dir := filepath.Dir(persistedPath)
	if dir != "." {
		if err = os.MkdirAll(dir, 0o700); err != nil {
			return "", persistedPath, false, fmt.Errorf("create JWT secret directory: %w", err)
		}
	}
	file, createErr := os.OpenFile(persistedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if createErr != nil {
		if errors.Is(createErr, os.ErrExist) {
			secret, err = readJWTSecret(persistedPath)
			return secret, persistedPath, false, err
		}
		return "", persistedPath, false, fmt.Errorf("persist JWT secret: %w", createErr)
	}
	if _, err = file.WriteString(secret + "\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(persistedPath)
		return "", persistedPath, false, fmt.Errorf("persist JWT secret: %w", err)
	}
	if err = file.Close(); err != nil {
		return "", persistedPath, false, fmt.Errorf("close JWT secret: %w", err)
	}
	return secret, persistedPath, true, nil
}

func jwtSecretPath(databasePath string) string {
	path := strings.TrimSpace(databasePath)
	if path == "" || path == ":memory:" {
		return filepath.Join(".", ".buildworld"+generatedJWTSecretSuffix)
	}
	return path + generatedJWTSecretSuffix
}

func readJWTSecret(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", fmt.Errorf("JWT secret file %s is empty", path)
	}
	return secret, nil
}

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	loginAttemptAccountLimit = 10
	loginAttemptSourceLimit  = 60
)

// ReserveLoginAttempt atomically records an attempt before password checking.
// The existing audit table keeps limits across restarts without a schema upgrade.
// Source must be the transport peer, not a caller-controlled forwarded header.
func (s *Store) ReserveLoginAttempt(username, source string) (bool, error) {
	if s == nil || s.db == nil || strings.TrimSpace(username) == "" || strings.TrimSpace(source) == "" {
		return false, fmt.Errorf("login attempt identity unavailable")
	}
	accountKey := loginAttemptKey(strings.ToLower(strings.TrimSpace(username)))
	sourceKey := loginAttemptKey(strings.ToLower(strings.TrimSpace(source)))
	result, err := s.db.Exec(`INSERT INTO audit_logs (username, action, resource_type, ip)
		SELECT ?, 'login-attempt', 'login-throttle', ?
		WHERE (SELECT COUNT(*) FROM audit_logs WHERE action = 'login-attempt'
			AND resource_type = 'login-throttle' AND username = ?
			AND created_at >= datetime('now', '-5 minutes')) < ?
		AND (SELECT COUNT(*) FROM audit_logs WHERE action = 'login-attempt'
			AND resource_type = 'login-throttle' AND ip = ?
			AND created_at >= datetime('now', '-5 minutes')) < ?`,
		accountKey, sourceKey, accountKey, loginAttemptAccountLimit, sourceKey, loginAttemptSourceLimit)
	if err != nil {
		return false, fmt.Errorf("reserve login attempt: %w", err)
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func loginAttemptKey(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

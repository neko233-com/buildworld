package store

import (
	"fmt"
	"strconv"
)

// ConsumeTOTP atomically prevents reuse of a valid code for the same user,
// including after a restart. It never stores the secret or the code itself.
func (s *Store) ConsumeTOTP(userID, counter int64) (bool, error) {
	if s == nil || s.db == nil || userID <= 0 || counter < 0 {
		return false, fmt.Errorf("invalid TOTP identity")
	}
	counterText := strconv.FormatInt(counter, 10)
	result, err := s.db.Exec(`INSERT INTO audit_logs (user_id, action, resource_type, resource_id)
		SELECT ?, 'totp-consumed', 'login', ?
		WHERE NOT EXISTS (SELECT 1 FROM audit_logs WHERE user_id = ?
		AND action = 'totp-consumed' AND resource_type = 'login' AND resource_id = ?)`,
		userID, counterText, userID, counterText)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

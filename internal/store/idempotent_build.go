package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

// ErrBuildIdempotencyConflict means a key was already used with another request or actor.
var ErrBuildIdempotencyConflict = errors.New("build idempotency key conflicts with an existing request")

// IdempotentBuildAudit records a policy change in the same transaction as its build.
type IdempotentBuildAudit struct {
	Username string
	Detail   string
	IP       string
}

// FindIdempotentBuild returns the original build and refuses a changed request.
// The key and fingerprint are hashes; raw request parameters are never audited.
func (s *Store) FindIdempotentBuild(key, fingerprint string, actorID int64) (*Build, error) {
	var buildIDText, savedFingerprint string
	var savedActorID int64
	err := s.db.QueryRow(`SELECT resource_id, detail, user_id FROM audit_logs
		WHERE action = 'build-idempotency' AND resource_type = 'build-request' AND username = ?`, key).
		Scan(&buildIDText, &savedFingerprint, &savedActorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if savedActorID != actorID || savedFingerprint != fingerprint {
		return nil, ErrBuildIdempotencyConflict
	}
	id, err := strconv.ParseInt(buildIDText, 10, 64)
	if err != nil || id <= 0 {
		return nil, errors.New("stored build idempotency receipt is invalid")
	}
	return s.GetBuild(id)
}

// CreateIdempotentBuild commits the reservation and build together. It uses
// the existing audit table so older binaries can still read the database.
func (s *Store) CreateIdempotentBuild(projectID, actorID int64, key, fingerprint, branch, commit, parameters string, approvalRequired bool, audit *IdempotentBuildAudit) (*Build, bool, error) {
	if projectID <= 0 || actorID <= 0 || key == "" || fingerprint == "" {
		return nil, false, errors.New("invalid build idempotency identity")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	reservation, err := tx.Exec(`INSERT INTO audit_logs (user_id, username, action, resource_type, resource_id, detail)
		SELECT ?, ?, 'build-idempotency', 'build-request', '', ?
		WHERE NOT EXISTS (SELECT 1 FROM audit_logs WHERE action = 'build-idempotency'
		AND resource_type = 'build-request' AND username = ?)`, actorID, key, fingerprint, key)
	if err != nil {
		return nil, false, err
	}
	created, err := reservation.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if created == 0 {
		if err = tx.Commit(); err != nil {
			return nil, false, err
		}
		build, findErr := s.FindIdempotentBuild(key, fingerprint, actorID)
		if findErr != nil || build == nil {
			if findErr == nil {
				findErr = errors.New("build idempotency receipt disappeared")
			}
			return nil, false, findErr
		}
		return build, false, nil
	}
	receiptID, err := reservation.LastInsertId()
	if err != nil {
		return nil, false, err
	}
	status := "pending"
	if approvalRequired {
		status = "pending_approval"
	}
	result, err := tx.Exec(`INSERT INTO builds (project_id, number, status, trigger, branch, commit_sha, parameters, approval_required)
		SELECT ?, COALESCE(MAX(number), 0) + 1, ?, 'manual', ?, ?, ?, ? FROM builds WHERE project_id = ?`,
		projectID, status, branch, commit, parameters, approvalRequired, projectID)
	if err != nil {
		return nil, false, err
	}
	buildID, err := result.LastInsertId()
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec("UPDATE audit_logs SET resource_id = ? WHERE id = ?", fmt.Sprint(buildID), receiptID); err != nil {
		return nil, false, err
	}
	if audit != nil {
		if _, err := tx.Exec(`INSERT INTO audit_logs (user_id, username, action, resource_type, resource_id, detail, ip)
			VALUES (?, ?, 'apply_notification_policy', 'build', ?, ?, ?)`, actorID, audit.Username, fmt.Sprint(buildID), audit.Detail, audit.IP); err != nil {
			return nil, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	build, err := s.GetBuild(buildID)
	return build, true, err
}

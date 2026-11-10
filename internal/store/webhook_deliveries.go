package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrWebhookDeliveryConflict indicates reuse of a delivery ID for another body.
var ErrWebhookDeliveryConflict = errors.New("webhook delivery ID was reused with a different body")

// WebhookTarget is one project's durable work for a provider delivery.
type WebhookTarget struct {
	DeliveryID int64
	ProjectID  int64
	BuildID    int64
	Attempt    int64
	Payload    string
}

// WebhookTargetStatus exposes operational state without payloads or secrets.
type WebhookTargetStatus struct {
	Provider    string `json:"provider"`
	DeliveryID  string `json:"delivery_id"`
	ProjectID   int64  `json:"project_id"`
	BuildID     *int64 `json:"build_id"`
	State       string `json:"state"`
	Attempts    int64  `json:"attempts"`
	NextAttempt int64  `json:"next_attempt_unix"`
	LastError   string `json:"last_error"`
}

// ListWebhookTargets returns recent target state for administrator diagnosis.
func (s *Store) ListWebhookTargets() ([]WebhookTargetStatus, error) {
	rows, err := s.db.Query(`SELECT d.provider,d.delivery_id,t.project_id,t.build_id,t.state,t.attempts,t.next_attempt,t.last_error
		FROM webhook_delivery_targets t JOIN webhook_deliveries d ON d.id=t.delivery_id
		ORDER BY d.id DESC,t.project_id LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]WebhookTargetStatus, 0)
	for rows.Next() {
		var item WebhookTargetStatus
		var buildID sql.NullInt64
		if err := rows.Scan(&item.Provider, &item.DeliveryID, &item.ProjectID, &buildID, &item.State, &item.Attempts, &item.NextAttempt, &item.LastError); err != nil {
			return nil, err
		}
		if buildID.Valid {
			item.BuildID = &buildID.Int64
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// RecordWebhookDelivery atomically snapshots the matching projects before the
// provider receives success. A replay cannot add projects or create new builds.
func (s *Store) RecordWebhookDelivery(provider, deliveryID, hash, payload string, projectIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT OR IGNORE INTO webhook_deliveries(provider,delivery_id,body_hash,payload) VALUES(?,?,?,?)`, provider, deliveryID, hash, payload)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		var previous string
		if err := tx.QueryRow(`SELECT body_hash FROM webhook_deliveries WHERE provider=? AND delivery_id=?`, provider, deliveryID).Scan(&previous); err != nil {
			return err
		}
		if previous != hash {
			return ErrWebhookDeliveryConflict
		}
		return tx.Commit()
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for _, projectID := range projectIDs {
		if _, err := tx.Exec(`INSERT INTO webhook_delivery_targets(delivery_id,project_id) VALUES(?,?)`, id, projectID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ClaimWebhookTarget acquires a bounded lease; the incrementing attempt fences
// a worker whose lease expired while it was resolving source code.
func (s *Store) ClaimWebhookTarget(now time.Time, lease time.Duration) (*WebhookTarget, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	target := &WebhookTarget{}
	var build sql.NullInt64
	err = tx.QueryRow(`SELECT t.delivery_id,t.project_id,t.build_id,t.attempts,d.payload
		FROM webhook_delivery_targets t JOIN webhook_deliveries d ON d.id=t.delivery_id
		WHERE t.state='pending' AND t.next_attempt<=? AND t.lease_until<=?
		ORDER BY t.delivery_id,t.project_id LIMIT 1`, now.Unix(), now.Unix()).Scan(&target.DeliveryID, &target.ProjectID, &build, &target.Attempt, &target.Payload)
	if err != nil {
		return nil, err
	}
	target.BuildID = build.Int64
	target.Attempt++
	_, err = tx.Exec(`UPDATE webhook_delivery_targets SET attempts=?,lease_until=? WHERE delivery_id=? AND project_id=?`, target.Attempt, now.Add(lease).Unix(), target.DeliveryID, target.ProjectID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return target, nil
}

// CreateWebhookTargetBuild commits the build and its delivery identity in one
// transaction. A crash cannot create a second build for the same target.
func (s *Store) CreateWebhookTargetBuild(t *WebhookTarget, branch, revision, parameters string, hold bool) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var attempt int64
	var existing sql.NullInt64
	if err := tx.QueryRow(`SELECT attempts,build_id FROM webhook_delivery_targets WHERE delivery_id=? AND project_id=? AND state='pending'`, t.DeliveryID, t.ProjectID).Scan(&attempt, &existing); err != nil {
		return 0, err
	}
	if attempt != t.Attempt {
		return 0, errors.New("webhook lease expired")
	}
	if existing.Valid {
		return existing.Int64, nil
	}
	var enabled bool
	if err := tx.QueryRow(`SELECT enabled FROM projects WHERE id=?`, t.ProjectID).Scan(&enabled); err != nil {
		return 0, err
	}
	if !enabled {
		return 0, errors.New("webhook project is disabled")
	}
	status := "pending"
	if hold {
		status = "pending_approval"
	}
	res, err := tx.Exec(`INSERT INTO builds(project_id,number,status,trigger,branch,commit_sha,parameters,approval_required)
		SELECT ?,COALESCE(MAX(number),0)+1,?,'webhook',?,?,?,? FROM builds WHERE project_id=?`, t.ProjectID, status, branch, revision, parameters, hold, t.ProjectID)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE webhook_delivery_targets SET build_id=? WHERE delivery_id=? AND project_id=? AND attempts=?`, id, t.DeliveryID, t.ProjectID, t.Attempt); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// FinishWebhookTarget acknowledges, skips, or schedules a retry, only while
// the caller still owns the same lease generation.
func (s *Store) FinishWebhookTarget(t *WebhookTarget, state, reason string, retryAt time.Time) error {
	res, err := s.db.Exec(`UPDATE webhook_delivery_targets SET state=?,last_error=?,next_attempt=?,lease_until=0 WHERE delivery_id=? AND project_id=? AND attempts=? AND state='pending'`, state, reason, retryAt.Unix(), t.DeliveryID, t.ProjectID, t.Attempt)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("webhook lease expired for delivery %d project %d", t.DeliveryID, t.ProjectID)
	}
	return nil
}

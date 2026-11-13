package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/neko233-com/buildworld/internal/workercontrol"
)

var ErrWorkerOperationConflict = errors.New("worker operation request conflicts with existing input")
var workerRequestPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func (s *Store) GetWorkerOperation(workerID, id string) (*workercontrol.Operation, error) {
	var raw string
	if err := s.db.QueryRow(`SELECT receipt FROM worker_operations WHERE worker_id=? AND id=?`, workerID, id).Scan(&raw); err != nil {
		return nil, err
	}
	var value workercontrol.Operation
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func addWorkerControl(tx *sql.Tx) error {
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS worker_controls (worker_id TEXT PRIMARY KEY REFERENCES workers(id) ON DELETE CASCADE, draining BOOLEAN NOT NULL DEFAULT FALSE, runtime TEXT NOT NULL DEFAULT '{}', observed_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS worker_operations (id TEXT PRIMARY KEY, worker_id TEXT NOT NULL REFERENCES workers(id) ON DELETE CASCADE, request_id TEXT NOT NULL, input TEXT NOT NULL, status TEXT NOT NULL, receipt TEXT NOT NULL, UNIQUE(worker_id, request_id))`,
		`CREATE INDEX IF NOT EXISTS worker_operations_queue ON worker_operations(worker_id, status)`,
	} {
		if _, err := tx.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

// QueueWorkerOperation records the immutable input before any worker side effect.
func (s *Store) QueueWorkerOperation(workerID string, input workercontrol.Operation) (*workercontrol.Operation, error) {
	if !workerRequestPattern.MatchString(input.RequestID) {
		return nil, errors.New("request_id must be a stable identifier of at most 128 characters")
	}
	switch input.Kind {
	case "diagnose", "drain", "resume":
		if input.Capacity != 0 || input.Version != "" || input.SHA256 != "" || input.Platform != "" {
			return nil, errors.New("unexpected operation parameters")
		}
	case "concurrency":
		if input.Capacity < 1 || input.Capacity > 64 || input.Version != "" || input.SHA256 != "" || input.Platform != "" {
			return nil, errors.New("concurrency must be between 1 and 64")
		}
	case "update":
		if input.Version == "" || len(input.SHA256) != 64 || input.Platform == "" || input.Capacity != 0 {
			return nil, errors.New("verified update metadata is required")
		}
	default:
		return nil, errors.New("unsupported worker operation")
	}
	input.ID, input.WorkerID, input.Status, input.Message, input.CreatedAt, input.UpdatedAt, input.Result = "", "", "", "", "", "", nil
	raw, _ := json.Marshal(input)
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var previousInput, previousReceipt string
	err = tx.QueryRow(`SELECT input, receipt FROM worker_operations WHERE worker_id=? AND request_id=?`, workerID, input.RequestID).Scan(&previousInput, &previousReceipt)
	if err == nil {
		if previousInput != string(raw) {
			return nil, ErrWorkerOperationConflict
		}
		var previous workercontrol.Operation
		if err = json.Unmarshal([]byte(previousReceipt), &previous); err != nil {
			return nil, err
		}
		return &previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var workerStatus string
	if err = tx.QueryRow(`SELECT status FROM workers WHERE id=?`, workerID).Scan(&workerStatus); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`INSERT INTO worker_controls(worker_id) VALUES(?) ON CONFLICT(worker_id) DO NOTHING`, workerID); err != nil {
		return nil, err
	}
	if input.Kind == "drain" || input.Kind == "update" {
		if _, err = tx.Exec(`UPDATE worker_controls SET draining=TRUE WHERE worker_id=?`, workerID); err != nil {
			return nil, err
		}
	}
	input.ID, input.WorkerID, input.Status, input.CreatedAt = uuid.NewString(), workerID, "pending", time.Now().UTC().Format(time.RFC3339Nano)
	input.UpdatedAt = input.CreatedAt
	receipt, _ := json.Marshal(input)
	if _, err = tx.Exec(`INSERT INTO worker_operations(id,worker_id,request_id,input,status,receipt) VALUES(?,?,?,?,?,?)`, input.ID, workerID, input.RequestID, string(raw), input.Status, string(receipt)); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &input, nil
}

func (s *Store) WorkerRuntime(id string) (json.RawMessage, bool, string, error) {
	var raw, observed string
	var draining bool
	err := s.db.QueryRow(`SELECT runtime,draining,observed_at FROM worker_controls WHERE worker_id=?`, id).Scan(&raw, &draining, &observed)
	if errors.Is(err, sql.ErrNoRows) {
		return json.RawMessage(`{}`), false, "", nil
	}
	return json.RawMessage(raw), draining, observed, err
}

func (s *Store) ListWorkerOperations(id string) ([]*workercontrol.Operation, error) {
	rows, err := s.db.Query(`SELECT receipt FROM worker_operations WHERE worker_id=? ORDER BY rowid DESC LIMIT 50`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]*workercontrol.Operation, 0)
	for rows.Next() {
		var raw string
		var value workercontrol.Operation
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		values = append(values, &value)
	}
	return values, rows.Err()
}

// ReportWorkerRuntime preserves controller scheduling leases; worker samples cannot reset them.
func (s *Store) ReportWorkerRuntime(id string, report workercontrol.Heartbeat) (*workercontrol.Operation, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if report.Runtime != nil {
		raw, marshalErr := json.Marshal(report.Runtime)
		if marshalErr != nil || len(raw) > workercontrol.MaxReportBytes || len(report.Runtime.Tasks) > 64 || report.Runtime.Capacity < 1 || report.Runtime.Capacity > 64 {
			return nil, errors.New("invalid worker runtime report")
		}
		_, err = tx.Exec(`INSERT INTO worker_controls(worker_id,runtime,observed_at) VALUES(?,?,?) ON CONFLICT(worker_id) DO UPDATE SET runtime=excluded.runtime,observed_at=excluded.observed_at`, id, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return nil, err
		}
	}
	if completed := report.Completion; completed != nil {
		if completed.Status != "success" && completed.Status != "failed" && completed.Status != "rolled_back" {
			return nil, errors.New("invalid completion state")
		}
		if len(completed.Message) > 2048 || len(completed.Result) > workercontrol.MaxReportBytes {
			return nil, errors.New("completion exceeds limit")
		}
		var raw, status string
		if err = tx.QueryRow(`SELECT receipt,status FROM worker_operations WHERE id=? AND worker_id=?`, completed.ID, id).Scan(&raw, &status); err != nil {
			return nil, err
		}
		if status == "pending" || status == "running" {
			var op workercontrol.Operation
			if err = json.Unmarshal([]byte(raw), &op); err != nil {
				return nil, err
			}
			op.Status, op.Message, op.Result, op.UpdatedAt = completed.Status, completed.Message, completed.Result, time.Now().UTC().Format(time.RFC3339Nano)
			rawBytes, _ := json.Marshal(op)
			if _, err = tx.Exec(`UPDATE worker_operations SET status=?,receipt=? WHERE id=?`, op.Status, string(rawBytes), op.ID); err != nil {
				return nil, err
			}
			if op.Status == "success" && op.Kind == "resume" {
				if _, err = tx.Exec(`UPDATE worker_controls SET draining=FALSE WHERE worker_id=?`, id); err != nil {
					return nil, err
				}
			}
			if op.Status == "success" && op.Kind == "concurrency" {
				if _, err = tx.Exec(`UPDATE workers SET max_concurrent_builds=? WHERE id=?`, op.Capacity, id); err != nil {
					return nil, err
				}
			}
		}
	}
	var raw string
	err = tx.QueryRow(`SELECT receipt FROM worker_operations WHERE worker_id=? AND status IN ('pending','running') ORDER BY rowid LIMIT 1`, id).Scan(&raw)
	var next *workercontrol.Operation
	if err == nil {
		var op workercontrol.Operation
		if err = json.Unmarshal([]byte(raw), &op); err != nil {
			return nil, err
		}
		op.Status, op.UpdatedAt = "running", time.Now().UTC().Format(time.RFC3339Nano)
		data, _ := json.Marshal(op)
		if _, err = tx.Exec(`UPDATE worker_operations SET status='running',receipt=? WHERE id=?`, string(data), op.ID); err != nil {
			return nil, err
		}
		next = &op
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit worker report: %w", err)
	}
	return next, nil
}

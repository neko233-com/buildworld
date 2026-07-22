package store

import (
	"database/sql"
	"fmt"
	"time"
)

// ProjectChange is the SCM revision recorded for one project build. BuildWorld
// does not currently persist per-commit author or message data, so this model
// exposes only fields backed by the builds table.
type ProjectChange struct {
	BuildID     int64      `json:"build_id"`
	BuildNumber int        `json:"build_number"`
	Status      string     `json:"status"`
	CommitSHA   string     `json:"commit_sha"`
	Branch      string     `json:"branch"`
	Timestamp   *time.Time `json:"timestamp"`
}

// ListProjectChanges returns recorded SCM revisions newest build first. Builds
// without a commit SHA are not changes and are intentionally omitted.
func (s *Store) ListProjectChanges(projectID int64) ([]*ProjectChange, error) {
	rows, err := s.db.Query(`
		SELECT id, number, status, commit_sha, COALESCE(branch, ''),
			started_at, finished_at
		FROM builds
		WHERE project_id = ? AND TRIM(COALESCE(commit_sha, '')) <> ''
		ORDER BY number DESC, id DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project changes: %w", err)
	}
	defer rows.Close()

	changes := make([]*ProjectChange, 0)
	for rows.Next() {
		change := &ProjectChange{}
		var startedAt, finishedAt sql.NullTime
		if err := rows.Scan(
			&change.BuildID,
			&change.BuildNumber,
			&change.Status,
			&change.CommitSHA,
			&change.Branch,
			&startedAt,
			&finishedAt,
		); err != nil {
			return nil, fmt.Errorf("scan project change: %w", err)
		}
		if startedAt.Valid {
			value := startedAt.Time
			change.Timestamp = &value
		} else if finishedAt.Valid {
			value := finishedAt.Time
			change.Timestamp = &value
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project changes: %w", err)
	}
	return changes, nil
}

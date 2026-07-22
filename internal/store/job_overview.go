package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// BuildOverview contains only fields needed by Jenkins-style project lists.
// Build logs, parameters, and source metadata are intentionally excluded.
type BuildOverview struct {
	ID         int64      `json:"id"`
	Number     int        `json:"number"`
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMs *int64     `json:"duration_ms"`
}

type ProjectBuildOverview struct {
	ProjectID      int64          `json:"project_id"`
	Latest         *BuildOverview `json:"latest"`
	LastSuccess    *BuildOverview `json:"last_success"`
	LastFailure    *BuildOverview `json:"last_failure"`
	RecentStatuses []string       `json:"recent_statuses"`
}

type nullableBuildOverview struct {
	id         sql.NullInt64
	number     sql.NullInt64
	status     sql.NullString
	startedAt  sql.NullTime
	finishedAt sql.NullTime
	durationMs sql.NullInt64
}

func (b nullableBuildOverview) value() *BuildOverview {
	if !b.id.Valid {
		return nil
	}
	result := &BuildOverview{
		ID:     b.id.Int64,
		Number: int(b.number.Int64),
		Status: b.status.String,
	}
	if b.startedAt.Valid {
		startedAt := b.startedAt.Time
		result.StartedAt = &startedAt
	}
	if b.finishedAt.Valid {
		finishedAt := b.finishedAt.Time
		result.FinishedAt = &finishedAt
	}
	if b.durationMs.Valid {
		durationMs := b.durationMs.Int64
		result.DurationMs = &durationMs
	}
	return result
}

func buildOverviewScanTargets(build *nullableBuildOverview) []any {
	return []any{
		&build.id,
		&build.number,
		&build.status,
		&build.startedAt,
		&build.finishedAt,
		&build.durationMs,
	}
}

// ListProjectBuildOverviews returns bounded build data for each project.
// Correlated lookups use the existing (project_id, number) unique index and
// keep old success/failure results exact even when they fall outside history.
func (s *Store) ListProjectBuildOverviews() ([]*ProjectBuildOverview, error) {
	rows, err := s.db.Query(`
		SELECT
			p.id,
			latest.id, latest.number, latest.status,
			latest.started_at, latest.finished_at, latest.duration_ms,
			last_success.id, last_success.number, last_success.status,
			last_success.started_at, last_success.finished_at, last_success.duration_ms,
			last_failure.id, last_failure.number, last_failure.status,
			last_failure.started_at, last_failure.finished_at, last_failure.duration_ms,
			COALESCE((
				SELECT json_group_array(recent.status)
				FROM (
					SELECT status
					FROM builds
					WHERE project_id = p.id
					ORDER BY number DESC, id DESC
					LIMIT 5
				) AS recent
			), '[]')
		FROM projects AS p
		LEFT JOIN builds AS latest ON latest.id = (
			SELECT id
			FROM builds
			WHERE project_id = p.id
			ORDER BY number DESC, id DESC
			LIMIT 1
		)
		LEFT JOIN builds AS last_success ON last_success.id = (
			SELECT id
			FROM builds
			WHERE project_id = p.id AND status = 'success'
			ORDER BY number DESC, id DESC
			LIMIT 1
		)
		LEFT JOIN builds AS last_failure ON last_failure.id = (
			SELECT id
			FROM builds
			WHERE project_id = p.id AND status = 'failed'
			ORDER BY number DESC, id DESC
			LIMIT 1
		)
		ORDER BY p.favorite DESC, p.quick_access DESC, p.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list project build overviews: %w", err)
	}
	defer rows.Close()

	overviews := make([]*ProjectBuildOverview, 0)
	for rows.Next() {
		var projectID int64
		var latest, lastSuccess, lastFailure nullableBuildOverview
		var recentStatusesJSON string
		targets := []any{&projectID}
		targets = append(targets, buildOverviewScanTargets(&latest)...)
		targets = append(targets, buildOverviewScanTargets(&lastSuccess)...)
		targets = append(targets, buildOverviewScanTargets(&lastFailure)...)
		targets = append(targets, &recentStatusesJSON)
		if err := rows.Scan(targets...); err != nil {
			return nil, fmt.Errorf("scan project build overview: %w", err)
		}

		recentStatuses := make([]string, 0, 5)
		if err := json.Unmarshal([]byte(recentStatusesJSON), &recentStatuses); err != nil {
			return nil, fmt.Errorf("decode recent build statuses for project %d: %w", projectID, err)
		}
		overviews = append(overviews, &ProjectBuildOverview{
			ProjectID:      projectID,
			Latest:         latest.value(),
			LastSuccess:    lastSuccess.value(),
			LastFailure:    lastFailure.value(),
			RecentStatuses: recentStatuses,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project build overviews: %w", err)
	}
	return overviews, nil
}

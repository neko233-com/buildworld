package store

import (
	"database/sql"
	"time"
)

func addProjectBuildOnStartup(tx *sql.Tx) error {
	exists, err := sqliteColumnExists(tx, "projects", "build_on_startup")
	if err != nil || exists {
		return err
	}
	_, err = tx.Exec(`ALTER TABLE projects ADD COLUMN build_on_startup BOOLEAN NOT NULL DEFAULT FALSE`)
	return err
}

func (s *Store) SetProjectBuildOnStartup(id int64, enabled bool) error {
	result, err := s.db.Exec(`UPDATE projects SET build_on_startup=?, updated_at=? WHERE id=?`, enabled, time.Now(), id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

const startupEligibility = `SELECT enabled AND build_on_startup AND NOT EXISTS (
	SELECT 1 FROM builds WHERE project_id=projects.id AND status IN ('pending','running','pending_approval','queued')
) FROM projects WHERE id=?`

func (s *Store) StartupBuildEligible(id int64) (bool, error) {
	var eligible bool
	err := s.db.QueryRow(startupEligibility, id).Scan(&eligible)
	return eligible, err
}

// CreateStartupBuild checks eligibility and allocates a build number atomically.
// Existing pending, running or approval work already provides startup recovery.
func (s *Store) CreateStartupBuild(id int64, branch, parameters string, approvalRequired bool) (*Build, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var eligible bool
	err = tx.QueryRow(startupEligibility, id).Scan(&eligible)
	if err != nil || !eligible {
		return nil, err
	}
	result, err := tx.Exec(`INSERT INTO builds (project_id, number, status, trigger, branch, commit_sha, parameters, approval_required)
		SELECT ?, COALESCE(MAX(number),0)+1, CASE WHEN ? THEN 'pending_approval' ELSE 'pending' END, 'startup', ?, '', ?, ? FROM builds WHERE project_id=?`, id, approvalRequired, branch, parameters, approvalRequired, id)
	if err != nil {
		return nil, err
	}
	buildID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetBuild(buildID)
}

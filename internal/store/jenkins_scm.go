package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// JenkinsSCMCheckout is the immutable changelog captured when a build checks out SCM.
type JenkinsSCMCheckout struct {
	BuildID        int64
	ProjectID      int64
	BuildNumber    int
	RepositoryHash string
	Branch         string
	Revision       string
	BaseRevision   string
	HistoryKnown   bool
	ChangesJSON    string
}

// JenkinsSCMCheckoutForBuild returns nil when this build has not checked out SCM.
func (s *Store) JenkinsSCMCheckoutForBuild(buildID int64) (*JenkinsSCMCheckout, error) {
	record := &JenkinsSCMCheckout{}
	err := s.db.QueryRow(`SELECT build_id, project_id, build_number, repository_hash, branch, revision, base_revision, history_known, changes_json
		FROM jenkins_scm_checkouts WHERE build_id=?`, buildID).Scan(&record.BuildID, &record.ProjectID, &record.BuildNumber, &record.RepositoryHash, &record.Branch, &record.Revision, &record.BaseRevision, &record.HistoryKnown, &record.ChangesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load Jenkins SCM checkout: %w", err)
	}
	return record, nil
}

// PreviousJenkinsSCMRevision finds the most recent earlier build that actually checked out the same repository and branch.
func (s *Store) PreviousJenkinsSCMRevision(projectID int64, buildNumber int, repositoryHash, branch string) (string, bool, error) {
	var revision string
	err := s.db.QueryRow(`SELECT revision FROM jenkins_scm_checkouts
		WHERE project_id=? AND build_number<? AND repository_hash=? AND branch=?
		ORDER BY build_number DESC LIMIT 1`, projectID, buildNumber, repositoryHash, branch).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		var previousExists bool
		err = s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM builds WHERE project_id=? AND number<? AND
			(branch=? OR branch=? OR ((branch IS NULL OR branch='') AND ?=(SELECT default_branch FROM projects WHERE id=?))))`,
			projectID, buildNumber, branch, "refs/heads/"+branch, branch, projectID).Scan(&previousExists)
		if err != nil {
			return "", false, fmt.Errorf("inspect prior build history: %w", err)
		}
		return "", !previousExists, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load previous Jenkins SCM checkout: %w", err)
	}
	return revision, true, nil
}

// SaveJenkinsSCMCheckout persists the first checkout for a build without replacing its snapshot on recovery.
func (s *Store) SaveJenkinsSCMCheckout(record JenkinsSCMCheckout) error {
	result, err := s.db.Exec(`INSERT INTO jenkins_scm_checkouts
		(build_id, project_id, build_number, repository_hash, branch, revision, base_revision, history_known, changes_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(build_id) DO NOTHING`,
		record.BuildID, record.ProjectID, record.BuildNumber, record.RepositoryHash, record.Branch, record.Revision, record.BaseRevision, record.HistoryKnown, record.ChangesJSON)
	if err != nil {
		return fmt.Errorf("save Jenkins SCM checkout: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect saved Jenkins SCM checkout: %w", err)
	}
	if inserted == 0 {
		existing, err := s.JenkinsSCMCheckoutForBuild(record.BuildID)
		if err != nil {
			return err
		}
		if existing == nil || *existing != record {
			return fmt.Errorf("conflicting Jenkins SCM checkout for build %d", record.BuildID)
		}
	}
	return nil
}

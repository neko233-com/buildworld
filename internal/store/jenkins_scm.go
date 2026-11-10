package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

// JenkinsSCMCheckoutsForHead returns snapshots of the same SCM head in earlier
// builds, optionally including the current build after its checkout completed.
func (s *Store) JenkinsSCMCheckoutsForHead(projectID int64, buildNumber int, repositoryHash, branch string, includeCurrent bool, maxBytes int) ([]JenkinsSCMCheckout, error) {
	rows, err := s.db.Query(`SELECT build_id, project_id, build_number, repository_hash, branch, revision, base_revision, history_known, changes_json
		FROM jenkins_scm_checkouts WHERE project_id=? AND repository_hash=? AND branch=? AND (build_number<? OR (? AND build_number=?)) ORDER BY build_number DESC`,
		projectID, repositoryHash, branch, buildNumber, includeCurrent, buildNumber)
	if err != nil {
		return nil, fmt.Errorf("load Jenkins SCM head history: %w", err)
	}
	defer rows.Close()
	var records []JenkinsSCMCheckout
	bytes := 0
	for rows.Next() {
		var record JenkinsSCMCheckout
		if err := rows.Scan(&record.BuildID, &record.ProjectID, &record.BuildNumber, &record.RepositoryHash, &record.Branch, &record.Revision, &record.BaseRevision, &record.HistoryKnown, &record.ChangesJSON); err != nil {
			return nil, fmt.Errorf("scan Jenkins SCM head history: %w", err)
		}
		bytes += len(record.ChangesJSON)
		if maxBytes < 1 || bytes > maxBytes {
			return nil, fmt.Errorf("Jenkins SCM head history exceeds %d bytes", maxBytes)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Jenkins SCM head history: %w", err)
	}
	return records, nil
}

// JenkinsSCMHeadHasUnrecordedBuilds reports completed builds whose checkout
// state predates durable SCM snapshots. Active builds have no changes until
// checkout finishes and are therefore not treated as legacy history.
func (s *Store) JenkinsSCMHeadHasUnrecordedBuilds(projectID int64, buildNumber int, branch string) (bool, error) {
	var missing bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM builds b LEFT JOIN jenkins_scm_checkouts c ON c.build_id=b.id
		WHERE b.project_id=? AND b.branch=? AND b.number<? AND b.status IN ('success','failed','cancelled','canceled','error') AND c.build_id IS NULL)`,
		projectID, branch, buildNumber).Scan(&missing)
	if err != nil {
		return false, fmt.Errorf("inspect Jenkins SCM head history: %w", err)
	}
	return missing, nil
}

// PreviousJenkinsSCMRevision finds the most recent earlier build that actually checked out the same repository and branch.
func (s *Store) PreviousJenkinsSCMRevision(projectID int64, buildNumber int, repositoryHash, branch string) (string, bool, error) {
	var revision string
	err := s.db.QueryRow(`SELECT revision FROM jenkins_scm_checkouts
		WHERE project_id=? AND build_number<? AND repository_hash=? AND branch=? AND revision<>''
		ORDER BY build_number DESC LIMIT 1`, projectID, buildNumber, repositoryHash, branch).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		var previousExists bool
		err = s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM builds b WHERE b.project_id=? AND b.number<? AND
			(b.branch=? OR b.branch=? OR ((b.branch IS NULL OR b.branch='') AND ?=(SELECT default_branch FROM projects WHERE id=?)))
			AND (?=0 OR b.status NOT IN ('pending','running','pending_approval','queued'))
			AND NOT EXISTS(SELECT 1 FROM jenkins_scm_checkouts c WHERE c.build_id=b.id AND c.repository_hash=? AND c.branch=? AND c.revision=''))`,
			projectID, buildNumber, branch, "refs/heads/"+branch, branch, projectID, strings.HasPrefix(branch, "refs/pull/") || strings.HasPrefix(branch, "refs/merge-requests/"), repositoryHash, branch).Scan(&previousExists)
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

// SaveJenkinsSCMCheckout makes an actual checkout immutable across recovery.
// An explicitly recorded no-checkout state may be replaced by the first checkout.
func (s *Store) SaveJenkinsSCMCheckout(record JenkinsSCMCheckout) error {
	result, err := s.db.Exec(`INSERT INTO jenkins_scm_checkouts
		(build_id, project_id, build_number, repository_hash, branch, revision, base_revision, history_known, changes_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(build_id) DO UPDATE SET
		revision=excluded.revision, base_revision=excluded.base_revision, history_known=excluded.history_known, changes_json=excluded.changes_json
		WHERE jenkins_scm_checkouts.revision='' AND excluded.revision<>''
		AND jenkins_scm_checkouts.project_id=excluded.project_id AND jenkins_scm_checkouts.build_number=excluded.build_number
		AND jenkins_scm_checkouts.repository_hash=excluded.repository_hash AND jenkins_scm_checkouts.branch=excluded.branch`,
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

package store

import (
	"strings"
)

// ListBuildNeighborhood returns the direct parents and children of the supplied
// builds across every durable relationship currently understood by the store.
// Log bodies are omitted so graph traversal stays bounded and lightweight.
func (s *Store) ListBuildNeighborhood(buildIDs []int64, limit int) ([]*Build, error) {
	if len(buildIDs) == 0 {
		return []*Build{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(buildIDs)), ",")
	const fields = `b.id, b.project_id, b.number, b.status, b.trigger, b.branch,
		b.commit_sha, b.parameters, b.wait_dependency_on, b.retried_from,
		b.pinned, '' AS log, b.started_at, b.finished_at, b.duration_ms,
		b.approval_required, b.approved_by, b.approved_at, b.timeout_sec,
		b.test_result_id`
	query := `SELECT ` + fields + ` FROM builds b WHERE
		b.retried_from IN (` + placeholders + `) OR
		b.wait_dependency_on IN (` + placeholders + `) OR
		b.id IN (SELECT retried_from FROM builds WHERE id IN (` + placeholders + `)) OR
		b.id IN (SELECT wait_dependency_on FROM builds WHERE id IN (` + placeholders + `))
		ORDER BY b.id ASC LIMIT ?`
	args := make([]any, 0, len(buildIDs)*4+1)
	for range 4 {
		for _, id := range buildIDs {
			args = append(args, id)
		}
	}
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	builds, err := scanBuilds(rows)
	if builds == nil {
		builds = []*Build{}
	}
	return builds, err
}

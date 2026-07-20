package store

import (
	"fmt"
	"strings"
)

const BuildSearchResultVersion = 1

type BuildSearch struct {
	Query     string
	ProjectID *int64
	Statuses  []string
	Trigger   string
	Branch    string
	Pinned    *bool
	Limit     int
	Offset    int
}

type BuildSearchResult struct {
	Version int      `json:"version"`
	Items   []*Build `json:"items"`
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func buildSearchWhere(search BuildSearch) (string, []any) {
	var clauses []string
	var args []any
	if search.ProjectID != nil {
		clauses = append(clauses, "b.project_id = ?")
		args = append(args, *search.ProjectID)
	}
	if len(search.Statuses) > 0 {
		placeholders := make([]string, 0, len(search.Statuses))
		for _, status := range search.Statuses {
			status = strings.TrimSpace(status)
			if status == "" {
				continue
			}
			placeholders = append(placeholders, "?")
			args = append(args, status)
		}
		if len(placeholders) > 0 {
			clauses = append(clauses, "b.status IN ("+strings.Join(placeholders, ",")+")")
		}
	}
	if trigger := strings.TrimSpace(search.Trigger); trigger != "" {
		clauses = append(clauses, "LOWER(b.trigger) = LOWER(?)")
		args = append(args, trigger)
	}
	if branch := strings.TrimSpace(search.Branch); branch != "" {
		clauses = append(clauses, "LOWER(COALESCE(b.branch, '')) = LOWER(?)")
		args = append(args, branch)
	}
	if search.Pinned != nil {
		clauses = append(clauses, "b.pinned = ?")
		args = append(args, *search.Pinned)
	}
	if query := strings.TrimSpace(search.Query); query != "" {
		pattern := "%" + strings.ToLower(escapeLike(strings.TrimPrefix(query, "#"))) + "%"
		clauses = append(clauses, `(LOWER(p.name) LIKE ? ESCAPE '\' OR
			LOWER(COALESCE(b.branch, '')) LIKE ? ESCAPE '\' OR
			LOWER(COALESCE(b.commit_sha, '')) LIKE ? ESCAPE '\' OR
			LOWER(b.status) LIKE ? ESCAPE '\' OR
			CAST(b.number AS TEXT) LIKE ? ESCAPE '\')`)
		for range 5 {
			args = append(args, pattern)
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// SearchBuilds is the lightweight history-query path. It intentionally omits
// durable log bodies; callers can fetch a single build or standalone log on
// demand. This keeps history payloads bounded as installations grow.
func (s *Store) SearchBuilds(search BuildSearch) (*BuildSearchResult, error) {
	if search.Limit <= 0 {
		search.Limit = 25
	}
	if search.Limit > 100 {
		search.Limit = 100
	}
	if search.Offset < 0 {
		search.Offset = 0
	}
	where, args := buildSearchWhere(search)

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM builds b JOIN projects p ON p.id = b.project_id"+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count builds: %w", err)
	}

	const fields = `b.id, b.project_id, b.number, b.status, b.trigger, b.branch,
		b.commit_sha, b.parameters, b.wait_dependency_on, b.retried_from,
		b.pinned, '' AS log, b.started_at, b.finished_at, b.duration_ms,
		b.approval_required, b.approved_by, b.approved_at, b.timeout_sec,
		b.test_result_id`
	queryArgs := append(append([]any(nil), args...), search.Limit, search.Offset)
	rows, err := s.db.Query(
		"SELECT "+fields+" FROM builds b JOIN projects p ON p.id = b.project_id"+
			where+" ORDER BY b.id DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return nil, fmt.Errorf("search builds: %w", err)
	}
	defer rows.Close()
	items, err := scanBuilds(rows)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []*Build{}
	}
	return &BuildSearchResult{
		Version: BuildSearchResultVersion,
		Items:   items,
		Total:   total,
		Limit:   search.Limit,
		Offset:  search.Offset,
	}, nil
}

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/neko233-com/buildworld/internal/releasemeta"
)

const ReleaseResultMarker = "::buildworld:release-result "

type BuildReleaseResult struct {
	Channel     string `json:"channel"`
	Environment string `json:"environment"`
	Action      string `json:"action"`
	Version     string `json:"version"`
	Branch      string `json:"branch,omitempty"`
	Component   string `json:"component,omitempty"`
}

const releaseResultsSchema = `CREATE TABLE IF NOT EXISTS build_release_results (
	build_id INTEGER NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
	component TEXT NOT NULL DEFAULT '',
	channel TEXT NOT NULL,
	environment TEXT NOT NULL,
	action TEXT NOT NULL,
	version TEXT NOT NULL,
	branch TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (build_id, component)
)`

func addBuildReleaseResults(tx *sql.Tx) error {
	_, err := tx.Exec(releaseResultsSchema)
	return err
}

func validReleaseResultText(value string, maxLength int, required bool) bool {
	return (!required || strings.TrimSpace(value) != "") && len(value) <= maxLength && strings.IndexFunc(value, unicode.IsControl) < 0
}

// CaptureReleaseResults recognizes complete marker lines before log retention.
// Result metadata is accepted only for the requested target/action and survives
// trimmed logs. No result is treated as successful until the build finishes.
func (s *Store) CaptureReleaseResults(buildID int64, content string) error {
	if !strings.Contains(content, ReleaseResultMarker) {
		return nil
	}
	var parametersJSON sql.NullString
	if err := s.db.QueryRow("SELECT parameters FROM builds WHERE id=?", buildID).Scan(&parametersJSON); err != nil {
		return err
	}
	var parameters map[string]interface{}
	if json.Unmarshal([]byte(parametersJSON.String), &parameters) != nil {
		return nil
	}
	for _, line := range strings.Split(content, "\n") {
		index := strings.Index(line, ReleaseResultMarker)
		if index < 0 {
			continue
		}
		var result BuildReleaseResult
		if json.Unmarshal([]byte(line[index+len(ReleaseResultMarker):]), &result) != nil {
			continue
		}
		if !validReleaseResultText(result.Channel, 64, true) || !releasemeta.ValidEnvironment(result.Environment) || !validReleaseResultText(result.Version, 128, true) || !validReleaseResultText(result.Branch, 255, false) || !validReleaseResultText(result.Component, 64, false) {
			continue
		}
		if result.Action != "restart-version" && result.Action != "publish-latest" {
			continue
		}
		if parameters["RELEASE_CHANNEL"] != result.Channel || parameters["RELEASE_ENVIRONMENT"] != result.Environment || parameters["RELEASE_ACTION"] != result.Action {
			continue
		}
		if result.Action == "restart-version" && parameters["RELEASE_VERSION"] != result.Version {
			continue
		}
		_, err := s.db.Exec(`INSERT INTO build_release_results (build_id, component, channel, environment, action, version, branch)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(build_id, component) DO NOTHING`, buildID, result.Component, result.Channel, result.Environment, result.Action, result.Version, result.Branch)
		if err != nil {
			return fmt.Errorf("persist release result: %w", err)
		}
	}
	return nil
}

// AttachReleaseResults batches metadata reads and never loads build log bodies.
func (s *Store) AttachReleaseResults(builds []*Build) error {
	if len(builds) == 0 {
		return nil
	}
	byID := make(map[int64]*Build, len(builds))
	args := make([]any, 0, len(builds))
	placeholders := make([]string, 0, len(builds))
	for _, build := range builds {
		build.ReleaseResults = nil
		byID[build.ID] = build
		args, placeholders = append(args, build.ID), append(placeholders, "?")
	}
	rows, err := s.db.Query("SELECT build_id, channel, environment, action, version, branch, component FROM build_release_results WHERE build_id IN ("+strings.Join(placeholders, ",")+") ORDER BY component", args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var result BuildReleaseResult
		if err := rows.Scan(&id, &result.Channel, &result.Environment, &result.Action, &result.Version, &result.Branch, &result.Component); err != nil {
			return err
		}
		byID[id].ReleaseResults = append(byID[id].ReleaseResults, &result)
	}
	return rows.Err()
}

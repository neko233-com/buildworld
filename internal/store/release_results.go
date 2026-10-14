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
	SourceCommit     string `json:"sourceCommit,omitempty"`
	ConfigCommit     string `json:"configCommit,omitempty"`
	ConfigSnapshotID string `json:"configSnapshotId,omitempty"`
	ConfigVersion    string `json:"configVersion,omitempty"`
	SDKVersion       string `json:"sdkVersion,omitempty"`
	GatewayVersion   string `json:"gatewayVersion,omitempty"`
	Channel          string `json:"channel"`
	Environment      string `json:"environment"`
	Action           string `json:"action"`
	Version          string `json:"version"`
	Branch           string `json:"branch,omitempty"`
	Component        string `json:"component,omitempty"`
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

func addBuildReleaseIdentities(tx *sql.Tx) error {
	for _, column := range []string{"source_commit", "config_commit", "config_snapshot_id", "config_version", "sdk_version", "gateway_version"} {
		if _, err := tx.Exec("ALTER TABLE build_release_results ADD COLUMN " + column + " TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	_, err := tx.Exec("UPDATE build_release_results SET branch=substr(branch, 7) WHERE branch LIKE 'codex/release/%'")
	return err
}

func validReleaseIdentityHash(value string, length int) bool {
	return value == "" || len(value) == length && strings.Trim(value, "0123456789abcdef") == ""
}

func validReleaseIdentity(result BuildReleaseResult) bool {
	return validReleaseIdentityHash(result.SourceCommit, 40) && validReleaseIdentityHash(result.ConfigCommit, 40) && validReleaseIdentityHash(result.ConfigSnapshotID, 64) && validReleaseResultText(result.ConfigVersion, 128, false) && validReleaseResultText(result.SDKVersion, 128, false) && validReleaseResultText(result.GatewayVersion, 128, false) && (result.ConfigCommit == "") == (result.ConfigSnapshotID == "")
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
		if !validReleaseIdentity(result) || (result.Action != "restart-version" && result.Action != "publish-latest") {
			continue
		}
		if parameters["RELEASE_CHANNEL"] != result.Channel || parameters["RELEASE_ENVIRONMENT"] != result.Environment || parameters["RELEASE_ACTION"] != result.Action {
			continue
		}
		if result.Action == "restart-version" && parameters["RELEASE_VERSION"] != result.Version {
			continue
		}
		if selected, ok := parameters["RELEASE_BRANCH"].(string); ok && selected != "" && selected != result.Branch {
			continue
		}
		_, err := s.db.Exec(`INSERT INTO build_release_results (build_id, component, channel, environment, action, version, branch, source_commit, config_commit, config_snapshot_id, config_version, sdk_version, gateway_version)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(build_id, component) DO NOTHING`, buildID, result.Component, result.Channel, result.Environment, result.Action, result.Version, result.Branch, result.SourceCommit, result.ConfigCommit, result.ConfigSnapshotID, result.ConfigVersion, result.SDKVersion, result.GatewayVersion)
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
	rows, err := s.db.Query("SELECT build_id, channel, environment, action, version, branch, component, source_commit, config_commit, config_snapshot_id, config_version, sdk_version, gateway_version FROM build_release_results WHERE build_id IN ("+strings.Join(placeholders, ",")+") ORDER BY component", args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var result BuildReleaseResult
		if err := rows.Scan(&id, &result.Channel, &result.Environment, &result.Action, &result.Version, &result.Branch, &result.Component, &result.SourceCommit, &result.ConfigCommit, &result.ConfigSnapshotID, &result.ConfigVersion, &result.SDKVersion, &result.GatewayVersion); err != nil {
			return err
		}
		byID[id].ReleaseResults = append(byID[id].ReleaseResults, &result)
	}
	return rows.Err()
}

// ListReleaseRecords returns completed successful publications without build logs or secret parameters.
func (s *Store) ListReleaseRecords(projectID int64) ([]*BuildReleaseResult, error) {
	rows, err := s.db.Query(`SELECT r.channel, r.environment, r.action, r.version, r.branch, r.component, r.source_commit, r.config_commit, r.config_snapshot_id, r.config_version, r.sdk_version, r.gateway_version
		FROM build_release_results r JOIN builds b ON b.id=r.build_id WHERE b.project_id=? AND b.status='success' AND julianday(b.finished_at) IS NOT NULL AND r.branch<>'' ORDER BY b.finished_at DESC, b.id DESC LIMIT 512`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*BuildReleaseResult, 0)
	seen := make(map[string]bool)
	for rows.Next() {
		var record BuildReleaseResult
		if err := rows.Scan(&record.Channel, &record.Environment, &record.Action, &record.Version, &record.Branch, &record.Component, &record.SourceCommit, &record.ConfigCommit, &record.ConfigSnapshotID, &record.ConfigVersion, &record.SDKVersion, &record.GatewayVersion); err != nil {
			return nil, err
		}
		if seen[record.Branch] {
			continue
		}
		if !releasemeta.ValidRecord(record.Branch, record.Channel, record.Environment, record.Component, record.Version) {
			continue
		}
		seen[record.Branch] = true
		result = append(result, &record)
	}
	return result, rows.Err()
}

// GetProjectBuildReleaseStatus reads bounded metadata for a project-scoped
// release observer. Log bodies and parameters are never loaded into memory.
func (s *Store) GetProjectBuildReleaseStatus(projectID, buildID int64) (*Build, error) {
	build := &Build{}
	var started, finished sql.NullTime
	var duration sql.NullInt64
	err := s.db.QueryRow(`SELECT id, project_id, number, status, started_at, finished_at, duration_ms
		FROM builds WHERE project_id=? AND id=?`, projectID, buildID).Scan(
		&build.ID, &build.ProjectID, &build.Number, &build.Status, &started, &finished, &duration)
	if err != nil {
		return nil, err
	}
	if started.Valid {
		build.StartedAt = &started.Time
	}
	if finished.Valid {
		build.FinishedAt = &finished.Time
	}
	if duration.Valid {
		build.DurationMs = &duration.Int64
	}
	return build, nil
}

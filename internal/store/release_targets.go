package store

import (
	"fmt"

	"github.com/neko233-com/buildworld/internal/releasemeta"
)

// These names are public release metadata, never secret parameters.
const ReleaseChannelParameter = "RELEASE_CHANNEL"
const ReleaseEnvironmentParameter = "RELEASE_ENVIRONMENT"

type ReleaseTargetOverview struct {
	Channel     string         `json:"channel"`
	Environment string         `json:"environment"`
	Latest      *BuildOverview `json:"latest"`
	LastSuccess *BuildOverview `json:"last_success"`
}

// ListReleaseTargetOverviews reads the entire retained history, without loading
// logs or returning other parameters. Old builds with no complete target remain
// unclassified. Successful publication time is the completed pipeline time.
func (s *Store) ListReleaseTargetOverviews(projectID int64) ([]*ReleaseTargetOverview, error) {
	rows, err := s.db.Query(`
		WITH parsed AS (
			SELECT *, CASE WHEN json_valid(parameters) THEN parameters ELSE '{}' END AS target_parameters
			FROM builds WHERE project_id = ?
		), targeted AS (
			SELECT *, trim(json_extract(target_parameters, '$.RELEASE_CHANNEL')) AS channel,
				json_extract(target_parameters, '$.RELEASE_ENVIRONMENT') AS environment
			FROM parsed
			WHERE json_type(target_parameters, '$.RELEASE_CHANNEL') = 'text'
				AND json_type(target_parameters, '$.RELEASE_ENVIRONMENT') = 'text'
		), targets AS (
			SELECT DISTINCT channel, environment FROM targeted WHERE channel <> ''
				AND length(environment) BETWEEN 1 AND 32
				AND substr(environment, 1, 1) GLOB '[a-z]'
				AND environment NOT GLOB '*[^a-z0-9-]*'
		)
		SELECT targets.channel, targets.environment,
			latest.id, latest.number, latest.status, latest.started_at, latest.finished_at, latest.duration_ms,
			COALESCE(json_extract(latest.parameters, '$.RELEASE_ACTION'), ''), COALESCE(json_extract(latest.parameters, '$.RELEASE_VERSION'), ''),
			success.id, success.number, success.status, success.started_at, success.finished_at, success.duration_ms,
			COALESCE(json_extract(success.parameters, '$.RELEASE_ACTION'), ''), COALESCE(json_extract(success.parameters, '$.RELEASE_VERSION'), '')
		FROM targets
		LEFT JOIN builds AS latest ON latest.id = (
			SELECT id FROM targeted WHERE channel = targets.channel AND environment = targets.environment
			ORDER BY number DESC, id DESC LIMIT 1
		)
		LEFT JOIN builds AS success ON success.id = (
			SELECT id FROM targeted WHERE channel = targets.channel AND environment = targets.environment
				AND status = 'success' AND julianday(finished_at) IS NOT NULL
			ORDER BY julianday(finished_at) DESC, id DESC LIMIT 1
		)
		ORDER BY targets.channel, targets.environment`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list release targets: %w", err)
	}
	defer rows.Close()
	result := make([]*ReleaseTargetOverview, 0)
	builds := make([]*Build, 0)
	for rows.Next() {
		var target ReleaseTargetOverview
		var latest, success nullableBuildOverview
		fields := []any{&target.Channel, &target.Environment}
		fields = append(fields, buildOverviewScanTargets(&latest)...)
		var latestAction, latestVersion, successAction, successVersion string
		fields = append(fields, &latestAction, &latestVersion)
		fields = append(fields, buildOverviewScanTargets(&success)...)
		fields = append(fields, &successAction, &successVersion)
		if err := rows.Scan(fields...); err != nil {
			return nil, fmt.Errorf("scan release target: %w", err)
		}
		if !releasemeta.ValidEnvironment(target.Environment) {
			continue
		}
		target.Latest, target.LastSuccess = latest.value(), success.value()
		if target.Latest != nil {
			target.Latest.Action, target.Latest.Version = latestAction, latestVersion
			builds = append(builds, &Build{ID: target.Latest.ID})
		}
		if target.LastSuccess != nil {
			target.LastSuccess.Action, target.LastSuccess.Version = successAction, successVersion
			builds = append(builds, &Build{ID: target.LastSuccess.ID})
		}
		result = append(result, &target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := s.AttachReleaseResults(builds); err != nil {
		return nil, err
	}
	byID := make(map[int64]*Build, len(builds))
	for _, build := range builds {
		byID[build.ID] = build
	}
	for _, target := range result {
		for _, build := range []*BuildOverview{target.Latest, target.LastSuccess} {
			if build == nil {
				continue
			}
			build.ReleaseResults = byID[build.ID].ReleaseResults
			if len(build.ReleaseResults) == 1 {
				metadata := build.ReleaseResults[0]
				build.Action, build.Version, build.Branch = metadata.Action, metadata.Version, metadata.Branch
			}
		}
	}
	return result, nil
}

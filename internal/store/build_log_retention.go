package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	BuildLogRetentionDaysSetting   = "build_log_retention_days"
	DefaultBuildLogRetentionDays   = 30
	MinBuildLogRetentionDays       = 1
	MaxBuildLogRetentionDays       = 3650
	BuildLogRetentionSweepInterval = 6 * time.Hour
	BuildLogExpiredMarker          = "[buildworld] This retained log expired and was removed by the configured retention policy.\n"
)

// ParseBuildLogRetentionDays validates the administrator-facing retention
// setting. A minimum of one day keeps the control useful without allowing a
// mistaken zero value to erase logs immediately.
func ParseBuildLogRetentionDays(value string) (int, error) {
	days, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || days < MinBuildLogRetentionDays || days > MaxBuildLogRetentionDays {
		return 0, fmt.Errorf("build_log_retention_days must be between %d and %d days", MinBuildLogRetentionDays, MaxBuildLogRetentionDays)
	}
	return days, nil
}

// GetBuildLogRetentionDays reads the persisted administrator setting and falls
// back to the safe default when an older installation has no setting yet.
func (s *Store) GetBuildLogRetentionDays() (int, error) {
	values, err := s.ListEnvVars("system", nil)
	if err != nil {
		return 0, err
	}
	for _, value := range values {
		if value.Name != BuildLogRetentionDaysSetting {
			continue
		}
		days, parseErr := ParseBuildLogRetentionDays(value.Value)
		if parseErr != nil {
			return DefaultBuildLogRetentionDays, nil
		}
		return days, nil
	}
	return DefaultBuildLogRetentionDays, nil
}

// PurgeExpiredBuildLogs removes only durable log payloads. Build metadata,
// artifacts, and pinned builds remain available for history and audit work.
// The terminal-status guard prevents cleanup from touching active or pending
// builds whose timestamps are not a reliable retention boundary.
func (s *Store) PurgeExpiredBuildLogs(retentionDays int, now time.Time) (int64, error) {
	if retentionDays < MinBuildLogRetentionDays || retentionDays > MaxBuildLogRetentionDays {
		return 0, fmt.Errorf("build log retention must be between %d and %d days", MinBuildLogRetentionDays, MaxBuildLogRetentionDays)
	}
	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	result, err := s.db.Exec(`
		UPDATE builds
		SET log = ?
		WHERE status IN ('success', 'failed', 'cancelled')
		  AND pinned = 0
		  AND COALESCE(finished_at, started_at) IS NOT NULL
		  AND COALESCE(finished_at, started_at) < ?
		  AND COALESCE(log, '') <> ''
		  AND log <> ?`, BuildLogExpiredMarker, cutoff, BuildLogExpiredMarker)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

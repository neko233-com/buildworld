package engine

import (
	"slices"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

// StatisticsService 聚合构建统计
type StatisticsService struct {
	store *store.Store
}

func NewStatisticsService(s *store.Store) *StatisticsService {
	return &StatisticsService{store: s}
}

// RecordBuildCompletion 在构建完成时更新当日统计（upsert）。
func (ss *StatisticsService) RecordBuildCompletion(build *store.Build) error {
	date := time.Now().Format("2006-01-02")
	if build.FinishedAt != nil {
		date = build.FinishedAt.Format("2006-01-02")
	}

	switch build.Status {
	case "success", "failed":
	default:
		return nil
	}

	// Rebuild the materialized singleton row from the builds table instead
	// of incrementing it. Completion callbacks can be delivered more than once
	// after retries or restarts, so an additive update is not idempotent.
	current, err := ss.store.GetProjectBuildStats(build.ProjectID, 2)
	if err != nil {
		return err
	}
	for _, daily := range current {
		if daily.Date == date {
			_, err = ss.store.CreateBuildStat(
				build.ProjectID,
				date,
				daily.TotalBuilds,
				daily.SuccessCount,
				daily.FailedCount,
				daily.AvgDuration,
			)
			return err
		}
	}
	return nil
}

// GetProjectStats 返回最近 days 天的项目统计。
func (ss *StatisticsService) GetProjectStats(projectID int64, days int) ([]store.BuildStat, error) {
	stats, err := ss.store.GetProjectBuildStats(projectID, days)
	if err != nil {
		return nil, err
	}
	out := make([]store.BuildStat, 0, len(stats))
	for _, s := range stats {
		out = append(out, *s)
	}
	return out, nil
}

// GetDashboardStats 返回全局统计概览。
func (ss *StatisticsService) GetDashboardStats() (map[string]interface{}, error) {
	stats, err := ss.store.GetDashboardStats()
	if err != nil {
		return nil, err
	}
	projects, _ := ss.store.ListProjects()
	builds, _ := ss.store.ListBuilds(1000)

	var running, success, failed int
	var durationTotal int64
	var durationCount int64
	for _, b := range builds {
		switch b.Status {
		case "running":
			running++
		case "success":
			success++
		case "failed":
			failed++
		}
		if b.DurationMs != nil {
			durationTotal += *b.DurationMs
			durationCount++
		}
	}

	var recentStats []store.BuildStat
	for _, s := range stats {
		recentStats = append(recentStats, *s)
	}
	trend := slices.Clone(recentStats)
	slices.Reverse(trend)

	finished := success + failed
	successRate, failureRate := 0.0, 0.0
	if finished > 0 {
		successRate = float64(success) * 100 / float64(finished)
		failureRate = float64(failed) * 100 / float64(finished)
	}
	var avgDuration int64
	if durationCount > 0 {
		avgDuration = durationTotal / durationCount
	}

	return map[string]interface{}{
		"projects":        len(projects),
		"total_builds":    len(builds),
		"running":         running,
		"success_total":   success,
		"failed_total":    failed,
		"success_rate":    successRate,
		"failure_rate":    failureRate,
		"avg_duration_ms": avgDuration,
		"trend":           trend,
		"recent_stats":    recentStats,
	}, nil
}

package engine

import (
	"time"

	"github.com/neko233-com/buildworld233/internal/store"
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

	var total, success, failed int
	switch build.Status {
	case "success":
		total, success = 1, 1
	case "failed":
		total, failed = 1, 1
	default:
		return nil
	}

	var duration int64
	if build.DurationMs != nil {
		duration = *build.DurationMs
	}

	// 读已有行，做增量更新（按 project+date 唯一）。
	existing, _ := ss.store.GetProjectBuildStats(build.ProjectID, 1)
	var prev *store.BuildStat
	for _, s := range existing {
		if s.Date == date {
			prev = s
			break
		}
	}
	if prev != nil {
		total += prev.TotalBuilds
		success += prev.SuccessCount
		failed += prev.FailedCount
		// 简单平均：把已有 avg 加上本次并除以 2，避免维护 sum 字段。
		duration = (prev.AvgDuration + duration) / 2
	}

	_, err := ss.store.CreateBuildStat(build.ProjectID, date, total, success, failed, duration)
	return err
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
	for _, b := range builds {
		switch b.Status {
		case "running":
			running++
		case "success":
			success++
		case "failed":
			failed++
		}
	}

	var recentStats []store.BuildStat
	for _, s := range stats {
		recentStats = append(recentStats, *s)
	}
	return map[string]interface{}{
		"projects":      len(projects),
		"total_builds":  len(builds),
		"running":       running,
		"success_total": success,
		"failed_total":  failed,
		"recent_stats":  recentStats,
	}, nil
}

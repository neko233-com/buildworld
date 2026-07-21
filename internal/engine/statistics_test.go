package engine

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

func newStatisticsTestStore(t *testing.T) *store.Store {
	t.Helper()
	data, err := store.New(filepath.Join(t.TempDir(), "statistics.db"))
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	t.Cleanup(func() { _ = data.Close() })
	return data
}

func createFinishedStatisticsBuild(t *testing.T, data *store.Store, projectID int64, number int, status string, duration int64) *store.Build {
	t.Helper()
	build, err := data.CreateBuild(projectID, number, "manual", "main", "", "{}", nil, nil)
	if err != nil {
		t.Fatalf("CreateBuild(%d) error = %v", number, err)
	}
	if err := data.StartBuild(build.ID); err != nil {
		t.Fatalf("StartBuild(%d) error = %v", number, err)
	}
	if err := data.FinishBuild(build.ID, status, duration); err != nil {
		t.Fatalf("FinishBuild(%d) error = %v", number, err)
	}
	build, err = data.GetBuild(build.ID)
	if err != nil {
		t.Fatalf("GetBuild(%d) error = %v", number, err)
	}
	return build
}

func TestRecordBuildCompletionIsIdempotent(t *testing.T) {
	data := newStatisticsTestStore(t)
	project, err := data.CreateProject("idempotent-stats", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build := createFinishedStatisticsBuild(t, data, project.ID, 1, "success", 1250)
	service := NewStatisticsService(data)

	if err := service.RecordBuildCompletion(build); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordBuildCompletion(build); err != nil {
		t.Fatal(err)
	}

	var total, success, failed int
	err = data.DB().QueryRow(
		"SELECT total_builds, success_count, failed_count FROM build_stats WHERE project_id=? AND date=?",
		project.ID,
		build.FinishedAt.Format("2006-01-02"),
	).Scan(&total, &success, &failed)
	if err != nil {
		t.Fatalf("read materialized statistics error = %v", err)
	}
	if total != 1 || success != 1 || failed != 0 {
		t.Fatalf("materialized statistics = %d/%d/%d, want 1/1/0", total, success, failed)
	}
}

func TestBigScreenTrendUsesLiveBuilds(t *testing.T) {
	data := newStatisticsTestStore(t)
	project, err := data.CreateProject("live-dashboard", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	createFinishedStatisticsBuild(t, data, project.ID, 1, "success", 1000)
	createFinishedStatisticsBuild(t, data, project.ID, 2, "failed", 2000)
	cancelled, err := data.CreateBuild(project.ID, 3, "manual", "main", "", "{}", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CancelBuild(cancelled.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuildStat(project.ID, time.Now().Format("2006-01-02"), 50, 40, 10, 9000); err != nil {
		t.Fatal(err)
	}

	dashboard, err := NewBigScreenService(data).GetBigScreenData()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.Summary.TotalBuilds != 3 {
		t.Fatalf("summary total = %d, want 3 live builds", dashboard.Summary.TotalBuilds)
	}
	if len(dashboard.TrendData) != 1 {
		t.Fatalf("trend = %#v, want one live day", dashboard.TrendData)
	}
	if point := dashboard.TrendData[0]; point.Success != 1 || point.Failed != 1 || point.Running != 0 {
		t.Fatalf("trend point = %#v, want live 1/1/0", point)
	}
	if len(dashboard.ProjectStats) != 1 || dashboard.ProjectStats[0].TotalBuilds != 3 {
		t.Fatalf("project stats = %#v, want all three live builds", dashboard.ProjectStats)
	}
}

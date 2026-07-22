package store

import (
	"reflect"
	"testing"
	"time"
)

func TestListProjectBuildOverviews(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()

	empty, err := data.CreateProject("empty", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	history, err := data.CreateProject("history", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateProject("other", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	historyStatuses := []string{
		"success",
		"failed",
		"cancelled",
		"pending",
		"running",
		"cancelled",
		"pending",
		"running",
	}
	var historyBuilds []*Build
	for index, status := range historyStatuses {
		build, err := data.CreateBuild(history.ID, index+1, "manual", "main", "", "", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := data.UpdateBuildStatus(build.ID, status); err != nil {
			t.Fatal(err)
		}
		historyBuilds = append(historyBuilds, build)
	}
	startedAt := time.Date(2026, time.July, 22, 10, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(37 * time.Second)
	if _, err := data.DB().Exec(
		"UPDATE builds SET started_at = ?, finished_at = ?, duration_ms = ? WHERE id = ?",
		startedAt, finishedAt, int64(37_000), historyBuilds[7].ID,
	); err != nil {
		t.Fatal(err)
	}

	otherSuccess, err := data.CreateBuild(other.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.UpdateBuildStatus(otherSuccess.ID, "success"); err != nil {
		t.Fatal(err)
	}
	otherFailure, err := data.CreateBuild(other.ID, 2, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.UpdateBuildStatus(otherFailure.ID, "failed"); err != nil {
		t.Fatal(err)
	}

	overviews, err := data.ListProjectBuildOverviews()
	if err != nil {
		t.Fatal(err)
	}
	if len(overviews) != 3 {
		t.Fatalf("overview count = %d, want 3", len(overviews))
	}
	byProject := make(map[int64]*ProjectBuildOverview, len(overviews))
	for _, overview := range overviews {
		byProject[overview.ProjectID] = overview
	}

	emptyOverview := byProject[empty.ID]
	if emptyOverview == nil {
		t.Fatal("missing empty project overview")
	}
	if emptyOverview.Latest != nil || emptyOverview.LastSuccess != nil || emptyOverview.LastFailure != nil {
		t.Fatalf("empty project has builds: %#v", emptyOverview)
	}
	if emptyOverview.RecentStatuses == nil || len(emptyOverview.RecentStatuses) != 0 {
		t.Fatalf("empty recent statuses = %#v, want non-nil empty slice", emptyOverview.RecentStatuses)
	}

	historyOverview := byProject[history.ID]
	if historyOverview == nil {
		t.Fatal("missing history project overview")
	}
	if historyOverview.Latest == nil || historyOverview.Latest.ID != historyBuilds[7].ID || historyOverview.Latest.Number != 8 {
		t.Fatalf("latest = %#v, want build 8", historyOverview.Latest)
	}
	if historyOverview.LastSuccess == nil || historyOverview.LastSuccess.ID != historyBuilds[0].ID {
		t.Fatalf("last success = %#v, want build 1", historyOverview.LastSuccess)
	}
	if historyOverview.LastFailure == nil || historyOverview.LastFailure.ID != historyBuilds[1].ID {
		t.Fatalf("last failure = %#v, want build 2", historyOverview.LastFailure)
	}
	wantRecent := []string{"running", "pending", "cancelled", "running", "pending"}
	if !reflect.DeepEqual(historyOverview.RecentStatuses, wantRecent) {
		t.Fatalf("recent statuses = %#v, want %#v", historyOverview.RecentStatuses, wantRecent)
	}
	if historyOverview.Latest.StartedAt == nil || !historyOverview.Latest.StartedAt.Equal(startedAt) {
		t.Fatalf("latest started_at = %v, want %v", historyOverview.Latest.StartedAt, startedAt)
	}
	if historyOverview.Latest.FinishedAt == nil || !historyOverview.Latest.FinishedAt.Equal(finishedAt) {
		t.Fatalf("latest finished_at = %v, want %v", historyOverview.Latest.FinishedAt, finishedAt)
	}
	if historyOverview.Latest.DurationMs == nil || *historyOverview.Latest.DurationMs != 37_000 {
		t.Fatalf("latest duration_ms = %v, want 37000", historyOverview.Latest.DurationMs)
	}

	otherOverview := byProject[other.ID]
	if otherOverview == nil || otherOverview.Latest == nil || otherOverview.Latest.ID != otherFailure.ID {
		t.Fatalf("other latest = %#v, want build 2", otherOverview)
	}
	if otherOverview.LastSuccess == nil || otherOverview.LastSuccess.ID != otherSuccess.ID {
		t.Fatalf("other last success = %#v, want build 1", otherOverview.LastSuccess)
	}
	if otherOverview.LastFailure == nil || otherOverview.LastFailure.ID != otherFailure.ID {
		t.Fatalf("other last failure = %#v, want build 2", otherOverview.LastFailure)
	}
}

package store

import (
	"reflect"
	"testing"
	"time"
)

func TestListProjectChangesReturnsRecordedRevisionsNewestBuildFirst(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()

	project, err := data.CreateProject("changes", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateProject("other", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	first, err := data.CreateBuild(project.ID, 1, "manual", "main", "abc001", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuild(project.ID, 2, "manual", "main", "", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	third, err := data.CreateBuild(project.ID, 3, "webhook", "release", "abc003", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuild(other.ID, 1, "manual", "main", "other001", "", nil, nil); err != nil {
		t.Fatal(err)
	}

	firstStarted := time.Date(2026, time.July, 20, 8, 30, 0, 0, time.UTC)
	thirdFinished := time.Date(2026, time.July, 22, 12, 4, 50, 0, time.UTC)
	if _, err := data.DB().Exec(
		"UPDATE builds SET status = 'success', started_at = ? WHERE id = ?",
		firstStarted, first.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := data.DB().Exec(
		"UPDATE builds SET status = 'failed', finished_at = ? WHERE id = ?",
		thirdFinished, third.ID,
	); err != nil {
		t.Fatal(err)
	}

	changes, err := data.ListProjectChanges(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("change count = %d, want 2: %#v", len(changes), changes)
	}
	if got := []int{changes[0].BuildNumber, changes[1].BuildNumber}; !reflect.DeepEqual(got, []int{3, 1}) {
		t.Fatalf("build order = %v, want [3 1]", got)
	}
	if changes[0].BuildID != third.ID || changes[0].Status != "failed" || changes[0].CommitSHA != "abc003" || changes[0].Branch != "release" {
		t.Fatalf("newest change = %#v", changes[0])
	}
	if changes[0].Timestamp == nil || !changes[0].Timestamp.Equal(thirdFinished) {
		t.Fatalf("newest timestamp = %v, want %v", changes[0].Timestamp, thirdFinished)
	}
	if changes[1].BuildID != first.ID || changes[1].Timestamp == nil || !changes[1].Timestamp.Equal(firstStarted) {
		t.Fatalf("oldest change = %#v", changes[1])
	}
}

func TestListProjectChangesReturnsNonNilEmptySlice(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()

	project, err := data.CreateProject("empty-changes", "", "", "git", "main", `{}`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := data.ListProjectChanges(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if changes == nil || len(changes) != 0 {
		t.Fatalf("changes = %#v, want non-nil empty slice", changes)
	}
}

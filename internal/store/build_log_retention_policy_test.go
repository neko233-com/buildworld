package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPurgeExpiredBuildLogsClearsOnlyEligiblePayloads(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "retention-policy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()

	project, err := data.CreateProject("retention-policy", "", "", "git", "main", "{}", 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 24, 12, 0, 0, 0, time.UTC)
	oldTime := now.Add(-31 * 24 * time.Hour)
	recentTime := now.Add(-29 * 24 * time.Hour)

	old, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.FinishBuild(old.ID, "success", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := data.DB().Exec("UPDATE builds SET finished_at=? WHERE id=?", oldTime, old.ID); err != nil {
		t.Fatal(err)
	}
	if err := data.SetBuildLog(old.ID, "old log"); err != nil {
		t.Fatal(err)
	}

	pinned, err := data.CreateBuild(project.ID, 2, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.FinishBuild(pinned.ID, "failed", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := data.DB().Exec("UPDATE builds SET finished_at=? WHERE id=?", oldTime, pinned.ID); err != nil {
		t.Fatal(err)
	}
	if err := data.PinBuild(pinned.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := data.SetBuildLog(pinned.ID, "pinned log"); err != nil {
		t.Fatal(err)
	}

	recent, err := data.CreateBuild(project.ID, 3, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.FinishBuild(recent.ID, "cancelled", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := data.DB().Exec("UPDATE builds SET finished_at=? WHERE id=?", recentTime, recent.ID); err != nil {
		t.Fatal(err)
	}
	if err := data.SetBuildLog(recent.ID, "recent log"); err != nil {
		t.Fatal(err)
	}

	running, err := data.CreateBuild(project.ID, 4, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.StartBuild(running.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := data.DB().Exec("UPDATE builds SET started_at=? WHERE id=?", oldTime, running.ID); err != nil {
		t.Fatal(err)
	}
	if err := data.SetBuildLog(running.ID, "running log"); err != nil {
		t.Fatal(err)
	}

	purged, err := data.PurgeExpiredBuildLogs(30, now)
	if err != nil {
		t.Fatal(err)
	}
	if purged != 1 {
		t.Fatalf("purged = %d, want 1", purged)
	}

	oldAfter, _ := data.GetBuild(old.ID)
	pinnedAfter, _ := data.GetBuild(pinned.ID)
	recentAfter, _ := data.GetBuild(recent.ID)
	runningAfter, _ := data.GetBuild(running.ID)
	if oldAfter.Log != BuildLogExpiredMarker || oldAfter.Number != 1 || oldAfter.Status != "success" {
		t.Fatalf("old build after purge = %#v", oldAfter)
	}
	if pinnedAfter.Log != "pinned log" || recentAfter.Log != "recent log" || runningAfter.Log != "running log" {
		t.Fatalf("protected logs changed: pinned=%q recent=%q running=%q", pinnedAfter.Log, recentAfter.Log, runningAfter.Log)
	}
}

func TestBuildLogRetentionSettingDefaultsAndPersists(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "retention-setting.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()

	days, err := data.GetBuildLogRetentionDays()
	if err != nil || days != DefaultBuildLogRetentionDays {
		t.Fatalf("default retention = %d, err = %v", days, err)
	}
	if err := data.SetEnvVar("system", nil, BuildLogRetentionDaysSetting, "7", false, ""); err != nil {
		t.Fatal(err)
	}
	days, err = data.GetBuildLogRetentionDays()
	if err != nil || days != 7 {
		t.Fatalf("persisted retention = %d, err = %v", days, err)
	}
	if _, err := ParseBuildLogRetentionDays("0"); err == nil {
		t.Fatal("zero retention should be rejected")
	}
}

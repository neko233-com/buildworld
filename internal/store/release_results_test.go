package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCatalogKeepsFrozenConfigAndOnlyCompletedSuccess(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("catalog", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	record := BuildReleaseResult{Channel: "wechat", Environment: "release", Action: "publish-latest", Component: "game-server", Version: "0.1.234", Branch: "release/wechat/release/game-server/0.1.234", SourceCommit: strings.Repeat("a", 40), ConfigCommit: strings.Repeat("b", 40), ConfigSnapshotID: strings.Repeat("c", 64), ConfigVersion: "0.0.731", SDKVersion: "0.0.68", GatewayVersion: "0.1.42"}
	for index, status := range []string{"success", "failed", "running"} {
		build, err := data.CreateBuild(project.ID, index+1, "manual", "main", "", `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"release","RELEASE_ACTION":"publish-latest"}`, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(record)
		if err := data.CaptureReleaseResults(build.ID, ReleaseResultMarker+string(payload)); err != nil {
			t.Fatal(err)
		}
		if status != "running" {
			if err := data.FinishBuild(build.ID, status, 0); err != nil {
				t.Fatal(err)
			}
		}
		// The newer failed/running builds must never replace this catalog identity.
		record.ConfigCommit = strings.Repeat("d", 40)
	}
	records, err := data.ListReleaseRecords(project.ID)
	if err != nil || len(records) != 1 {
		t.Fatal("catalog included unfinished or failed records", err)
	}
	if records[0].ConfigCommit != strings.Repeat("b", 40) || records[0].ConfigVersion != "0.0.731" || records[0].SourceCommit != strings.Repeat("a", 40) {
		t.Fatal("catalog used a mutable latest config")
	}
	other, err := data.CreateProject("other", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if records, err := data.ListReleaseRecords(other.ID); err != nil || len(records) != 0 {
		t.Fatal("project catalog leaked into another project")
	}
}

func TestReleaseResultMatchesRequestedTargetAndSurvivesLogRetention(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "results.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("result", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"audit","RELEASE_ACTION":"publish-latest","CONFIRM_PUBLISH_LATEST":true}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []string{
		`{"channel":"taptap","environment":"audit","action":"publish-latest","version":"0.1.234","branch":"release/wechat"}`,
		`{"channel":"wechat","environment":"release","action":"publish-latest","version":"0.1.234","branch":"release/wechat"}`,
		`{"channel":"wechat","environment":"audit","action":"restart-version","version":"0.1.234","branch":"release/wechat"}`,
		`{"channel":"wechat","environment":"audit","action":"publish-latest","version":"","branch":"release/wechat"}`,
	}
	for _, payload := range invalid {
		if err := data.AppendBuildLog(build.ID, ReleaseResultMarker+payload+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := data.AttachReleaseResults([]*Build{build}); err != nil {
		t.Fatal(err)
	}
	if len(build.ReleaseResults) != 0 {
		t.Fatal("invalid marker was persisted")
	}
	valid := `{"channel":"wechat","environment":"audit","action":"publish-latest","version":"0.1.234","branch":"release/wechat","component":"game-server"}`
	if err := data.AppendBuildLog(build.ID, "[10:00:00] [Deploy] "+ReleaseResultMarker+valid+"\n"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := data.AppendBuildLog(build.ID, strings.Repeat("padding\n", 35_000)); err != nil {
			t.Fatal(err)
		}
	}
	if err := data.AttachReleaseResults([]*Build{build}); err != nil {
		t.Fatal(err)
	}
	if len(build.ReleaseResults) != 1 || build.ReleaseResults[0].Branch != "release/wechat" {
		t.Fatalf("result = %+v", build.ReleaseResults)
	}
	before, err := data.ListReleaseTargetOverviews(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before[0].LastSuccess != nil {
		t.Fatal("marker alone became successful publication")
	}
	if err := data.FinishBuild(build.ID, "success", 0); err != nil {
		t.Fatal(err)
	}
	targets, err := data.ListReleaseTargetOverviews(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if targets[0].LastSuccess.Version != "0.1.234" || targets[0].LastSuccess.Branch != "release/wechat" {
		t.Fatalf("successful metadata = %+v", targets[0].LastSuccess)
	}
	stored, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.Log, ReleaseResultMarker) {
		t.Fatal("marker unexpectedly retained in large log")
	}
}

func TestReleaseResultMigrationPreservesExistingBuilds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	data, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject("legacy", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec("DROP TABLE build_release_results"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec("DELETE FROM schema_migrations WHERE version>=14"); err != nil {
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	data, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	if _, err := data.GetBuild(build.ID); err != nil {
		t.Fatal("existing build lost during upgrade", err)
	}
	if err := data.AttachReleaseResults([]*Build{build}); err != nil {
		t.Fatal(err)
	}
	if len(build.ReleaseResults) != 0 {
		t.Fatal("legacy build assigned a release result")
	}
}

func TestReleaseResultRejectsUnsafeEnvironmentEvenWhenRequestMatches(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "unsafe.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("unsafe", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"../release","RELEASE_ACTION":"publish-latest"}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.CaptureReleaseResults(build.ID, ReleaseResultMarker+`{"channel":"wechat","environment":"../release","action":"publish-latest","version":"0.1.234"}`); err != nil {
		t.Fatal(err)
	}
	if err := data.AttachReleaseResults([]*Build{build}); err != nil {
		t.Fatal(err)
	}
	if len(build.ReleaseResults) != 0 {
		t.Fatal("unsafe marker environment accepted")
	}
}

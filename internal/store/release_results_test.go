package store

import (
	"path/filepath"
	"strings"
	"testing"
)

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
	if _, err := data.db.Exec("DELETE FROM schema_migrations WHERE version=14"); err != nil {
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

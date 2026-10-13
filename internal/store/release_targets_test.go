package store

import (
	"path/filepath"
	"testing"
)

func TestReleaseTargetsUseExplicitSnapshotsAndCompletedSuccessAcrossFullHistory(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "targets.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("release", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	create := func(number int, parameters, status, finished string) *Build {
		t.Helper()
		build, err := data.CreateBuild(project.ID, number, "manual", "main", "", parameters, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if finished == "" {
			_, err = data.db.Exec("UPDATE builds SET status=? WHERE id=?", status, build.ID)
		} else {
			_, err = data.db.Exec("UPDATE builds SET status=?, finished_at=? WHERE id=?", status, finished, build.ID)
		}
		if err != nil {
			t.Fatal(err)
		}
		return build
	}
	oldSuccess := create(1, `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"test","TOKEN":"secret"}`, "success", "2026-10-10T02:00:00Z")
	create(2, `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"release"}`, "success", "2026-10-11T02:00:00Z")
	create(3, `{"RELEASE_CHANNEL":"taptap","RELEASE_ENVIRONMENT":"test"}`, "failed", "2026-10-11T03:00:00Z")
	create(4, `{"RELEASE_CHANNEL":"taptap","RELEASE_ENVIRONMENT":"release"}`, "success", "")
	for number := 5; number < 108; number++ {
		create(number, "", "success", "2026-10-12T01:00:00Z")
	}
	latest := create(108, `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"test"}`, "failed", "2026-10-12T02:00:00Z")
	for i, parameters := range []string{`{"RELEASE_CHANNEL":"wechat"}`, `{broken`, `{"RELEASE_CHANNEL":123,"RELEASE_ENVIRONMENT":"release"}`, `{"RELEASE_CHANNEL":" ","RELEASE_ENVIRONMENT":"release"}`, `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"../release"}`, `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":" audit "}`, `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"Audit"}`, `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"test\u0000invalid"}`} {
		create(109+i, parameters, "success", "2026-10-13T01:00:00Z")
	}
	targets, err := data.ListReleaseTargetOverviews(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 4 {
		t.Fatalf("targets = %+v, want four explicit targets", targets)
	}
	for _, target := range targets {
		if target.Channel == "wechat" && target.Environment == "test" {
			if target.Latest.ID != latest.ID || target.LastSuccess == nil || target.LastSuccess.ID != oldSuccess.ID || target.LastSuccess.FinishedAt == nil {
				t.Fatalf("wechat test = %+v, want retained old success and latest failure", target)
			}
		} else if target.Channel == "taptap" && target.LastSuccess != nil {
			t.Fatalf("incomplete or failed build counted as success: %+v", target)
		}
	}
	search, err := data.SearchBuilds(BuildSearch{ProjectID: &project.ID, ReleaseChannel: "wechat", ReleaseEnvironment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if search.Total != 2 || len(search.Items) != 2 || search.Items[0].ID != latest.ID {
		t.Fatalf("filtered history = %+v", search)
	}
	create(120, `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"qa-17"}`, "success", "2026-10-13T02:00:00Z")
	search, err = data.SearchBuilds(BuildSearch{ProjectID: &project.ID, ReleaseEnvironment: "qa-17"})
	if err != nil || search.Total != 1 {
		t.Fatalf("N environment search = %+v, %v", search, err)
	}
	if _, err := data.SearchBuilds(BuildSearch{ReleaseEnvironment: "../release"}); err == nil {
		t.Fatal("unsafe environment search accepted")
	}
}

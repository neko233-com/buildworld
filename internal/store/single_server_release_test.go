package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSingleServerReleaseReceiptIsScopedAndNotCataloguedAsWholeEnvironment(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "release.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("release", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"test","RELEASE_ACTION":"restart-version","RELEASE_VERSION":"0.1.2","TARGET_SERVER_ID":"10002"}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	record := BuildReleaseResult{Channel: "wechat", Environment: "test", Action: "restart-version", Version: "0.1.2", Component: "game-server", TargetServerIDs: []string{"10001"}}
	payload, _ := json.Marshal(record)
	if err := data.CaptureReleaseResults(build.ID, ReleaseResultMarker+string(payload)); err != nil {
		t.Fatal(err)
	}
	if err := data.AttachReleaseResults([]*Build{build}); err != nil || len(build.ReleaseResults) != 0 {
		t.Fatal("mismatched target was accepted", err)
	}
	record.TargetServerIDs = []string{"10002"}
	payload, _ = json.Marshal(record)
	if err := data.CaptureReleaseResults(build.ID, ReleaseResultMarker+string(payload)); err != nil {
		t.Fatal(err)
	}
	if err := data.CaptureReleaseResults(build.ID, ReleaseResultMarker+string(payload)); err != nil {
		t.Fatal(err)
	}
	if err := data.FinishBuild(build.ID, "success", 0); err != nil {
		t.Fatal(err)
	}
	if err := data.AttachReleaseResults([]*Build{build}); err != nil || len(build.ReleaseResults) != 1 || build.ReleaseResults[0].TargetServerIDs[0] != "10002" {
		t.Fatalf("single target receipt missing: %+v err=%v", build.ReleaseResults, err)
	}
	if records, err := data.ListReleaseRecords(project.ID); err != nil || len(records) != 0 {
		t.Fatal("single target was exposed as a whole-environment release", err)
	}
	if overviews, err := data.ListReleaseTargetOverviews(project.ID); err != nil || len(overviews) != 0 {
		t.Fatal("single target was exposed as the whole-environment status", err)
	}
}

func TestRequestedReleaseServerIDRejectsUnexpectedTypes(t *testing.T) {
	for _, input := range []any{map[string]any{"id": 10001}, []any{10001}, "../10001", "-1", "01", float64(1.5)} {
		if _, ok := requestedReleaseServerID(input); ok {
			t.Fatalf("invalid target accepted: %#v", input)
		}
	}
}

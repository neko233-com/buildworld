package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiServerReleaseReceiptIsScopedAndExcludedFromEnvironmentCatalog(t *testing.T) {
	data, err := New(filepath.Join(t.TempDir(), "release.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("release", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"test","RELEASE_ACTION":"restart-version","RELEASE_VERSION":"0.1.2","TARGET_SERVER_IDS":"10002,10003"}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := BuildReleaseResult{Channel: "wechat", Environment: "test", Action: "restart-version", Version: "0.1.2", Component: "game-server", SourceCommit: strings.Repeat("a", 40), TargetServerIDs: []string{"10002", "10004"}}
	write := func() {
		t.Helper()
		payload, err := json.Marshal(result)
		if err != nil || data.CaptureReleaseResults(build.ID, ReleaseResultMarker+string(payload)) != nil {
			t.Fatal("cannot capture result")
		}
	}
	write()
	if err := data.AttachReleaseResults([]*Build{build}); err != nil || len(build.ReleaseResults) != 0 {
		t.Fatal("mismatched targets must be rejected", err)
	}
	result.TargetServerIDs = []string{"10003", "10002"}
	write()
	write()
	if err := data.FinishBuild(build.ID, "success", 0); err != nil {
		t.Fatal(err)
	}
	if err := data.AttachReleaseResults([]*Build{build}); err != nil || len(build.ReleaseResults) != 1 || !sameReleaseServerIDs(build.ReleaseResults[0].TargetServerIDs, []string{"10002", "10003"}) {
		t.Fatal("matching partial receipt must survive and deduplicate", err)
	}
	if records, err := data.ListReleaseRecords(project.ID); err != nil || len(records) != 0 {
		t.Fatal("partial release must not enter the whole-environment catalog", err)
	}
	if overviews, err := data.ListReleaseTargetOverviews(project.ID); err != nil || len(overviews) != 0 {
		t.Fatal("partial release must not become the whole-environment status", err)
	}
	full, err := data.CreateBuild(project.ID, 2, "manual", "main", "", `{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"test","RELEASE_ACTION":"restart-version","RELEASE_VERSION":"0.1.2","TARGET_SERVER_IDS":""}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result.TargetServerIDs = nil
	result.Branch = "release/wechat/test/game-server/0.1.2"
	payload, _ := json.Marshal(result)
	if err := data.CaptureReleaseResults(full.ID, ReleaseResultMarker+string(payload)); err != nil {
		t.Fatal(err)
	}
	if err := data.FinishBuild(full.ID, "success", 0); err != nil {
		t.Fatal(err)
	}
	if records, err := data.ListReleaseRecords(project.ID); err != nil || len(records) != 1 {
		t.Fatal("empty multi-target parameter must preserve full release catalog behavior", err)
	}
	if overviews, err := data.ListReleaseTargetOverviews(project.ID); err != nil || len(overviews) != 1 || overviews[0].LastSuccess == nil || overviews[0].LastSuccess.ID != full.ID {
		t.Fatal("full release must remain visible in environment status", err)
	}
}

func TestRequestedMultipleServersRejectInvalidAndMixedInputs(t *testing.T) {
	for _, input := range []map[string]interface{}{
		{"TARGET_SERVER_IDS": "10001,10001"}, {"TARGET_SERVER_IDS": "10001,"}, {"TARGET_SERVER_IDS": "010001"},
		{"TARGET_SERVER_IDS": []any{"10001"}}, {"TARGET_SERVER_IDS": nil}, {"TARGET_SERVER_ID": "10001", "TARGET_SERVER_IDS": "10002"},
	} {
		if _, valid := requestedReleaseServerIDs(input); valid {
			t.Fatalf("invalid selection accepted: %#v", input)
		}
	}
	if ids, valid := requestedReleaseServerIDs(map[string]interface{}{"TARGET_SERVER_IDS": "10002, 10001"}); !valid || !sameReleaseServerIDs(ids, []string{"10001", "10002"}) {
		t.Fatal("valid selection was rejected")
	}
	if ids, valid := requestedReleaseServerIDs(map[string]interface{}{"TARGET_SERVER_IDS": ""}); !valid || len(ids) != 0 {
		t.Fatal("empty selection must retain whole-environment behavior")
	}
}

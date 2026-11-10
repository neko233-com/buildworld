package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestJenkinsRetryCausesPreserveScheduleAndReplaceManualUser(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("retry-causes", "", "", "git", "main", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := db.CreateBuild(project.ID, 1, "schedule", "main", "abc", `{"COUNT":9007199254740993}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	makeRetry := func(parent *store.Build, kind, user string) *store.Build {
		t.Helper()
		parameters, err := AttachJenkinsRetryContext(parent.Parameters, kind, user)
		if err != nil {
			t.Fatal(err)
		}
		build, err := db.RetryBuildWithApprovalAndParameters(parent, false, parameters)
		if err != nil {
			t.Fatal(err)
		}
		return build
	}
	auto := makeRetry(root, "automatic", "")
	manual := makeRetry(auto, "manual", "bob")
	secondAuto := makeRetry(manual, "automatic", "")
	secondManual := makeRetry(secondAuto, "manual", "carol")
	runner := NewBuildRunner(db, nil, t.TempDir(), nil)
	defer runner.Shutdown(context.Background())
	for _, tc := range []struct {
		build  *store.Build
		manual bool
		user   string
	}{
		{auto, false, ""}, {manual, true, "bob"}, {secondAuto, true, "bob"}, {secondManual, true, "carol"},
	} {
		causes, err := runner.resolveJenkinsRetryCauses(tc.build)
		if err != nil || causes.UnknownNonUser || causes.HasManualRetry != tc.manual {
			t.Fatalf("build=%d causes=%+v err=%v", tc.build.Number, causes, err)
		}
		foundTimer, foundUser, foundRebuild := false, false, false
		for _, cause := range causes.Causes {
			switch cause.Class {
			case "TimerTriggerCause":
				foundTimer = true
			case "UserIdCause":
				foundUser = cause.User == tc.user
			case "RebuildCause":
				foundRebuild = true
			}
		}
		if !foundTimer || foundUser != tc.manual || foundRebuild != tc.manual {
			t.Fatalf("build=%d causes=%+v", tc.build.Number, causes)
		}
	}
	for _, mutate := range []func(*store.Build){
		func(b *store.Build) { b.Parameters = `{"incorrect":true}` },
		func(b *store.Build) { b.CommitSHA = "different" },
		func(b *store.Build) { b.Branch = "other" },
		func(b *store.Build) { b.ProjectID++ },
	} {
		stale := *root
		mutate(&stale)
		if _, err := db.RetryBuildWithApprovalAndParameters(&stale, false, `{}`); err == nil {
			t.Fatal("retry accepted a changed parent identity")
		}
	}
	if params := parseParams(secondManual.Parameters); params["COUNT"].(interface{ String() string }).String() != "9007199254740993" || len(params) != 1 {
		t.Fatalf("public parameters=%v", params)
	}
}

func TestPipelineUsesJenkinsTriggerOnlyForCalls(t *testing.T) {
	for _, tc := range []struct {
		condition string
		want      bool
	}{
		{`params.VALUE == "jenkinsTriggeredBy("`, false},
		{`true && jenkinsTriggeredBy ("RebuildCause", "", "false")`, true},
		{`params.FLAG == true`, false},
	} {
		config := &BuildConfig{Stages: []Stage{{JenkinsWhen: tc.condition}}}
		if got := pipelineUsesJenkinsTrigger(config); got != tc.want {
			t.Fatalf("%s got=%v", tc.condition, got)
		}
	}
}

func TestRemoteCauseSurvivesAutomaticRetry(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "remote-retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("remote-retry", "", "", "git", "main", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := db.CreateBuild(project.ID, 1, "http", "main", "abc", `{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	params, err := AttachJenkinsRetryContext(root.Parameters, "automatic", "")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := db.RetryBuildWithApprovalAndParameters(root, false, params)
	if err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(db, nil, t.TempDir(), nil)
	defer runner.Shutdown(context.Background())
	causes, err := runner.resolveJenkinsRetryCauses(retry)
	if err != nil || causes.UnknownNonUser || len(causes.Causes) != 1 || causes.Causes[0].Class != "RemoteCause" {
		t.Fatalf("causes=%+v err=%v", causes, err)
	}
}

func TestJenkinsPRRetryKeepsVerifiedMetadataAndKnownRebuildCause(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "pr-retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("pr-retry", "", "", "git", "main", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := AttachJenkinsChangeRequestContext(`{"LARGE":9007199254740993}`, JenkinsChangeRequestContext{ID: "42", Target: "main"})
	if err != nil {
		t.Fatal(err)
	}
	root, err := db.CreateBuild(project.ID, 1, "webhook", "refs/pull/42/head", "abc", parameters, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := AttachJenkinsRetryContext(parameters, "manual", "bob")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := db.RetryBuildWithApprovalAndParameters(root, false, updated)
	if err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(db, nil, t.TempDir(), nil)
	defer runner.Shutdown(context.Background())
	origin, err := runner.changeRequestWebhookOrigin(retry)
	if err != nil || !origin || readJenkinsChangeRequestContext(retry.Parameters, "42").Target != "main" {
		t.Fatalf("origin=%v retry=%+v err=%v", origin, retry, err)
	}
	causes, err := runner.resolveJenkinsRetryCauses(retry)
	if err != nil || !causes.UnknownNonUser || !causes.HasManualRetry {
		t.Fatalf("causes=%+v err=%v", causes, err)
	}
	encoded, err := json.Marshal(causes)
	if err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{jenkinsTriggerTypeContextKey: "retry", jenkinsRetryCausesContextKey: string(encoded)}
	for _, tc := range []struct {
		condition string
		want      bool
		wantErr   bool
	}{
		{`jenkinsTriggeredBy("UserIdCause","bob","true")`, true, false},
		{`jenkinsTriggeredBy("UserIdCause","alice","true")`, false, false},
		{`jenkinsTriggeredBy("useridcause","alice","true")`, true, false},
		{`jenkinsTriggeredBy("buildCause","","false")`, false, true},
		{`jenkinsTriggeredBy("RebuildCause","","false")`, true, false},
		{`jenkinsTriggeredBy("TimerTrigger","","false")`, false, true},
	} {
		got, err := EvaluateJenkinsWhenContext(tc.condition, nil, environment, "")
		if got != tc.want || (err != nil) != tc.wantErr {
			t.Fatalf("%s: got=%v err=%v", tc.condition, got, err)
		}
	}
}

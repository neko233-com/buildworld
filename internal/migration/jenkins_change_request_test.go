package migration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestJenkinsChangeRequestConditions(t *testing.T) {
	meta := map[string]string{"CHANGE_ID": "23", "CHANGE_TARGET": "Main", "CHANGE_BRANCH": "feature/ui", "CHANGE_FORK": "contributor", "CHANGE_TITLE": "Improve UX", "CHANGE_AUTHOR": "alice", "CHANGE_AUTHOR_DISPLAY_NAME": "Alice", "CHANGE_AUTHOR_EMAIL": "alice@example.com"}
	for _, tc := range []struct {
		condition string
		env       map[string]string
		want      bool
	}{
		{`changeRequest()`, meta, true},
		{`changeRequest()`, nil, false},
		{`changeRequest()`, map[string]string{"CHANGE_ID": ""}, true},
		{`changeRequest target: 'Main', author: 'alice'`, meta, true},
		{`changeRequest(target: 'main')`, meta, false},
		{`changeRequest target: 'm*', branch: 'feature/**', comparator: 'GLOB'`, meta, true},
		{`changeRequest authorEmail: '.+@example\\.com', comparator: 'REGEXP'`, meta, true},
		{`changeRequest id: '23', title: 'Other'`, meta, false},
		{`changeRequest authorDisplayName: 'Alice', fork: 'contributor'`, meta, true},
		{`changeRequest target: 'Main', comparator: 'unknown'`, meta, true},
		{`changeRequest target: ''`, meta, true},
		{`allOf { changeRequest(); branch 'main' }`, meta, true},
		{`not { changeRequest() }`, nil, true},
	} {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('test') { when { %s } steps { sh 'echo ok' } } } }`, tc.condition)
		cfg, err := engine.ParseJenkinsfileConfig(source, "change-request")
		if err != nil {
			t.Fatalf("%s: %v", tc.condition, err)
		}
		got, err := engine.ShouldRunStageWithJenkinsContext(cfg.Stages[0], nil, nil, tc.env, "main")
		if err != nil || got != tc.want {
			t.Fatalf("%s: got=%v want=%v error=%v", tc.condition, got, tc.want, err)
		}
	}
}

func TestJenkinsChangeRequestRejectsUnsupportedOrDynamicArguments(t *testing.T) {
	for _, condition := range []string{
		`changeRequest url: 'https://example.test/pr/1'`,
		`changeRequest target: params.TARGET`,
		`changeRequest target: 'main', target: 'other'`,
		`changeRequest unknown: 'value'`,
	} {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('test') { when { %s } steps { sh 'echo ok' } } } }`, condition)
		_, err := engine.ParseJenkinsfileConfig(source, "change-request")
		if err == nil || !strings.Contains(err.Error(), "when") {
			t.Fatalf("accepted %s: %v", condition, err)
		}
	}
}

func TestJenkinsChangeRequestMigrationRoundTrip(t *testing.T) {
	source := `pipeline { agent any stages { stage('pr') { when { changeRequest target: 'release/**', comparator: 'GLOB' } steps { sh 'echo ok' } } } }`
	converted, err := NewJenkinsfileStrategy().Convert(Request{Source: source})
	if err != nil {
		t.Fatal(err)
	}
	config, err := engine.ParsePipelineConfig(converted.Config)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Stages) != 1 {
		t.Fatalf("stages = %d", len(config.Stages))
	}
	for _, tc := range []struct {
		env  map[string]string
		want bool
	}{
		{map[string]string{"CHANGE_ID": "9", "CHANGE_TARGET": "release/v1"}, true},
		{map[string]string{"CHANGE_ID": "9", "CHANGE_TARGET": "main"}, false},
		{nil, false},
	} {
		got, err := engine.ShouldRunStageWithJenkinsContext(config.Stages[0], nil, nil, tc.env, "main")
		if err != nil || got != tc.want {
			t.Fatalf("env=%v got=%v want=%v error=%v", tc.env, got, tc.want, err)
		}
	}
}

func TestJenkinsChangeRequestRunsWithBuildEnvironment(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := store.New(filepath.Join(t.TempDir(), "changes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `pipeline { agent any
environment {
CHANGE_ID = '23'
CHANGE_TARGET = 'Main'
}
stages {
stage('matches') { when { changeRequest target: 'm*', comparator: 'GLOB' } steps { sh 'echo pr-stage-executed' } }
stage('skips') { when { changeRequest target: 'other' } steps { sh 'echo pr-stage-incorrect' } }
stage('escaped') { when { branch pattern: 'm\\w+n', comparator: 'REGEXP' } steps { sh 'echo regex-stage-executed' } }
} }`
	project, err := db.CreateProject("change-request", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", "{}", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(t.TempDir(), "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "success" || build.Status == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if build.Status != "success" || !strings.Contains(build.Log, "pr-stage-executed") || !strings.Contains(build.Log, "regex-stage-executed") || strings.Contains(build.Log, "pr-stage-incorrect") {
		t.Fatalf("status=%s log=%s", build.Status, build.Log)
	}
}

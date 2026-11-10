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

func TestJenkinsTriggeredByRunsWithStoredCause(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, tc := range []struct {
		trigger, condition, actor string
		want                      bool
		wantFailure               bool
	}{
		{"manual", `triggeredBy cause: 'UserIdCause', detail: 'alice'`, "alice", true, false},
		{"manual", `triggeredBy(cause: 'UserIdCause', detail: 'bob')`, "alice", false, false},
		{"manual", `triggeredBy cause: 'UserIdCause', detail: 'alice'`, "", false, true},
		{"manual", `triggeredBy 'UserIdCause'`, "", true, false},
		{"manual", `triggeredBy cause: 'useridcause', detail: 'bob'`, "alice", true, false},
		{"manual", `triggeredBy cause: 'User', detail: 'bob'`, "alice", true, false},
		{"manual", `triggeredBy 'IdCause'`, "alice", false, false},
		{"schedule", `triggeredBy 'TimerTrigger'`, "", true, false},
		{"schedule", `triggeredBy 'timer'`, "", true, false},
		{"schedule", `triggeredBy 'Trigger'`, "", false, false},
		{"schedule", `triggeredBy 'UnknownCause'`, "", false, false},
		{"vcs", `triggeredBy('SCMTrigger')`, "", true, false},
		{"finish", `triggeredBy 'BuildUpstreamCause'`, "", true, false},
		{"http", `triggeredBy 'RemoteCause'`, "", true, false},
		{"schedule", `triggeredBy 'SCMTrigger'`, "", false, false},
		{"retry", `triggeredBy 'UserIdCause'`, "alice", false, true},
	} {
		t.Run(tc.trigger+"/"+tc.condition+"/"+tc.actor, func(t *testing.T) {
			db, err := store.New(filepath.Join(t.TempDir(), "buildworld.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			source := fmt.Sprintf(`pipeline { agent any stages { stage('guard') { when { %s } steps { sh 'echo CAUSE_STAGE_EXECUTED' } } } }`, tc.condition)
			project, err := db.CreateProject("trigger", "", "", "git", "main", source, 0, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			parameters := `{}`
			if tc.actor != "" {
				parameters, err = engine.AttachJenkinsTriggerUser(parameters, tc.actor)
				if err != nil {
					t.Fatal(err)
				}
			}
			build, err := db.CreateBuild(project.ID, 1, tc.trigger, "main", "", parameters, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			runner := engine.NewBuildRunner(db, nil, filepath.Join(t.TempDir(), "workspaces"), nil)
			defer runner.Shutdown(context.Background())
			runner.Run(build.ID)
			for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
				build, err = db.GetBuild(build.ID)
				if err != nil {
					t.Fatal(err)
				}
				if build.Status == "success" || build.Status == "failed" {
					break
				}
			}
			if (build.Status == "failed") != tc.wantFailure || (!tc.wantFailure && build.Status != "success") || strings.Contains(build.Log, "CAUSE_STAGE_EXECUTED") != tc.want {
				t.Fatalf("status=%s log=%s", build.Status, build.Log)
			}
		})
	}
}

func TestJenkinsTriggeredByRejectsUnsupportedArguments(t *testing.T) {
	for _, condition := range []string{`triggeredBy 'Timer.*'`, `triggeredBy ''`, `triggeredBy cause: 'UserIdCause', surprise: 'x'`, `triggeredBy params.CAUSE`, `triggeredBy(cause: 'UserIdCause', cause: 'TimerTrigger')`} {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('guard') { when { %s } steps { sh 'echo ok' } } } }`, condition)
		if _, err := engine.ParseJenkinsfileConfig(source, "trigger"); err == nil {
			t.Fatalf("accepted %s", condition)
		}
	}
}

func TestAutomaticRetryPreservesTimerCondition(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := store.New(filepath.Join(t.TempDir(), "retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `pipeline { agent any stages { stage('timer') { when { triggeredBy 'TimerTrigger' } steps { sh 'echo TIMER_RETRY_EXECUTED; exit 1' } } } }`
	project, err := db.CreateProject("timer-retry", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "schedule", "main", "", `{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(t.TempDir(), "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	if err := runner.ConfigureExecutionPolicy(engine.ExecutionPolicy{DefaultTimeoutSec: 10, MaxConcurrentBuilds: 1, MaxConcurrentLocalBuilds: 1, RetryLimit: 1, FailFast: true}); err != nil {
		t.Fatal(err)
	}
	runner.Run(build.ID)
	var builds []*store.Build
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		builds, err = db.ListBuildsByProject(project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(builds) == 2 && builds[0].Status == "failed" && builds[1].Status == "failed" {
			break
		}
	}
	if len(builds) != 2 || builds[0].Status != "failed" || builds[1].Status != "failed" || !strings.Contains(builds[0].Log, "TIMER_RETRY_EXECUTED") || !strings.Contains(builds[1].Log, "TIMER_RETRY_EXECUTED") {
		t.Fatalf("builds=%+v", builds)
	}
}

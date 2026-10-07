package engine

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

type slowStartupSCMFetcher struct{ entered chan struct{} }

func (f slowStartupSCMFetcher) FetchSCMPipeline(ctx context.Context, _ SCMPipelineSource) (string, PipelineFormat, error) {
	close(f.entered)
	<-ctx.Done()
	return "", FormatJenkinsfile, ctx.Err()
}

func TestStartupSCMDoesNotBlockReadinessAndStopsWithQueue(t *testing.T) {
	root := t.TempDir()
	data, err := store.New(filepath.Join(root, "slow-startup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("slow-scm", "", "https://example.invalid/repo.git", "git", "main", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectPipelineSource(project.ID, "jenkinsfile", "scm", project.RepoURL, "main", "Jenkinsfile"); err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectBuildOnStartup(project.ID, true); err != nil {
		t.Fatal(err)
	}
	previous := DefaultSCMFetcher
	entered := make(chan struct{})
	DefaultSCMFetcher = slowStartupSCMFetcher{entered: entered}
	t.Cleanup(func() { DefaultSCMFetcher = previous })
	runner := NewBuildRunner(data, nil, filepath.Join(root, "workspaces"), nil)
	started := make(chan struct{})
	go func() { runner.StartQueue(context.Background()); close(started) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("startup SCM blocked queue readiness")
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("SCM did not start")
	}
	runner.StopQueue()
	builds, _ := data.ListBuildsByProject(project.ID)
	if len(builds) != 0 {
		t.Fatal("cancelled startup created a build")
	}
}

func TestStartupRejectsMissingRequiredDefaultWithoutExecuting(t *testing.T) {
	root := t.TempDir()
	data, err := store.New(filepath.Join(root, "required.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("required", "", "", "git", "main", "parameters:\n  - name: token\n    type: password\n    required: true\njobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectBuildOnStartup(project.ID, true); err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(data, nil, filepath.Join(root, "workspaces"), nil)
	err = runner.triggerStartupBuild(context.Background(), project, NewApprovalService(data))
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("missing default error=%v", err)
	}
	builds, _ := data.ListBuildsByProject(project.ID)
	if len(builds) != 0 {
		t.Fatal("missing required parameter queued a build")
	}
}

func TestStartupQueueRunsOncePerServerAndRecoversWithoutDuplicate(t *testing.T) {
	root := t.TempDir()
	data, err := store.New(filepath.Join(root, "startup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project := executionPolicyProject(t, data, "startup", "echo startup-ok")
	if err := data.SetProjectBuildOnStartup(project.ID, true); err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(data, nil, filepath.Join(root, "workspaces"), nil)
	runner.StartQueue(context.Background())
	t.Cleanup(runner.StopQueue)
	builds := waitForStartupBuildCount(t, data, project.ID, 1)
	if len(builds) != 1 || builds[0].Trigger != "startup" {
		t.Fatalf("builds=%+v", builds)
	}
	finished := waitForBuildStatus(t, data, builds[0].ID, 5*time.Second, "success")
	if !strings.Contains(finished.Log, "startup-ok") {
		t.Fatal(finished.Log)
	}
	runner.StartQueue(context.Background())
	builds, _ = data.ListBuildsByProject(project.ID)
	if len(builds) != 1 {
		t.Fatal("same server started twice")
	}
	runner.StopQueue()
	// A new server gets another one-shot, while an interrupted running build
	// is recovered as the same build rather than duplicated by startup.
	interrupted := executionPolicyBuild(t, data, project, 2)
	if err := data.StartBuild(interrupted.ID); err != nil {
		t.Fatal(err)
	}
	restarted := NewBuildRunner(data, nil, filepath.Join(root, "workspaces"), nil)
	restarted.StartQueue(context.Background())
	defer restarted.StopQueue()
	waitForBuildStatus(t, data, interrupted.ID, 5*time.Second, "success")
	builds, _ = data.ListBuildsByProject(project.ID)
	if len(builds) != 2 {
		t.Fatalf("recovery duplicated: %d", len(builds))
	}
	restarted.StopQueue()
	next := NewBuildRunner(data, nil, filepath.Join(root, "workspaces"), nil)
	next.StartQueue(context.Background())
	defer next.StopQueue()
	builds = waitForStartupBuildCount(t, data, project.ID, 3)
	if len(builds) != 3 || builds[0].Trigger != "startup" {
		t.Fatalf("next startup=%+v", builds)
	}
	waitForBuildStatus(t, data, builds[0].ID, 5*time.Second, "success")
}

func TestStartupUsesDefaultParametersAndApproval(t *testing.T) {
	root := t.TempDir()
	data, err := store.New(filepath.Join(root, "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("approval", "", "", "git", "main", `approval:
  strategy: single
parameters:
  - name: target
    type: string
    default: staging
jobs:
  build:
    steps:
      - run: echo should-wait
`, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectBuildOnStartup(project.ID, true); err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(data, nil, filepath.Join(root, "workspaces"), nil)
	runner.StartQueue(context.Background())
	defer runner.StopQueue()
	builds := waitForStartupBuildCount(t, data, project.ID, 1)
	if len(builds) != 1 || !builds[0].ApprovalRequired || builds[0].StartedAt != nil || !strings.Contains(builds[0].Parameters, `"target":"staging"`) {
		t.Fatalf("builds=%+v", builds)
	}
	// Startup is asynchronous: the durable approval hold is created first so
	// dispatch cannot race the approval row. Await that row, not just the build.
	deadline := time.Now().Add(3 * time.Second)
	for {
		approval, err := data.GetBuildApprovalByBuild(builds[0].ID)
		if err == nil {
			if approval.Status != "pending" {
				t.Fatalf("approval=%+v", approval)
			}
			break
		}
		if !errors.Is(err, sql.ErrNoRows) || time.Now().After(deadline) {
			t.Fatalf("approval=%+v err=%v", approval, err)
		}
		current, err := data.GetBuild(builds[0].ID)
		if err != nil || current.Status != "pending_approval" || current.StartedAt != nil {
			t.Fatalf("approval hold lost: build=%+v err=%v", current, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForStartupBuildCount(t *testing.T, data *store.Store, projectID int64, count int) []*store.Build {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		builds, err := data.ListBuildsByProject(projectID)
		if err != nil {
			t.Fatal(err)
		}
		if len(builds) == count {
			return builds
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("startup build count did not reach %d", count)
	return nil
}

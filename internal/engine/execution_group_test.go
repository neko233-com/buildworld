package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestExecutionGroupQueuesAcrossProjectsWithoutConsumingWorkerCapacity(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "execution-group.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	firstProject := executionPolicyProject(t, database, "android-publish", executionPolicyCommand(3))
	secondProject := executionPolicyProject(t, database, "wechat-publish", executionPolicyCommand(1))
	independentProject := executionPolicyProject(t, database, "backend-publish", executionPolicyCommand(3))
	ordinaryProject := executionPolicyProject(t, database, "ordinary", executionPolicyCommand(3))
	for _, project := range []*store.Project{firstProject, secondProject} {
		if err := database.SetEnvVar("project", &project.ID, ExecutionGroupVariable, "tuanjie-hot-update", false, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.SetEnvVar("project", &independentProject.ID, ExecutionGroupVariable, "backend-release", false, ""); err != nil {
		t.Fatal(err)
	}
	if err := database.SetEnvVar("global", nil, ExecutionGroupVariable, "tuanjie-hot-update", false, ""); err != nil {
		t.Fatal(err)
	}

	first := executionPolicyBuild(t, database, firstProject, 1)
	second := executionPolicyBuild(t, database, secondProject, 1)
	independent := executionPolicyBuild(t, database, independentProject, 1)
	ordinary := executionPolicyBuild(t, database, ordinaryProject, 1)
	for _, build := range []*store.Build{first, second, independent, ordinary} {
		project, err := database.GetProject(build.ProjectID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.CreateBuildQueueItem(build.ID, project.ID, project.Name, 0, build.Trigger, build.Branch); err != nil {
			t.Fatal(err)
		}
	}
	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	if err := runner.ConfigureExecutionPolicy(ExecutionPolicy{
		DefaultTimeoutSec: 15, MaxConcurrentBuilds: 3, MaxConcurrentLocalBuilds: 3, FailFast: true,
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.StartQueue(ctx)
	defer runner.StopQueue()

	waitForBuildStatus(t, database, first.ID, 6*time.Second, "running")
	waitForBuildStatus(t, database, independent.ID, 6*time.Second, "running")
	waitForBuildStatus(t, database, ordinary.ID, 6*time.Second, "running")
	waiter, err := database.GetBuild(second.ID)
	if err != nil || waiter.Status != "pending" || waiter.StartedAt != nil {
		t.Fatalf("group peer acquired capacity before owner released: %+v (%v)", waiter, err)
	}
	waitForBuildStatus(t, database, first.ID, 9*time.Second, "success")
	waitForBuildStatus(t, database, second.ID, 9*time.Second, "success")
	owner, _ := database.GetBuild(first.ID)
	completed, _ := database.GetBuild(second.ID)
	if owner.FinishedAt == nil || completed.StartedAt == nil || completed.StartedAt.Before(*owner.FinishedAt) {
		t.Fatalf("group run times overlap: group owner=%v, peer=%v", owner.FinishedAt, completed.StartedAt)
	}
}

func TestExecutionGroupRejectsInvalidOrSecretProjectValue(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "execution-group-values.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project := executionPolicyProject(t, database, "invalid-execution-group", executionPolicyCommand(1))
	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)

	for _, tc := range []struct {
		value  string
		secret bool
	}{
		{value: ""},
		{value: "not a valid name"},
		{value: "tuanjie-hot-update", secret: true},
	} {
		if err := database.SetEnvVar("project", &project.ID, ExecutionGroupVariable, tc.value, tc.secret, ""); err != nil {
			t.Fatal(err)
		}
		if _, valid, err := runner.resolveExecutionGroup(project.ID); valid || err != nil {
			t.Fatalf("invalid project execution group was resolved (valid=%t, err=%v)", valid, err)
		}
	}
}

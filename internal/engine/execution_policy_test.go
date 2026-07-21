package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

func executionPolicyCommand(seconds int) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("ping -n %d 127.0.0.1 >NUL", seconds+1)
	}
	return fmt.Sprintf("sleep %d", seconds)
}

func executionPolicyProject(t *testing.T, database *store.Store, name, command string) *store.Project {
	t.Helper()
	config := testShellPipelineSource("Verify", "Command", command)
	project, err := database.CreateProject(name, "", "", "git", "main", config, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return project
}

func executionPolicyBuild(t *testing.T, database *store.Store, project *store.Project, number int) *store.Build {
	t.Helper()
	build, err := database.CreateBuild(project.ID, number, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return build
}

func waitForBuildStatus(t *testing.T, database *store.Store, buildID int64, timeout time.Duration, statuses ...string) *store.Build {
	t.Helper()
	accepted := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		accepted[status] = struct{}{}
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		build, err := database.GetBuild(buildID)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := accepted[build.Status]; ok {
			return build
		}
		time.Sleep(15 * time.Millisecond)
	}
	build, err := database.GetBuild(buildID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("build %d status = %q after %s, want one of %v", buildID, build.Status, timeout, statuses)
	return nil
}

func TestBuildRunnerAppliesDefaultTimeout(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "timeout.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project := executionPolicyProject(t, database, "timeout", executionPolicyCommand(4))
	build := executionPolicyBuild(t, database, project, 1)
	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	if err := runner.ConfigureExecutionPolicy(ExecutionPolicy{DefaultTimeoutSec: 1, MaxConcurrentBuilds: 1, MaxConcurrentLocalBuilds: 1, RetryLimit: 0, FailFast: true}); err != nil {
		t.Fatal(err)
	}

	runner.Run(build.ID)
	finished := waitForBuildStatus(t, database, build.ID, 5*time.Second, "failed")
	if finished.TimeoutSec != 1 {
		t.Fatalf("timeout_sec = %d, want 1", finished.TimeoutSec)
	}
	if !strings.Contains(finished.Log, "timed out after 1s") {
		t.Fatalf("build log does not explain applied timeout:\n%s", finished.Log)
	}
}

func TestBuildRunnerAppliesDefaultTimeoutWhenTerminalWatcherCanBeSkipped(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "conditional-watch-timeout.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	config := fmt.Sprintf(`import { definePipeline, shell, stage, step } from "@buildworld/pipeline"
export default definePipeline({
  allowLongRunning: true,
  stages: [
    stage("Slow", shell("Wait", %q)),
    stage("Observe", step("Logs", "service_watch", "", {
      if: "false",
      config: { pid_file: "server.pid", log_file: "server.log" },
    })),
  ],
})`, executionPolicyCommand(4))
	project, err := database.CreateProject("conditional-watch-timeout", "", "", "git", "main", config, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build := executionPolicyBuild(t, database, project, 1)
	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	if err := runner.ConfigureExecutionPolicy(ExecutionPolicy{DefaultTimeoutSec: 1, MaxConcurrentBuilds: 1, MaxConcurrentLocalBuilds: 1, RetryLimit: 0, FailFast: true}); err != nil {
		t.Fatal(err)
	}

	runner.Run(build.ID)
	finished := waitForBuildStatus(t, database, build.ID, 5*time.Second, "failed")
	if finished.TimeoutSec != 1 {
		t.Fatalf("timeout_sec = %d, want finite default timeout 1", finished.TimeoutSec)
	}
	if !strings.Contains(finished.Log, "timed out after 1s") {
		t.Fatalf("build log does not explain applied timeout:\n%s", finished.Log)
	}
}

func TestBuildRunnerRunsMigratedPostSteps(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "post.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	config := `import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({
  stages: [stage("Build", shell("main", "echo main"))],
  post: {
    always: [shell("always", "echo post-always")],
    success: [shell("success", "echo post-success")],
  },
})`
	project, err := database.CreateProject("post", "", "", "git", "main", config, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build := executionPolicyBuild(t, database, project, 1)
	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	runner.Run(build.ID)
	finished := waitForBuildStatus(t, database, build.ID, 5*time.Second, "success")
	if !strings.Contains(finished.Log, "post-always") || !strings.Contains(finished.Log, "post-success") {
		t.Fatalf("post steps missing from log:\n%s", finished.Log)
	}
}

func TestBuildRunnerEnforcesGlobalConcurrency(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "concurrency.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project := executionPolicyProject(t, database, "concurrency", executionPolicyCommand(1))
	first := executionPolicyBuild(t, database, project, 1)
	second := executionPolicyBuild(t, database, project, 2)
	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	if err := runner.ConfigureExecutionPolicy(ExecutionPolicy{DefaultTimeoutSec: 10, MaxConcurrentBuilds: 1, MaxConcurrentLocalBuilds: 1, RetryLimit: 0, FailFast: true}); err != nil {
		t.Fatal(err)
	}

	runner.Run(first.ID)
	runner.Run(second.ID)
	deadline := time.Now().Add(2 * time.Second)
	observedSerialized := false
	for time.Now().Before(deadline) {
		firstCurrent, _ := database.GetBuild(first.ID)
		secondCurrent, _ := database.GetBuild(second.ID)
		running := 0
		pending := 0
		for _, status := range []string{firstCurrent.Status, secondCurrent.Status} {
			if status == "running" {
				running++
			}
			if status == "pending" {
				pending++
			}
		}
		if running == 1 && pending == 1 {
			observedSerialized = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !observedSerialized {
		firstCurrent, _ := database.GetBuild(first.ID)
		secondCurrent, _ := database.GetBuild(second.ID)
		t.Fatalf("statuses = %q, %q; want exactly one running and one pending", firstCurrent.Status, secondCurrent.Status)
	}
	waitForBuildStatus(t, database, first.ID, 5*time.Second, "success")
	waitForBuildStatus(t, database, second.ID, 5*time.Second, "success")
}

func TestBuildRunnerDurableQueueHonorsReorderBeforeDispatch(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "durable-queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project := executionPolicyProject(t, database, "durable-queue", executionPolicyCommand(1))
	first := executionPolicyBuild(t, database, project, 1)
	second := executionPolicyBuild(t, database, project, 2)
	third := executionPolicyBuild(t, database, project, 3)
	firstQueue, err := database.CreateBuildQueueItem(first.ID, project.ID, project.Name, 3, first.Trigger, first.Branch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateBuildQueueItem(second.ID, project.ID, project.Name, 2, second.Trigger, second.Branch); err != nil {
		t.Fatal(err)
	}
	thirdQueue, err := database.CreateBuildQueueItem(third.ID, project.ID, project.Name, 1, third.Trigger, third.Branch)
	if err != nil {
		t.Fatal(err)
	}

	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	if err := runner.ConfigureExecutionPolicy(ExecutionPolicy{
		DefaultTimeoutSec:        10,
		MaxConcurrentBuilds:      1,
		MaxConcurrentLocalBuilds: 1,
		RetryLimit:               0,
		FailFast:                 true,
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.StartQueue(ctx)
	defer runner.StopQueue()

	waitForBuildStatus(t, database, first.ID, 3*time.Second, "running")
	firstQueueState, err := database.ListBuildQueue("")
	if err != nil {
		t.Fatal(err)
	}
	if len(firstQueueState) != 3 || firstQueueState[0].ID != firstQueue.ID {
		t.Fatalf("initial queue = %#v", firstQueueState)
	}
	if _, err := NewBuildQueueReorderService(database, nil).Apply(thirdQueue.ID, "move_top"); err != nil {
		t.Fatal(err)
	}
	runner.WakeQueue()

	waitForBuildStatus(t, database, first.ID, 5*time.Second, "success")
	waitForBuildStatus(t, database, third.ID, 5*time.Second, "success")
	waitForBuildStatus(t, database, second.ID, 5*time.Second, "success")
	secondFinished, _ := database.GetBuild(second.ID)
	thirdFinished, _ := database.GetBuild(third.ID)
	if secondFinished.StartedAt == nil || thirdFinished.StartedAt == nil || !thirdFinished.StartedAt.Before(*secondFinished.StartedAt) {
		t.Fatalf("second/third start times = %v/%v; reordered third build did not dispatch first", secondFinished.StartedAt, thirdFinished.StartedAt)
	}
}

func TestBuildRunnerStartQueueRecoversInterruptedBuild(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "restart-recovery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project := executionPolicyProject(t, database, "restart-recovery", "echo recovered")
	build := executionPolicyBuild(t, database, project, 1)
	if err := database.StartBuild(build.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateBuildQueueItem(build.ID, project.ID, project.Name, 0, build.Trigger, build.Branch); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateBuildQueueItemStatusByBuildID(build.ID, "running"); err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.StartQueue(ctx)
	defer runner.StopQueue()
	finished := waitForBuildStatus(t, database, build.ID, 5*time.Second, "success")
	if !strings.Contains(finished.Log, "Requeued after interrupted server restart") || !strings.Contains(finished.Log, "recovered") {
		t.Fatalf("recovered build log = %s", finished.Log)
	}
}

func TestBuildRunnerSchedulesBoundedAutomaticRetry(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "retry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project := executionPolicyProject(t, database, "retry", "command-that-does-not-exist-buildworld")
	build := executionPolicyBuild(t, database, project, 1)
	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	if err := runner.ConfigureExecutionPolicy(ExecutionPolicy{DefaultTimeoutSec: 10, MaxConcurrentBuilds: 1, MaxConcurrentLocalBuilds: 1, RetryLimit: 1, FailFast: true}); err != nil {
		t.Fatal(err)
	}

	runner.Run(build.ID)
	deadline := time.Now().Add(5 * time.Second)
	var builds []*store.Build
	for time.Now().Before(deadline) {
		builds, err = database.ListBuildsByProject(project.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(builds) == 2 && builds[0].Status == "failed" && builds[1].Status == "failed" {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if len(builds) != 2 {
		t.Fatalf("build count = %d, want original plus one automatic retry", len(builds))
	}
	for _, candidate := range builds {
		if candidate.Status != "failed" {
			t.Fatalf("build %d status = %q, want failed", candidate.ID, candidate.Status)
		}
	}
	retry := builds[0]
	if retry.RetriedFrom == nil || *retry.RetriedFrom != build.ID {
		t.Fatalf("retry.retried_from = %#v, want %d", retry.RetriedFrom, build.ID)
	}
	// Give a runaway retry loop enough time to expose itself.
	time.Sleep(150 * time.Millisecond)
	builds, err = database.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 2 {
		t.Fatalf("bounded retry created %d builds, want 2", len(builds))
	}
}

func TestBuildRunnerCanContinueAfterFailureAndStillFailBuild(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "fail-fast.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project := executionPolicyProject(t, database, "continue-after-failure", "")
	config := `import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({
  stages: [stage("Verify", [
    shell("Expected failure", "command-that-does-not-exist-buildworld"),
    shell("Follow-up", "echo FOLLOW_UP_EXECUTED"),
  ])],
})`
	if err := database.UpdateProject(project.ID, project.Name, project.Description, project.RepoURL, project.RepoType, project.DefaultBranch, config, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	build := executionPolicyBuild(t, database, project, 1)
	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	if err := runner.ConfigureExecutionPolicy(ExecutionPolicy{
		DefaultTimeoutSec:        10,
		MaxConcurrentBuilds:      1,
		MaxConcurrentLocalBuilds: 1,
		RetryLimit:               0,
		FailFast:                 false,
	}); err != nil {
		t.Fatal(err)
	}

	runner.Run(build.ID)
	finished := waitForBuildStatus(t, database, build.ID, 5*time.Second, "failed")
	if !strings.Contains(finished.Log, "FOLLOW_UP_EXECUTED") {
		t.Fatalf("follow-up step did not run with fail-fast disabled:\n%s", finished.Log)
	}
	if !strings.Contains(finished.Log, "1 step(s) failed") {
		t.Fatalf("final failure summary missing:\n%s", finished.Log)
	}
}

func TestExecutionResourceEnvironmentUsesConfiguredCPUBudget(t *testing.T) {
	policy := ExecutionPolicy{CPUPercent: 25}
	environment := executionResourceEnvironment(policy)
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}
	expectedThreads := fmt.Sprintf("%d", cpuThreadBudget(25))
	if values["BUILDWORLD_CPU_LIMIT_PERCENT"] != "25" {
		t.Fatalf("CPU percentage = %q, want 25", values["BUILDWORLD_CPU_LIMIT_PERCENT"])
	}
	for _, name := range []string{"BUILDWORLD_CPU_THREADS", "GOMAXPROCS", "CARGO_BUILD_JOBS", "CMAKE_BUILD_PARALLEL_LEVEL", "UV_THREADPOOL_SIZE", "NPM_CONFIG_JOBS"} {
		if values[name] != expectedThreads {
			t.Fatalf("%s = %q, want %s", name, values[name], expectedThreads)
		}
	}
}

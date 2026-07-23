package engine

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

type triggerStaticSCMFetcher struct{ source string }

func (f triggerStaticSCMFetcher) FetchSCMPipeline(context.Context, SCMPipelineSource) (string, PipelineFormat, error) {
	return f.source, FormatTypeScript, nil
}

func TestGitLSRemoteHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := gitLSRemoteContext(ctx, "https://example.invalid/repository.git", "main")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("git ls-remote error = %v, want context cancellation", err)
	}
}

func TestTriggerCheckerSkipsDisabledProject(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "disabled-trigger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("disabled-auto", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectEnabled(project.ID, false); err != nil {
		t.Fatal(err)
	}
	project, err = data.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}

	NewTriggerChecker(data, nil, nil).triggerProjectBuild(project, "schedule", "main", "", nil)
	builds, err := data.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 0 {
		t.Fatalf("disabled automatic trigger created %d builds", len(builds))
	}
}

func TestTriggerCheckerLoadsSCMPipelineTriggers(t *testing.T) {
	data, err := store.New(filepath.Join(t.TempDir(), "scm-trigger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	project, err := data.CreateProject("scm-trigger", "", "https://example.invalid/repo.git", "git", "main", "", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectPipelineSource(project.ID, "typescript", "scm", project.RepoURL, "main", "buildworld.ts"); err != nil {
		t.Fatal(err)
	}
	project, err = data.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	previous := DefaultSCMFetcher
	DefaultSCMFetcher = triggerStaticSCMFetcher{source: `import { definePipeline, stage, shell, trigger } from "@buildworld/pipeline"
export default definePipeline({ on: [trigger("cron", { expression: "* * * * *" })], stages: [stage("build", shell("build", "true"))] })`}
	t.Cleanup(func() { DefaultSCMFetcher = previous })
	checker := NewTriggerChecker(data, nil, nil)
	config, err := checker.loadProjectConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Triggers) != 1 || config.Triggers[0].Type != "cron" || config.Triggers[0].Config["expression"] != "* * * * *" {
		t.Fatalf("SCM triggers = %#v", config.Triggers)
	}
	checker.checkCronTriggers()
	builds, err := data.ListBuildsByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 1 || builds[0].Trigger != "schedule" {
		t.Fatalf("scheduled SCM builds = %#v", builds)
	}
}

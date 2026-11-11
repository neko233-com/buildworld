package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestJenkinsPostArchivesBeforeCleanup(t *testing.T) {
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("archive-post", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspaces", "job")
	if err := os.MkdirAll(filepath.Join(workspace, "dist", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "dist", "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "dist", "assets", "app.js"), []byte("app"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	runner.SetArtifactManager(NewArtifactManager(db, filepath.Join(root, "artifacts")))
	config := &BuildConfig{Post: map[string][]Step{
		"success": {{Name: "archive", Type: "jenkins_archive_artifacts", Config: map[string]string{"patterns": "dist/**", "only_if_successful": "true"}}},
		"cleanup": {{Name: "cleanWs", Type: "jenkins_delete_dir"}},
	}}
	if err := runner.runPostSteps(context.Background(), config, "success", workspace, project, build, nil, nil); err != nil {
		t.Fatal(err)
	}
	artifacts, err := db.ListArtifactsByBuild(build.ID)
	if err != nil || len(artifacts) != 2 {
		t.Fatalf("artifacts=%#v err=%v", artifacts, err)
	}
	names := map[string]bool{}
	for _, artifact := range artifacts {
		names[artifact.Name] = true
		if artifact.SHA256 == "" {
			t.Fatalf("artifact %q has no SHA-256", artifact.Name)
		}
		if _, err := os.ReadFile(artifact.Path); err != nil {
			t.Fatal(err)
		}
	}
	if !names["dist/index.html"] || !names["dist/assets/app.js"] {
		t.Fatalf("artifact paths=%v", names)
	}
	if _, err := os.Stat(filepath.Join(workspace, "dist")); !os.IsNotExist(err) {
		t.Fatalf("workspace was not cleaned: %v", err)
	}
}

func TestJenkinsPostArchiveFailureFailsBuildAndRunsCleanup(t *testing.T) {
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	config := &BuildConfig{
		Name:   "archive-missing",
		Stages: []Stage{{Name: "build", Steps: []Step{{Name: "ok", Type: "shell", Command: "echo ok"}}}},
		Post: map[string][]Step{
			"success": {{Name: "archive", Type: "jenkins_archive_artifacts", Config: map[string]string{"patterns": "missing/**"}}},
			"cleanup": {{Name: "cleanWs", Type: "jenkins_delete_dir"}},
		},
	}
	source, err := FormatTypeScriptPipeline(config)
	if err != nil {
		t.Fatal(err)
	}
	project, err := db.CreateProject("archive-missing", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	runner.SetArtifactManager(NewArtifactManager(db, filepath.Join(root, "artifacts")))
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	waitForBuildStatus(t, db, build.ID, 5*time.Second, "failed")
	finished, err := db.GetBuild(build.ID)
	if err != nil || !strings.Contains(finished.Log, "archiveArtifacts matched no files") || !strings.Contains(finished.Log, "post cleanup") {
		t.Fatalf("failed build=%#v err=%v", finished, err)
	}
}

func TestJenkinsArchiveRejectsEmptyAndTraversal(t *testing.T) {
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("archive-errors", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewArtifactManager(db, filepath.Join(root, "artifacts"))
	if _, err := manager.CollectJenkins(build.ID, root, "missing/**", false); err == nil || !strings.Contains(err.Error(), "no files") {
		t.Fatalf("empty archive error=%v", err)
	}
	if _, err := manager.CollectJenkins(build.ID, root, "../**", true); err == nil {
		t.Fatal("archive accepted traversal")
	}
	if count, err := manager.CollectJenkins(build.ID, root, "missing/**", true); err != nil || count != 0 {
		t.Fatalf("allowed empty archive count=%d err=%v", count, err)
	}
}

func TestJenkinsPostArchiveOnlyIfSuccessfulAndRemoteRejection(t *testing.T) {
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("post-failure", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspaces", "job")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "result.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	step := Step{Name: "archive", Type: "jenkins_archive_artifacts", Config: map[string]string{"patterns": "*.txt", "only_if_successful": "true"}}
	runner := NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	runner.SetArtifactManager(NewArtifactManager(db, filepath.Join(root, "artifacts")))
	config := &BuildConfig{Post: map[string][]Step{"always": {step}}}
	if err := runner.runPostSteps(context.Background(), config, "failure", workspace, project, build, nil, nil); err != nil {
		t.Fatal(err)
	}
	artifacts, err := db.ListArtifactsByBuild(build.ID)
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("failure archived files: %#v err=%v", artifacts, err)
	}
	config.Stages = []Stage{{Name: "build", Steps: []Step{{Name: "ok", Type: "shell", Command: "echo ok"}}}}
	config.AgentRequirements = []string{"remote"}
	if err := ValidatePipelineSemantics(config); err == nil || !strings.Contains(err.Error(), "local executor") {
		t.Fatalf("remote workspace operation error=%v", err)
	}
}

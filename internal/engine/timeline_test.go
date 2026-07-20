package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestParseBuildTimelineRunningUsesPlanOrder(t *testing.T) {
	config := &BuildConfig{Stages: []Stage{
		{Name: "Checkout", Steps: []Step{{Name: "Clone", Type: "git"}}},
		{Name: "Package", Steps: []Step{{Name: "Test", Type: "shell"}, {Name: "Build", Type: "shell"}}},
	}}
	logText := strings.Join([]string{
		"[10:00:00] [] " + encodeTimelinePlan(config),
		"[10:00:01] [Checkout] === Stage: Checkout ===",
		"[10:00:01] [Checkout] --- Step: Clone ---",
		"[10:00:02] [Checkout] --- Step complete: Clone ---",
		"[10:00:02] [Checkout] === Stage complete: Checkout ===",
		"[10:00:03] [Package] === Stage: Package ===",
		"[10:00:03] [Package] --- Step: Test ---",
	}, "\n")

	timeline := ParseBuildTimeline("running", logText)
	if timeline.TotalSteps != 3 || timeline.CompletedSteps != 1 || timeline.CurrentStep != 1 {
		t.Fatalf("timeline summary = %#v", timeline)
	}
	want := []string{"success", "running", "pending"}
	for i, status := range want {
		if timeline.Steps[i].Status != status {
			t.Fatalf("step %d status = %q, want %q", i, timeline.Steps[i].Status, status)
		}
		if timeline.Steps[i].Index != i {
			t.Fatalf("step %d index = %d", i, timeline.Steps[i].Index)
		}
	}
}

func TestParseBuildTimelineFailureSkipsRemainingSteps(t *testing.T) {
	config := &BuildConfig{Stages: []Stage{
		{Name: "Verify", Steps: []Step{{Name: "Compile"}, {Name: "Test"}, {Name: "Package"}}},
	}}
	logText := strings.Join([]string{
		"[10:00:00] [] " + encodeTimelinePlan(config),
		"[10:00:01] [Verify] === Stage: Verify ===",
		"[10:00:01] [Verify] --- Step: Compile ---",
		"[10:00:02] [Verify] --- Step complete: Compile ---",
		"[10:00:03] [Verify] --- Step: Test ---",
		"[10:00:04] [Verify] ERROR: exit status 1",
		"[10:00:04] [] BUILD FAILED: step \"Test\" failed",
	}, "\n")

	timeline := ParseBuildTimeline("failed", logText)
	want := []string{"success", "failed", "skipped"}
	for i, status := range want {
		if timeline.Steps[i].Status != status {
			t.Fatalf("step %d status = %q, want %q", i, timeline.Steps[i].Status, status)
		}
	}
}

func TestParseBuildTimelineReconstructsLegacyLogs(t *testing.T) {
	logText := strings.Join([]string{
		"[10:00:01] [Build] === Stage: Build ===",
		"[10:00:01] [Build] --- Step: Compile ---",
		"[10:00:02] [Build] compiler output",
		"[10:00:03] [Build] --- Step: Package ---",
		"[10:00:04] [] Build #1 succeeded",
	}, "\n")

	timeline := ParseBuildTimeline("success", logText)
	if timeline.TotalSteps != 2 || timeline.CompletedSteps != 2 {
		t.Fatalf("legacy timeline = %#v", timeline)
	}
	if timeline.Steps[0].Name != "Compile" || timeline.Steps[1].Name != "Package" {
		t.Fatalf("legacy step order = %#v", timeline.Steps)
	}
}

func TestParseBuildTimelinePendingDoesNotInventProgress(t *testing.T) {
	config := &BuildConfig{Stages: []Stage{{Name: "Build", Steps: []Step{{Name: "Compile"}}}}}
	timeline := ParseBuildTimeline("pending", "[10:00:00] [] "+encodeTimelinePlan(config))
	if timeline.CurrentStep != -1 || timeline.CompletedSteps != 0 || timeline.Steps[0].Status != "pending" {
		t.Fatalf("pending timeline = %#v", timeline)
	}
}

func TestParseBuildTimelineCancellationOverridesCommandError(t *testing.T) {
	config := &BuildConfig{Stages: []Stage{{
		Name: "Build",
		Steps: []Step{
			{Name: "Compile"},
			{Name: "Package"},
		},
	}}}
	logText := strings.Join([]string{
		"[10:00:00] [] " + encodeTimelinePlan(config),
		"[10:00:01] [Build] === Stage: Build ===",
		"[10:00:01] [Build] --- Step: Compile ---",
		"[10:00:02] [Build] ERROR: signal: killed",
	}, "\n")

	timeline := ParseBuildTimeline("cancelled", logText)
	if timeline.CurrentStep != 0 || timeline.Steps[0].Status != "cancelled" || timeline.Steps[1].Status != "skipped" {
		t.Fatalf("cancelled timeline = %#v", timeline)
	}
}

func TestBuildRunnerExecutesAndCollectsLinearPipeline(t *testing.T) {
	temporary := t.TempDir()
	database, err := store.New(filepath.Join(temporary, "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	config := BuildConfig{
		Name: "linear-smoke",
		Stages: []Stage{
			{Name: "Checkout", Steps: []Step{{Name: "Prepare", Type: "shell", Command: "echo checkout>order.txt"}}},
			{Name: "Verify", Steps: []Step{{Name: "Test", Type: "shell", Command: "echo verify>>order.txt"}}},
			{Name: "Package", Steps: []Step{{Name: "Archive", Type: "shell", Command: "echo package>>order.txt"}}},
		},
		Artifacts: []string{"order.txt"},
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject("linear-smoke", "", "", "git", "main", string(configJSON), 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	runner := NewBuildRunner(database, nil, filepath.Join(temporary, "workspace"), nil)
	runner.SetArtifactManager(NewArtifactManager(database, filepath.Join(temporary, "artifacts")))
	runner.run(context.Background(), build.ID)

	finished, err := database.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "success" {
		t.Fatalf("build status = %q\n%s", finished.Status, finished.Log)
	}
	timeline := ParseBuildTimeline(finished.Status, finished.Log)
	if timeline.TotalSteps != 3 || timeline.CompletedSteps != 3 {
		t.Fatalf("linear timeline = %#v", timeline)
	}
	prepare := strings.Index(finished.Log, "--- Step: Prepare ---")
	test := strings.Index(finished.Log, "--- Step: Test ---")
	archive := strings.Index(finished.Log, "--- Step: Archive ---")
	if prepare < 0 || test <= prepare || archive <= test {
		t.Fatalf("steps did not execute in declared order:\n%s", finished.Log)
	}

	artifacts, err := database.ListArtifactsByBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifact count = %d, want 1", len(artifacts))
	}
	content, err := os.ReadFile(artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(content)); strings.Join(got, ",") != "checkout,verify,package" {
		t.Fatalf("artifact recorded execution order %q", content)
	}
}

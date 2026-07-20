package engine

import (
	"strings"
	"testing"
)

func TestAnalyzeBuildProblemsFindsFailedStepAndTerminalSummary(t *testing.T) {
	logText := strings.Join([]string{
		"[10:00:01] [Verify] === Stage: Verify ===",
		"[10:00:02] [Verify] --- Step: Unit tests ---",
		"[10:00:03] [Verify] ERROR: exit status 1",
		"[10:00:04] [] BUILD FAILED: step \"Unit tests\" failed: exit status 1",
	}, "\n")

	report := AnalyzeBuildProblems("failed", logText)
	if report.Version != BuildProblemReportVersion || report.FailedStepCount != 1 {
		t.Fatalf("unexpected report summary: %#v", report)
	}
	if len(report.Problems) != 2 {
		t.Fatalf("problem count = %d, want 2", len(report.Problems))
	}
	first := report.Problems[0]
	if first.Stage != "Verify" || first.Step != "Unit tests" || first.Line != 3 {
		t.Fatalf("failed step context = %#v", first)
	}
	if first.ID != "problem-1" || first.SuggestedAction != "project_settings" {
		t.Fatalf("failed step metadata = %#v", first)
	}
}

func TestAnalyzeBuildProblemsClassifiesTimeout(t *testing.T) {
	report := AnalyzeBuildProblems("failed", strings.Join([]string{
		"[10:00:01] [Package] === Stage: Package ===",
		"[10:00:02] [Package] --- Step: Bundle ---",
		"[10:00:03] [Package] ERROR: context deadline exceeded",
	}, "\n"))
	if len(report.Problems) == 0 || report.Problems[0].Code != "step_timeout" {
		t.Fatalf("timeout problem = %#v", report.Problems)
	}
}

func TestAnalyzeBuildProblemsExplainsCancellation(t *testing.T) {
	report := AnalyzeBuildProblems("cancelled", "")
	if len(report.Problems) != 1 || report.Problems[0].Severity != "warning" || report.Problems[0].SuggestedAction != "retry" {
		t.Fatalf("cancelled report = %#v", report)
	}
}

func TestBuildProblemRegistryDeduplicatesStrategyOutput(t *testing.T) {
	strategy := duplicateProblemStrategy{}
	report := NewBuildProblemRegistry(strategy, strategy).Analyze(BuildProblemContext{Status: "failed"})
	if len(report.Problems) != 1 {
		t.Fatalf("deduplicated problem count = %d, want 1", len(report.Problems))
	}
}

type duplicateProblemStrategy struct{}

func (duplicateProblemStrategy) Analyze(BuildProblemContext) []BuildProblem {
	return []BuildProblem{{Code: "same", Message: "same", SuggestedAction: "inspect_logs"}}
}

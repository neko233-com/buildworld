package engine

import (
	"fmt"
	"strings"
)

const BuildProblemReportVersion = 1

// BuildProblem is a stable, UI-ready diagnosis derived from durable build
// output. Code and SuggestedAction are intentionally machine-readable so new
// renderers and automation can be added without parsing localized text.
type BuildProblem struct {
	ID              string `json:"id"`
	Code            string `json:"code"`
	Severity        string `json:"severity"`
	Stage           string `json:"stage,omitempty"`
	Step            string `json:"step,omitempty"`
	Message         string `json:"message"`
	Excerpt         string `json:"excerpt,omitempty"`
	Line            int    `json:"line,omitempty"`
	SuggestedAction string `json:"suggested_action"`
}

type BuildProblemReport struct {
	Version         int            `json:"version"`
	BuildStatus     string         `json:"build_status"`
	Summary         string         `json:"summary"`
	FailedStepCount int            `json:"failed_step_count"`
	Problems        []BuildProblem `json:"problems"`
}

type BuildProblemContext struct {
	Status string
	Log    string
}

// BuildProblemStrategy is the extension point for language-, toolchain-, or
// organization-specific diagnostics. Strategies append independent findings;
// the registry handles stable IDs and duplicate suppression.
type BuildProblemStrategy interface {
	Analyze(BuildProblemContext) []BuildProblem
}

type BuildProblemRegistry struct {
	strategies []BuildProblemStrategy
}

func NewBuildProblemRegistry(strategies ...BuildProblemStrategy) *BuildProblemRegistry {
	return &BuildProblemRegistry{strategies: append([]BuildProblemStrategy(nil), strategies...)}
}

func (r *BuildProblemRegistry) Analyze(context BuildProblemContext) BuildProblemReport {
	report := BuildProblemReport{
		Version: BuildProblemReportVersion, BuildStatus: context.Status,
		Problems: []BuildProblem{},
	}
	seen := make(map[string]struct{})
	for _, strategy := range r.strategies {
		for _, problem := range strategy.Analyze(context) {
			key := strings.Join([]string{problem.Code, problem.Stage, problem.Step, problem.Message}, "\x00")
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			problem.ID = fmt.Sprintf("problem-%d", len(report.Problems)+1)
			if problem.Severity == "" {
				problem.Severity = "error"
			}
			report.Problems = append(report.Problems, problem)
			if problem.Step != "" {
				report.FailedStepCount++
			}
		}
	}
	if len(report.Problems) > 0 {
		report.Summary = report.Problems[0].Message
	}
	return report
}

type stepFailureProblemStrategy struct{}

func (stepFailureProblemStrategy) Analyze(context BuildProblemContext) []BuildProblem {
	if context.Status != "failed" {
		return nil
	}
	var problems []BuildProblem
	stage, step := "", ""
	for index, line := range strings.Split(context.Log, "\n") {
		message := buildLogMessage(line)
		switch {
		case strings.HasPrefix(message, "=== Stage: ") && strings.HasSuffix(message, " ==="):
			stage = strings.TrimSuffix(strings.TrimPrefix(message, "=== Stage: "), " ===")
		case strings.HasPrefix(message, "--- Step: ") && strings.HasSuffix(message, " ---"):
			step = strings.TrimSuffix(strings.TrimPrefix(message, "--- Step: "), " ---")
		case strings.HasPrefix(message, "ERROR:"):
			detail := strings.TrimSpace(strings.TrimPrefix(message, "ERROR:"))
			code, action := classifyBuildFailure(detail)
			problems = append(problems, BuildProblem{
				Code: code, Stage: stage, Step: step, Message: detail,
				Excerpt: message, Line: index + 1, SuggestedAction: action,
			})
		}
	}
	return problems
}

type terminalBuildProblemStrategy struct{}

func (terminalBuildProblemStrategy) Analyze(context BuildProblemContext) []BuildProblem {
	if context.Status == "cancelled" {
		return []BuildProblem{{
			Code: "build_cancelled", Severity: "warning",
			Message:         "Build was cancelled before all steps completed",
			SuggestedAction: "retry",
		}}
	}
	if context.Status == "rejected" {
		return []BuildProblem{{
			Code: "approval_rejected", Severity: "warning",
			Message:         "Build approval was rejected",
			SuggestedAction: "approval",
		}}
	}
	if context.Status != "failed" {
		return nil
	}
	for index, line := range strings.Split(context.Log, "\n") {
		message := buildLogMessage(line)
		if !strings.HasPrefix(message, "BUILD FAILED:") {
			continue
		}
		detail := strings.TrimSpace(strings.TrimPrefix(message, "BUILD FAILED:"))
		code, action := classifyBuildFailure(detail)
		return []BuildProblem{{
			Code: code, Message: detail, Excerpt: message,
			Line: index + 1, SuggestedAction: action,
		}}
	}
	return []BuildProblem{{
		Code: "build_failed", Message: "Build failed without a structured error message",
		SuggestedAction: "inspect_logs",
	}}
}

func classifyBuildFailure(message string) (string, string) {
	normalized := strings.ToLower(message)
	switch {
	case strings.Contains(normalized, "timed out"), strings.Contains(normalized, "deadline exceeded"):
		return "step_timeout", "project_settings"
	case strings.Contains(normalized, "workspace"):
		return "workspace_error", "worker"
	case strings.Contains(normalized, "environment"), strings.Contains(normalized, "runtime"):
		return "environment_error", "worker"
	case strings.Contains(normalized, "no matching remote worker"), strings.Contains(normalized, "worker"):
		return "worker_unavailable", "worker"
	default:
		return "step_failed", "project_settings"
	}
}

var defaultBuildProblemRegistry = NewBuildProblemRegistry(
	stepFailureProblemStrategy{},
	terminalBuildProblemStrategy{},
)

func AnalyzeBuildProblems(status, logText string) BuildProblemReport {
	return defaultBuildProblemRegistry.Analyze(BuildProblemContext{Status: status, Log: logText})
}

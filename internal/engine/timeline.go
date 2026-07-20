package engine

import (
	"encoding/json"
	"strings"
)

const timelinePlanMarker = "::buildworld:plan "

type TimelinePlanStage struct {
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
}

type TimelinePlan struct {
	Stages []TimelinePlanStage `json:"stages"`
}

type BuildTimelineStep struct {
	Index  int    `json:"index"`
	Stage  string `json:"stage"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type BuildTimeline struct {
	BuildStatus    string              `json:"build_status"`
	CurrentStep    int                 `json:"current_step"`
	TotalSteps     int                 `json:"total_steps"`
	CompletedSteps int                 `json:"completed_steps"`
	Steps          []BuildTimelineStep `json:"steps"`
}

func timelinePlanFromConfig(config *BuildConfig) TimelinePlan {
	plan := TimelinePlan{Stages: make([]TimelinePlanStage, 0, len(config.Stages))}
	for _, stage := range config.Stages {
		steps := make([]string, 0, len(stage.Steps))
		for _, step := range stage.Steps {
			name := step.Name
			if name == "" {
				name = step.Type
			}
			steps = append(steps, name)
		}
		plan.Stages = append(plan.Stages, TimelinePlanStage{Name: stage.Name, Steps: steps})
	}
	return plan
}

func encodeTimelinePlan(config *BuildConfig) string {
	data, _ := json.Marshal(timelinePlanFromConfig(config))
	return timelinePlanMarker + string(data)
}

func timelineSteps(plan TimelinePlan) []BuildTimelineStep {
	var steps []BuildTimelineStep
	for _, stage := range plan.Stages {
		for _, name := range stage.Steps {
			steps = append(steps, BuildTimelineStep{
				Index: len(steps), Stage: stage.Name, Name: name, Status: "pending",
			})
		}
	}
	return steps
}

func buildLogMessage(line string) string {
	first := strings.Index(line, "] ")
	if first < 0 {
		return strings.TrimSpace(line)
	}
	second := strings.Index(line[first+2:], "] ")
	if second < 0 {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(line[first+2+second+2:])
}

func findTimelineStep(steps []BuildTimelineStep, stage, name string, after int) int {
	for i := after + 1; i < len(steps); i++ {
		if steps[i].Stage == stage && steps[i].Name == name {
			return i
		}
	}
	for i := 0; i < len(steps); i++ {
		if steps[i].Stage == stage && steps[i].Name == name && steps[i].Status == "pending" {
			return i
		}
	}
	return -1
}

func appendLegacyStep(steps []BuildTimelineStep, stage, name string) ([]BuildTimelineStep, int) {
	steps = append(steps, BuildTimelineStep{
		Index: len(steps), Stage: stage, Name: name, Status: "pending",
	})
	return steps, len(steps) - 1
}

// ParseBuildTimeline turns the durable build log into an ordered, UI-ready
// execution timeline. New builds carry a plan snapshot; legacy logs are
// reconstructed from their stage and step markers.
func ParseBuildTimeline(buildStatus, logText string) BuildTimeline {
	var plan TimelinePlan
	for _, line := range strings.Split(logText, "\n") {
		message := buildLogMessage(line)
		if !strings.HasPrefix(message, timelinePlanMarker) {
			continue
		}
		_ = json.Unmarshal([]byte(strings.TrimPrefix(message, timelinePlanMarker)), &plan)
	}

	steps := timelineSteps(plan)
	currentStage := ""
	active := -1
	lastSeen := -1

	for _, line := range strings.Split(logText, "\n") {
		message := buildLogMessage(line)
		switch {
		case strings.HasPrefix(message, "=== Stage: ") && strings.HasSuffix(message, " ==="):
			currentStage = strings.TrimSuffix(strings.TrimPrefix(message, "=== Stage: "), " ===")
		case strings.HasPrefix(message, "--- Step: ") && strings.HasSuffix(message, " ---"):
			name := strings.TrimSuffix(strings.TrimPrefix(message, "--- Step: "), " ---")
			if active >= 0 && steps[active].Status == "running" {
				steps[active].Status = "success"
			}
			index := findTimelineStep(steps, currentStage, name, lastSeen)
			if index < 0 {
				steps, index = appendLegacyStep(steps, currentStage, name)
			}
			steps[index].Status = "running"
			active = index
			lastSeen = index
		case strings.HasPrefix(message, "--- Step complete: ") && strings.HasSuffix(message, " ---"):
			name := strings.TrimSuffix(strings.TrimPrefix(message, "--- Step complete: "), " ---")
			index := findTimelineStep(steps, currentStage, name, active-1)
			if active >= 0 && steps[active].Name == name {
				index = active
			}
			if index >= 0 {
				steps[index].Status = "success"
				if active == index {
					active = -1
				}
			}
		case strings.HasPrefix(message, "ERROR:"), strings.HasPrefix(message, "BUILD FAILED:"):
			if active >= 0 {
				steps[active].Status = "failed"
			}
		}
	}

	switch buildStatus {
	case "success":
		for i := range steps {
			steps[i].Status = "success"
		}
	case "failed":
		if active >= 0 && steps[active].Status == "running" {
			steps[active].Status = "failed"
		}
		for i := range steps {
			if steps[i].Status == "pending" {
				steps[i].Status = "skipped"
			}
		}
	case "cancelled":
		if active >= 0 && (steps[active].Status == "running" || steps[active].Status == "failed") {
			steps[active].Status = "cancelled"
		}
		for i := range steps {
			if steps[i].Status == "pending" {
				steps[i].Status = "skipped"
			}
		}
	case "pending":
		for i := range steps {
			steps[i].Status = "pending"
		}
		active = -1
	}

	current := -1
	completed := 0
	for i := range steps {
		if steps[i].Status == "success" {
			completed++
		}
		if steps[i].Status == "running" || steps[i].Status == "failed" || steps[i].Status == "cancelled" {
			current = i
		}
	}
	if current < 0 && buildStatus == "success" && len(steps) > 0 {
		current = len(steps) - 1
	}
	return BuildTimeline{
		BuildStatus: buildStatus, CurrentStep: current, TotalSteps: len(steps),
		CompletedSteps: completed, Steps: steps,
	}
}

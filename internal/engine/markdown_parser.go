package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	markdownHeading  = regexp.MustCompile(`(?m)^###\s+(.+?)\s*$`)
	markdownFence    = regexp.MustCompile("(?s)```(default|macos|windows)\\s*([^\\n]*)\\n(.*?)```")
	markdownVar      = regexp.MustCompile(`^\s*-\s*([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(.+?)\s*$`)
	markdownRetain   = regexp.MustCompile(`^\s*-\s*completed\s*:\s*(-?\d+)\s*$`)
	markdownCron     = regexp.MustCompile(`^\s*-\s*cron\s*:\s*["']?([^"']+?)["']?\s*$`)
	markdownTool     = regexp.MustCompile(`^\s*-\s*([A-Za-z][A-Za-z0-9_-]*)\s*:\s*(.+?)\s*$`)
	markdownList     = regexp.MustCompile(`^\s*-\s*(.+?)\s*$`)
	markdownNeeds    = regexp.MustCompile(`(?m)^\s*-\s*needs\s*:\s*(.+?)\s*$`)
	markdownApproval = regexp.MustCompile(`^\s*-\s*([A-Za-z_][A-Za-z0-9_-]*)\s*:\s*(.+?)\s*$`)
)

// ParseMarkdownConfig parses a Buildworld Markdown pipeline. A level-three heading
// becomes a graph node/stage; default fences run everywhere and platform fences run
// afterwards on matching workers.
func ParseMarkdownConfig(source string) (*BuildConfig, error) {
	if !strings.Contains(source, "## Pipeline") {
		return nil, fmt.Errorf("Buildworld Markdown requires a ## Pipeline section")
	}
	cfg := &BuildConfig{Environment: map[string]string{}, Toolchains: map[string][]string{}, Triggers: []Trigger{{Type: "manual", Config: map[string]string{}}}}
	lines := strings.Split(source, "\n")
	inVariables := false
	inRetention := false
	inToolchains := false
	inSchedule := false
	inArtifacts := false
	inAgents := false
	inApproval := false
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") && cfg.Name == "" {
			cfg.Name = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
		if strings.TrimSpace(line) == "## Variables" {
			inVariables = true
			inRetention = false
			inToolchains = false
			inSchedule = false
			inArtifacts = false
			inAgents = false
			inApproval = false
			continue
		}
		if strings.TrimSpace(line) == "## Retention" {
			inRetention = true
			inVariables = false
			inToolchains = false
			inSchedule = false
			inArtifacts = false
			inAgents = false
			inApproval = false
			continue
		}
		if strings.TrimSpace(line) == "## Toolchains" {
			inToolchains = true
			inVariables = false
			inRetention = false
			inSchedule = false
			inArtifacts = false
			inAgents = false
			inApproval = false
			continue
		}
		if strings.TrimSpace(line) == "## Schedule" {
			inSchedule = true
			inVariables = false
			inRetention = false
			inToolchains = false
			inArtifacts = false
			inAgents = false
			inApproval = false
			continue
		}
		if strings.TrimSpace(line) == "## Artifacts" {
			inArtifacts = true
			inVariables = false
			inRetention = false
			inToolchains = false
			inSchedule = false
			inAgents = false
			inApproval = false
			continue
		}
		if strings.TrimSpace(line) == "## Agents" {
			inAgents = true
			inVariables = false
			inRetention = false
			inToolchains = false
			inSchedule = false
			inArtifacts = false
			inApproval = false
			continue
		}
		if strings.TrimSpace(line) == "## Approval" {
			inApproval = true
			inVariables = false
			inRetention = false
			inToolchains = false
			inSchedule = false
			inArtifacts = false
			inAgents = false
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			inVariables = false
			inRetention = false
			inToolchains = false
			inSchedule = false
			inArtifacts = false
			inAgents = false
			inApproval = false
		}
		if inVariables {
			if m := markdownVar.FindStringSubmatch(line); len(m) == 3 {
				cfg.Environment[m[1]] = strings.Trim(m[2], "\"'")
			}
		}
		if inRetention {
			if m := markdownRetain.FindStringSubmatch(line); len(m) == 2 {
				cfg.RetentionCompleted, _ = strconv.Atoi(m[1])
			}
		}
		if inToolchains {
			if m := markdownTool.FindStringSubmatch(line); len(m) == 3 {
				for _, version := range strings.Split(m[2], ",") {
					if version = strings.TrimSpace(version); version != "" {
						cfg.Toolchains[strings.ToLower(m[1])] = append(cfg.Toolchains[strings.ToLower(m[1])], version)
					}
				}
			}
		}
		if inSchedule {
			if m := markdownCron.FindStringSubmatch(line); len(m) == 2 {
				cfg.Triggers = append(cfg.Triggers, Trigger{Type: "schedule", Config: map[string]string{"cron": strings.TrimSpace(m[1])}})
			}
		}
		if inArtifacts {
			if m := markdownList.FindStringSubmatch(line); len(m) == 2 {
				if pattern := strings.Trim(strings.TrimSpace(m[1]), "\"'"); pattern != "" {
					cfg.Artifacts = append(cfg.Artifacts, pattern)
				}
			}
		}
		if inAgents {
			if m := markdownList.FindStringSubmatch(line); len(m) == 2 {
				appendAgentRequirements(cfg, m[1])
			}
		}
		if inApproval {
			if cfg.Approval == nil {
				cfg.Approval = &ApprovalPolicy{}
			}
			if m := markdownApproval.FindStringSubmatch(line); len(m) == 3 {
				key := strings.ToLower(strings.TrimSpace(m[1]))
				value := strings.Trim(strings.TrimSpace(m[2]), "\"'")
				switch key {
				case "version":
					cfg.Approval.Version, _ = strconv.Atoi(value)
				case "strategy":
					cfg.Approval.Strategy = value
				case "roles", "required_roles":
					cfg.Approval.RequiredRoles = nil
					for _, role := range strings.Split(value, ",") {
						if role = strings.TrimSpace(role); role != "" {
							cfg.Approval.RequiredRoles = append(cfg.Approval.RequiredRoles, role)
						}
					}
				case "allow_requester":
					allowed, _ := strconv.ParseBool(value)
					cfg.Approval.AllowRequester = &allowed
				case "prompt":
					cfg.Approval.Prompt = value
				}
			}
		}
	}

	headings := markdownHeading.FindAllStringSubmatchIndex(source, -1)
	for i, heading := range headings {
		start, end := heading[1], len(source)
		if i+1 < len(headings) {
			end = headings[i+1][0]
		}
		section := source[start:end]
		matches := markdownFence.FindAllStringSubmatch(section, -1)
		if len(matches) == 0 {
			continue
		}
		stage := Stage{Name: strings.TrimSpace(source[heading[2]:heading[3]])}
		if match := markdownNeeds.FindStringSubmatch(section); len(match) == 2 {
			for _, dependency := range strings.Split(match[1], ",") {
				if dependency = strings.Trim(strings.TrimSpace(dependency), "\"'"); dependency != "" {
					stage.DependsOn = append(stage.DependsOn, dependency)
				}
			}
		}
		step := Step{Name: stage.Name, Type: "shell", Config: map[string]string{}, PlatformAdditions: map[string]string{}}
		for _, block := range matches {
			platform, header, command := block[1], strings.Fields(block[2]), strings.TrimSpace(block[3])
			if len(header) > 0 {
				step.Type = header[0]
			}
			if len(header) > 1 {
				step.Runtime = header[1]
			}
			if platform == "default" {
				step.Command = command
			} else {
				step.PlatformAdditions[platform] = command
			}
		}
		if step.Command == "" && len(step.PlatformAdditions) > 0 {
			return nil, fmt.Errorf("step %q needs a default code fence", stage.Name)
		}
		stage.Steps = []Step{step}
		cfg.Stages = append(cfg.Stages, stage)
	}
	if len(cfg.Stages) == 0 {
		return nil, fmt.Errorf("Buildworld Markdown has no executable ### steps")
	}
	ordered, err := orderMarkdownStages(cfg.Stages)
	if err != nil {
		return nil, err
	}
	cfg.Stages = ordered
	if _, err := ResolveApprovalPolicy(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// orderMarkdownStages validates graph references and returns a stable topological
// order. Executors can run the result sequentially while Markdown remains the
// source of truth for the dependency graph.
func orderMarkdownStages(stages []Stage) ([]Stage, error) {
	byName := make(map[string]int, len(stages))
	for index, stage := range stages {
		if stage.Name == "" {
			return nil, fmt.Errorf("Buildworld Markdown has a step without a name")
		}
		if _, exists := byName[stage.Name]; exists {
			return nil, fmt.Errorf("duplicate Markdown step %q", stage.Name)
		}
		byName[stage.Name] = index
	}
	state := make([]uint8, len(stages))
	ordered := make([]Stage, 0, len(stages))
	var visit func(int) error
	visit = func(index int) error {
		switch state[index] {
		case 1:
			return fmt.Errorf("cycle detected at Markdown step %q", stages[index].Name)
		case 2:
			return nil
		}
		state[index] = 1
		for _, dependency := range stages[index].DependsOn {
			dependencyIndex, exists := byName[dependency]
			if !exists {
				return fmt.Errorf("Markdown step %q depends on unknown step %q", stages[index].Name, dependency)
			}
			if err := visit(dependencyIndex); err != nil {
				return err
			}
		}
		state[index] = 2
		ordered = append(ordered, stages[index])
		return nil
	}
	for index := range stages {
		if err := visit(index); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func appendAgentRequirements(cfg *BuildConfig, value string) {
	value = strings.Trim(strings.TrimSpace(value), "\"'")
	if value == "" {
		return
	}
	if key, rest, found := strings.Cut(value, ":"); found {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(rest), "\"'")
		if key == "pool" {
			if value != "" {
				cfg.AgentRequirements = append(cfg.AgentRequirements, "pool="+value)
			}
			return
		}
		if key != "label" && key != "labels" && key != "requires" {
			return
		}
	}
	for _, label := range strings.Split(value, ",") {
		if label = strings.TrimSpace(label); label != "" {
			cfg.AgentRequirements = append(cfg.AgentRequirements, label)
		}
	}
}

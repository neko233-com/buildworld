package engine

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type YAMLBuildConfig struct {
	Name               string              `yaml:"name"`
	Description        string              `yaml:"description"`
	Parameters         []BuildParameter    `yaml:"parameters"`
	Approval           *ApprovalPolicy     `yaml:"approval"`
	On                 interface{}         `yaml:"on"`
	Env                map[string]string   `yaml:"env"`
	Defaults           YAMLDefaults        `yaml:"defaults"`
	Jobs               YAMLJobs            `yaml:"jobs"`
	Artifacts          []string            `yaml:"artifacts"`
	AgentRequirements  []string            `yaml:"agent_requirements"`
	RetentionCompleted int                 `yaml:"retention_completed"`
	Toolchains         map[string][]string `yaml:"toolchains"`
}

type YAMLJobs []YAMLJob

func (jobs *YAMLJobs) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == 0 || node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return fmt.Errorf("jobs must be a non-empty mapping")
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("jobs must be a non-empty mapping")
	}
	if len(node.Content) == 0 {
		return fmt.Errorf("jobs must be a non-empty mapping")
	}
	seen := make(map[string]struct{}, len(node.Content)/2)
	for index := 0; index < len(node.Content); index += 2 {
		key := strings.TrimSpace(node.Content[index].Value)
		if key == "" {
			return fmt.Errorf("job id cannot be empty")
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("job %q is declared more than once", key)
		}
		seen[key] = struct{}{}
		if err := validateYAMLMappingKeys(node.Content[index+1], "job "+strconv.Quote(key),
			"name", "needs", "runs-on", "if", "env", "steps", "timeout-minutes",
			"parallel", "working-directory", "defaults",
		); err != nil {
			return err
		}
		var job YAMLJob
		if err := node.Content[index+1].Decode(&job); err != nil {
			return fmt.Errorf("job %q: %w", key, err)
		}
		if len(job.Steps) == 0 {
			return fmt.Errorf("job %q must contain at least one step", key)
		}
		job.ID = key
		*jobs = append(*jobs, job)
	}
	return nil
}

type YAMLStringList []string

func (values *YAMLStringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" || strings.TrimSpace(node.Value) == "" {
			return nil
		}
		*values = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode || item.Tag == "!!null" {
				return fmt.Errorf("expected a string list")
			}
			*values = append(*values, item.Value)
		}
		return nil
	default:
		return fmt.Errorf("expected a string or string list")
	}
}

type YAMLTimeoutMinutes struct {
	Set   bool
	Value float64
}

func (minutes *YAMLTimeoutMinutes) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag == "!!null" {
		return fmt.Errorf("timeout-minutes must be a positive number")
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(node.Value), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return fmt.Errorf("timeout-minutes must be a positive number")
	}
	minutes.Set = true
	minutes.Value = value
	return nil
}

type YAMLDefaults struct {
	Run YAMLRunDefaults `yaml:"run"`
}

func (defaults *YAMLDefaults) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil
	}
	if err := validateYAMLMappingKeys(node, "defaults", "run"); err != nil {
		return err
	}
	type plain YAMLDefaults
	var decoded plain
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*defaults = YAMLDefaults(decoded)
	return nil
}

type YAMLRunDefaults struct {
	Shell      string `yaml:"shell"`
	WorkingDir string `yaml:"working-directory"`
}

func (defaults *YAMLRunDefaults) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil
	}
	if err := validateYAMLMappingKeys(node, "run defaults", "shell", "working-directory"); err != nil {
		return err
	}
	type plain YAMLRunDefaults
	var decoded plain
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*defaults = YAMLRunDefaults(decoded)
	return nil
}

type YAMLJob struct {
	ID         string             `yaml:"-"`
	Name       string             `yaml:"name"`
	Needs      YAMLStringList     `yaml:"needs"`
	RunsOn     YAMLStringList     `yaml:"runs-on"`
	If         string             `yaml:"if"`
	Env        map[string]string  `yaml:"env"`
	Steps      []YAMLStep         `yaml:"steps"`
	Timeout    YAMLTimeoutMinutes `yaml:"timeout-minutes"`
	Parallel   bool               `yaml:"parallel"`
	WorkingDir string             `yaml:"working-directory"`
	Defaults   YAMLDefaults       `yaml:"defaults"`
}

type YAMLStep struct {
	Name       string            `yaml:"name"`
	ID         string            `yaml:"id"`
	If         string            `yaml:"if"`
	Run        string            `yaml:"run"`
	Shell      string            `yaml:"shell"`
	WorkingDir string            `yaml:"working-directory"`
	Env        map[string]string `yaml:"env"`
	Uses       string            `yaml:"uses"`
	With       map[string]string `yaml:"with"`
	Type       string            `yaml:"type"`
	Command    string            `yaml:"command"`
}

func (step *YAMLStep) UnmarshalYAML(node *yaml.Node) error {
	if err := validateYAMLMappingKeys(node, "step",
		"name", "id", "if", "run", "shell", "working-directory", "env",
		"uses", "with", "type", "command",
	); err != nil {
		return err
	}
	type plain YAMLStep
	var decoded plain
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*step = YAMLStep(decoded)
	return nil
}

func ParseYAMLConfig(yamlStr string) (*BuildConfig, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(yamlStr), &document); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	var yc YAMLBuildConfig
	decoder := yaml.NewDecoder(strings.NewReader(yamlStr))
	decoder.KnownFields(true)
	if err := decoder.Decode(&yc); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	if err := validateYAMLJobsDocument(document.Content); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	cfg := &BuildConfig{
		Name:               yc.Name,
		Description:        yc.Description,
		Parameters:         yc.Parameters,
		Approval:           yc.Approval,
		Environment:        make(map[string]string),
		Triggers:           parseTriggers(yc.On),
		Artifacts:          yc.Artifacts,
		AgentRequirements:  append([]string(nil), yc.AgentRequirements...),
		RetentionCompleted: yc.RetentionCompleted,
		Toolchains:         yc.Toolchains,
	}
	for key, value := range yc.Env {
		cfg.Environment[key] = value
	}

	orderedJobs, err := stableTopologicalJobs(yc.Jobs)
	if err != nil {
		return nil, err
	}
	for _, job := range orderedJobs {
		if err := ValidatePipelineCondition(job.If); err != nil {
			return nil, fmt.Errorf("job %q if: %w", job.ID, err)
		}
		defaults := yc.Defaults.Run
		if job.Defaults.Run.Shell != "" {
			defaults.Shell = job.Defaults.Run.Shell
		}
		if job.Defaults.Run.WorkingDir != "" {
			defaults.WorkingDir = job.Defaults.Run.WorkingDir
		}
		if job.WorkingDir != "" {
			defaults.WorkingDir = job.WorkingDir
		}
		steps, err := convertSteps(defaults, job.Steps)
		if err != nil {
			return nil, fmt.Errorf("job %q: %w", job.ID, err)
		}
		stageName := job.ID
		if strings.TrimSpace(job.Name) != "" {
			stageName = job.Name
		}
		stage := Stage{
			ID:               job.ID,
			Name:             stageName,
			Parallel:         job.Parallel,
			DependsOn:        append([]string(nil), job.Needs...),
			If:               job.If,
			Environment:      cloneStringMap(job.Env),
			WorkingDirectory: defaults.WorkingDir,
			Steps:            steps,
		}
		if job.Timeout.Set {
			seconds := math.Ceil(job.Timeout.Value * 60)
			if seconds > math.MaxInt {
				return nil, fmt.Errorf("job %q timeout-minutes is too large", job.ID)
			}
			stage.TimeoutSec = int(seconds)
		}
		cfg.Stages = append(cfg.Stages, stage)
	}

	if err := applyYAMLRunsOn(cfg, yc.Jobs); err != nil {
		return nil, err
	}
	if len(cfg.Triggers) == 0 {
		cfg.Triggers = append(cfg.Triggers, Trigger{Type: "manual", Config: map[string]string{}})
	}
	return cfg, nil
}

func validateYAMLJobsDocument(nodes []*yaml.Node) error {
	if len(nodes) == 0 {
		return fmt.Errorf("jobs must be a non-empty mapping")
	}
	root := nodes[0]
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return fmt.Errorf("jobs must be a non-empty mapping")
		}
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("pipeline root must be a mapping with jobs")
	}
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value != "jobs" {
			continue
		}
		jobs := root.Content[index+1]
		if jobs.Kind == yaml.ScalarNode && jobs.Tag == "!!null" ||
			jobs.Kind == yaml.MappingNode && len(jobs.Content) == 0 {
			return fmt.Errorf("jobs must be a non-empty mapping")
		}
		return nil
	}
	return fmt.Errorf("jobs must be a non-empty mapping")
}

func stableTopologicalJobs(jobs YAMLJobs) ([]YAMLJob, error) {
	indices := make(map[string]int, len(jobs))
	for index, job := range jobs {
		indices[job.ID] = index
	}
	indegree := make([]int, len(jobs))
	dependents := make([][]int, len(jobs))
	for index, job := range jobs {
		seen := make(map[string]struct{}, len(job.Needs))
		for _, rawDependency := range job.Needs {
			dependency := strings.TrimSpace(rawDependency)
			if dependency == "" {
				return nil, fmt.Errorf("job %q has an empty dependency", job.ID)
			}
			if _, duplicate := seen[dependency]; duplicate {
				return nil, fmt.Errorf("job %q repeats dependency %q", job.ID, dependency)
			}
			seen[dependency] = struct{}{}
			dependencyIndex, exists := indices[dependency]
			if !exists {
				return nil, fmt.Errorf("job %q depends on unknown job %q", job.ID, dependency)
			}
			indegree[index]++
			dependents[dependencyIndex] = append(dependents[dependencyIndex], index)
			job.Needs[len(seen)-1] = dependency
		}
		jobs[index] = job
	}

	processed := make([]bool, len(jobs))
	ordered := make([]YAMLJob, 0, len(jobs))
	for len(ordered) < len(jobs) {
		next := -1
		for index := range jobs {
			if !processed[index] && indegree[index] == 0 {
				next = index
				break
			}
		}
		if next < 0 {
			var cycle []string
			for index, job := range jobs {
				if !processed[index] {
					cycle = append(cycle, job.ID)
				}
			}
			return nil, fmt.Errorf("jobs contain a dependency cycle: %s", strings.Join(cycle, " -> "))
		}
		processed[next] = true
		ordered = append(ordered, jobs[next])
		for _, dependent := range dependents[next] {
			indegree[dependent]--
		}
	}
	return ordered, nil
}

func applyYAMLRunsOn(cfg *BuildConfig, jobs YAMLJobs) error {
	type target struct {
		job    string
		labels YAMLStringList
	}
	hasDirective := false
	for _, job := range jobs {
		hasDirective = hasDirective || job.RunsOn != nil
	}
	if !hasDirective {
		return nil
	}
	targets := make([]target, 0, len(jobs))
	for _, job := range jobs {
		targets = append(targets, target{job: job.ID, labels: job.RunsOn})
	}
	if len(targets) == 0 {
		return nil
	}

	var expected []string
	expectedLocal := false
	for index, item := range targets {
		labels, local, err := normalizeRunsOn(item.labels)
		if err != nil {
			return fmt.Errorf("%s runs-on: %w", item.job, err)
		}
		if index == 0 {
			expected, expectedLocal = labels, local
			continue
		}
		if local != expectedLocal || !equalStrings(labels, expected) {
			return fmt.Errorf("jobs must use one consistent runs-on target; %s differs from %s", item.job, targets[0].job)
		}
	}
	if expectedLocal {
		if len(cfg.AgentRequirements) > 0 {
			return fmt.Errorf("runs-on local conflicts with agent_requirements")
		}
		return nil
	}
	if len(cfg.AgentRequirements) == 0 {
		cfg.AgentRequirements = expected
		return nil
	}
	explicit := append([]string(nil), cfg.AgentRequirements...)
	sort.Strings(explicit)
	if !equalStrings(explicit, expected) {
		return fmt.Errorf("runs-on %s conflicts with agent_requirements %s", strings.Join(expected, ", "), strings.Join(explicit, ", "))
	}
	cfg.AgentRequirements = explicit
	return nil
}

func normalizeRunsOn(raw YAMLStringList) ([]string, bool, error) {
	if len(raw) == 0 {
		return nil, true, nil
	}
	seen := make(map[string]struct{}, len(raw))
	labels := make([]string, 0, len(raw))
	for _, value := range raw {
		label := strings.TrimSpace(value)
		if label == "" {
			return nil, false, fmt.Errorf("label cannot be empty")
		}
		if _, duplicate := seen[label]; duplicate {
			return nil, false, fmt.Errorf("label %q is repeated", label)
		}
		seen[label] = struct{}{}
		labels = append(labels, label)
	}
	if len(labels) == 1 && strings.EqualFold(labels[0], "local") {
		return nil, true, nil
	}
	for _, label := range labels {
		if strings.EqualFold(label, "local") {
			return nil, false, fmt.Errorf("local cannot be combined with remote labels")
		}
	}
	sort.Strings(labels)
	return labels, false, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func parseTriggers(on interface{}) []Trigger {
	var triggers []Trigger
	switch value := on.(type) {
	case string:
		triggers = append(triggers, triggerFromString(value))
	case []interface{}:
		for _, item := range value {
			if text, ok := item.(string); ok {
				triggers = append(triggers, triggerFromString(text))
			}
		}
	case map[string]interface{}:
		for key, config := range value {
			switch key {
			case "push", "webhook":
				triggers = append(triggers, Trigger{Type: "webhook", Config: map[string]string{}})
			case "schedule":
				if entries, ok := config.([]interface{}); ok {
					for _, entry := range entries {
						if values, ok := entry.(map[string]interface{}); ok {
							if cron, ok := values["cron"].(string); ok {
								triggers = append(triggers, Trigger{Type: "schedule", Config: map[string]string{"cron": cron}})
							}
						}
					}
				}
			default:
				triggers = append(triggers, triggerFromString(key))
			}
		}
	}
	for _, trigger := range triggers {
		if trigger.Type == "manual" {
			return triggers
		}
	}
	return append(triggers, Trigger{Type: "manual", Config: map[string]string{}})
}

func triggerFromString(value string) Trigger {
	switch value {
	case "push", "webhook":
		return Trigger{Type: "webhook", Config: map[string]string{}}
	case "schedule":
		return Trigger{Type: "schedule", Config: map[string]string{}}
	default:
		return Trigger{Type: "manual", Config: map[string]string{}}
	}
}

func convertSteps(defaults YAMLRunDefaults, yamlSteps []YAMLStep) ([]Step, error) {
	steps := make([]Step, 0, len(yamlSteps))
	for index, yamlStep := range yamlSteps {
		if err := ValidatePipelineCondition(yamlStep.If); err != nil {
			return nil, fmt.Errorf("step %q if: %w", yamlStep.Name, err)
		}
		step, err := convertStep(yamlStep)
		if err != nil {
			return nil, err
		}
		if step.Name == "" {
			step.Name = fmt.Sprintf("Step %d", index+1)
		}
		step.If = yamlStep.If
		if step.Shell == "" {
			step.Shell = defaults.Shell
		}
		step.Config = cloneStringMap(step.Config)
		for key, value := range yamlStep.Env {
			step.Config["env."+key] = value
		}
		workingDirectory := defaults.WorkingDir
		if yamlStep.WorkingDir != "" {
			workingDirectory = yamlStep.WorkingDir
		}
		if workingDirectory != "" {
			step.Config["working-directory"] = workingDirectory
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func convertStep(yamlStep YAMLStep) (Step, error) {
	step := Step{Name: yamlStep.Name}
	hasUses := strings.TrimSpace(yamlStep.Uses) != ""
	hasRun := strings.TrimSpace(yamlStep.Run) != ""
	hasType := strings.TrimSpace(yamlStep.Type) != ""
	hasCommand := strings.TrimSpace(yamlStep.Command) != ""
	if !hasUses && !hasRun && !hasType && !hasCommand {
		return Step{}, fmt.Errorf("step %q must define one supported action: uses, run, type, or command", yamlStep.Name)
	}
	if hasUses && (hasRun || hasType || hasCommand) {
		return Step{}, fmt.Errorf("step %q cannot combine uses with run, type, or command", yamlStep.Name)
	}
	if hasRun && (hasType || hasCommand) {
		return Step{}, fmt.Errorf("step %q cannot combine run with type or command", yamlStep.Name)
	}
	if yamlStep.Uses != "" {
		action := strings.SplitN(strings.TrimSpace(yamlStep.Uses), "@", 2)[0]
		if action != "actions/checkout" {
			return Step{}, fmt.Errorf("step %q uses unsupported action %q; only actions/checkout is built in", yamlStep.Name, yamlStep.Uses)
		}
		step.Type = "git"
		step.Command = "clone"
		step.Config = cloneStringMap(yamlStep.With)
		return step, nil
	}
	if yamlStep.Run != "" {
		step.Type = "shell"
		step.Command = yamlStep.Run
		step.Shell = yamlStep.Shell
		step.Config = make(map[string]string)
		return step, nil
	}
	if yamlStep.Type != "" {
		step.Type = yamlStep.Type
		step.Command = yamlStep.Command
		step.Shell = yamlStep.Shell
		step.Config = cloneStringMap(yamlStep.With)
		return step, nil
	}
	if yamlStep.Command != "" {
		step.Type = "shell"
		step.Command = yamlStep.Command
		step.Shell = yamlStep.Shell
		step.Config = make(map[string]string)
		return step, nil
	}
	return Step{}, fmt.Errorf("step %q does not define a supported action", yamlStep.Name)
}

func cloneStringMap(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func validateYAMLMappingKeys(node *yaml.Node, context string, allowed ...string) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s must be a mapping", context)
	}
	known := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		known[key] = struct{}{}
	}
	for index := 0; index < len(node.Content); index += 2 {
		key := node.Content[index].Value
		if _, exists := known[key]; !exists {
			return fmt.Errorf("%s contains unknown field %q", context, key)
		}
	}
	return nil
}

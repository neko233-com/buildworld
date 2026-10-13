package engine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/neko233-com/buildworld/internal/releasemeta"
)

type BuildParameter struct {
	Name        string      `json:"name" yaml:"name"`
	Type        string      `json:"type" yaml:"type"`
	Description string      `json:"description" yaml:"description"`
	Default     interface{} `json:"default" yaml:"default"`
	Required    bool        `json:"required" yaml:"required"`
	Choices     []string    `json:"choices,omitempty" yaml:"choices,omitempty"`
	IsSecret    bool        `json:"is_secret" yaml:"is_secret"`
}

type BuildConfig struct {
	Name           string                `json:"name"`
	Description    string                `json:"description"`
	Parameters     []BuildParameter      `json:"parameters,omitempty"`
	ReleaseTargets *ReleaseTargetCatalog `json:"release_targets,omitempty"`
	Approval       *ApprovalPolicy       `json:"approval,omitempty"`
	Stages         []Stage               `json:"stages"`
	Triggers       []Trigger             `json:"triggers,omitempty"`
	Environment    map[string]string     `json:"environment,omitempty"`
	Artifacts      []string              `json:"artifacts,omitempty"`
	// Post contains Jenkins-style lifecycle steps keyed by always, success,
	// failure, or cleanup. They run after the normal stages on every worker.
	Post               map[string][]Step `json:"post,omitempty"`
	AgentRequirements  []string          `json:"agent_requirements,omitempty"`
	RetentionCompleted int               `json:"retention_completed"`
	TimeoutSec         int               `json:"timeout_sec,omitempty"`
	// AllowLongRunning is derived during semantic validation for an
	// unconditional terminal service observer. A source flag alone cannot
	// suppress the default finite build timeout.
	AllowLongRunning  bool                `json:"allow_long_running,omitempty"`
	DisableConcurrent bool                `json:"disable_concurrent,omitempty"`
	AbortPrevious     bool                `json:"abort_previous,omitempty"`
	Toolchains        map[string][]string `json:"toolchains,omitempty"`
}

type Stage struct {
	ID               string            `json:"id,omitempty" yaml:"id,omitempty"`
	Name             string            `json:"name" yaml:"name"`
	Steps            []Step            `json:"steps" yaml:"steps"`
	Parallel         bool              `json:"parallel,omitempty" yaml:"parallel,omitempty"`
	DependsOn        []string          `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Branches         []string          `json:"branches,omitempty" yaml:"branches,omitempty"`
	If               string            `json:"if,omitempty" yaml:"if,omitempty"`
	Environment      map[string]string `json:"environment,omitempty" yaml:"env,omitempty"`
	WorkingDirectory string            `json:"working_directory,omitempty" yaml:"working-directory,omitempty"`
	TimeoutSec       int               `json:"timeout_sec,omitempty" yaml:"timeout_sec,omitempty"`
}

type Step struct {
	Name    string            `json:"name" yaml:"name"`
	Type    string            `json:"type" yaml:"type"`
	Command string            `json:"command,omitempty" yaml:"command,omitempty"`
	Shell   string            `json:"shell,omitempty" yaml:"shell,omitempty"`
	Config  map[string]string `json:"config,omitempty" yaml:"config,omitempty"`
	// PlatformAdditions run after the shared command on a matching worker OS.
	// This keeps a default pipeline readable and removes imperative platform branches.
	PlatformAdditions map[string]string `json:"platform_additions,omitempty" yaml:"platform_additions,omitempty"`
	Runtime           string            `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	If                string            `json:"if,omitempty" yaml:"if,omitempty"`
}

type Trigger struct {
	Type   string            `json:"type"`
	Config map[string]string `json:"config"`
}

// ResolveBuildParameters copies supplied values, applies declared defaults and
// validates the result. Every value must be declared so UI, token-triggered,
// and internal callers all use the same explicit contract.
func ResolveBuildParameters(parameters []BuildParameter, supplied map[string]interface{}) (map[string]interface{}, error) {
	resolved := make(map[string]interface{}, len(supplied)+len(parameters))
	for key, value := range supplied {
		resolved[key] = value
	}

	seen := make(map[string]struct{}, len(parameters))
	for _, parameter := range parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			return nil, fmt.Errorf("build parameter name cannot be empty")
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("build parameter %s is declared more than once", name)
		}
		seen[name] = struct{}{}
		if name == "RELEASE_CHANNEL" || name == "RELEASE_ACTION" {
			if parameter.IsSecret || !strings.EqualFold(strings.TrimSpace(parameter.Type), "choice") {
				return nil, fmt.Errorf("release target parameter %s must be a public choice", name)
			}
		}
		if name == "RELEASE_ENVIRONMENT" {
			parameterType := strings.ToLower(strings.TrimSpace(parameter.Type))
			if parameter.IsSecret || (parameterType != "choice" && parameterType != "string") {
				return nil, fmt.Errorf("release target parameter RELEASE_ENVIRONMENT must be a public string or choice")
			}
		}
		if name == "RELEASE_VERSION" {
			if parameter.IsSecret || !strings.EqualFold(strings.TrimSpace(parameter.Type), "string") {
				return nil, fmt.Errorf("release metadata parameter %s must be a public string", name)
			}
		}
		if name == "RELEASE_SHARED_SERVICE" && (parameter.IsSecret || !strings.EqualFold(strings.TrimSpace(parameter.Type), "string")) {
			return nil, fmt.Errorf("RELEASE_SHARED_SERVICE must be a public string")
		}
		if name == "CONFIRM_PUBLISH_LATEST" && (parameter.IsSecret || !strings.EqualFold(strings.TrimSpace(parameter.Type), "boolean")) {
			return nil, fmt.Errorf("CONFIRM_PUBLISH_LATEST must be a public boolean")
		}
		if _, exists := resolved[name]; !exists {
			switch parameterType := strings.ToLower(strings.TrimSpace(parameter.Type)); {
			case parameter.Default != nil:
				resolved[name] = parameter.Default
			case parameterType == "boolean":
				resolved[name] = false
			case parameterType == "choice" && len(parameter.Choices) > 0:
				resolved[name] = parameter.Choices[0]
			}
		}
	}
	for name := range supplied {
		if _, declared := seen[name]; !declared {
			return nil, fmt.Errorf("build parameter %s is not declared", name)
		}
	}
	_, sharedRelease := seen["RELEASE_SHARED_SERVICE"]
	if sharedRelease {
		for _, name := range []string{"RELEASE_CHANNEL", "RELEASE_ENVIRONMENT"} {
			if _, declared := seen[name]; declared {
				return nil, fmt.Errorf("shared release parameters cannot declare channel or environment")
			}
		}
	}

	if err := ValidateBuildParameters(parameters, resolved); err != nil {
		return nil, err
	}
	if environment, exists := resolved["RELEASE_ENVIRONMENT"]; exists {
		if value, ok := environment.(string); !ok || !releasemeta.ValidEnvironment(value) {
			return nil, fmt.Errorf("RELEASE_ENVIRONMENT must match ^[a-z][a-z0-9-]{0,31}$")
		}
	}
	if sharedRelease {
		service, ok := resolved["RELEASE_SHARED_SERVICE"].(string)
		if !ok || !regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`).MatchString(service) {
			return nil, fmt.Errorf("RELEASE_SHARED_SERVICE requires a safe service ID matching ^[a-z][a-z0-9-]{0,31}$")
		}
	}
	if action, exists := resolved["RELEASE_ACTION"]; exists {
		switch action {
		case "publish-latest":
			if confirmed, ok := resolved["CONFIRM_PUBLISH_LATEST"].(bool); !ok || !confirmed {
				return nil, fmt.Errorf("publish-latest requires CONFIRM_PUBLISH_LATEST=true")
			}
		case "restart-version":
			if version, ok := resolved["RELEASE_VERSION"].(string); !ok || strings.TrimSpace(version) == "" {
				return nil, fmt.Errorf("restart-version requires RELEASE_VERSION")
			}
		default:
			return nil, fmt.Errorf("unsupported RELEASE_ACTION %v", action)
		}
		if !sharedRelease {
			for _, name := range []string{"RELEASE_CHANNEL", "RELEASE_ENVIRONMENT"} {
				if value, ok := resolved[name].(string); !ok || strings.TrimSpace(value) == "" {
					return nil, fmt.Errorf("release action requires explicit %s", name)
				}
			}
		}
	}
	return resolved, nil
}

func ValidateBuildParameters(parameters []BuildParameter, params map[string]interface{}) error {
	for _, param := range parameters {
		name := strings.TrimSpace(param.Name)
		val, exists := params[name]

		if param.Required && (!exists || val == nil || isBlankBuildParameter(val)) {
			return fmt.Errorf("required parameter %s is missing", name)
		}

		if !exists || val == nil {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(param.Type)) {
		case "", "string", "password", "text":
			if _, ok := val.(string); !ok {
				return fmt.Errorf("parameter %s: must be string", name)
			}
		case "choice":
			choice, ok := val.(string)
			if !ok {
				return fmt.Errorf("parameter %s: must be a string choice", name)
			}
			if !contains(param.Choices, choice) {
				return fmt.Errorf("parameter %s: invalid choice, must be one of %v", name, param.Choices)
			}
		case "boolean":
			if _, ok := val.(bool); !ok {
				return fmt.Errorf("parameter %s: must be boolean", name)
			}
		case "number":
			if !isNumericBuildParameter(val) {
				return fmt.Errorf("parameter %s: must be a number", name)
			}
		default:
			return fmt.Errorf("parameter %s: unsupported type %q", name, param.Type)
		}
	}
	return nil
}

func isBlankBuildParameter(value interface{}) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) == ""
}

func isNumericBuildParameter(value interface{}) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:
		return true
	default:
		return false
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// decodeTypeScriptPipelineData maps the already allowlisted AST value into the
// execution model. JSON is only an internal serialization bridge; callers must
// enter through ParsePipelineConfig.
func decodeTypeScriptPipelineData(jsonStr string) (*BuildConfig, error) {
	config := &BuildConfig{}
	if err := json.Unmarshal([]byte(jsonStr), config); err != nil {
		return nil, err
	}
	if config.Environment == nil {
		config.Environment = map[string]string{}
	}
	return config, nil
}

func ParsePipelineConfig(raw string) (*BuildConfig, error) {
	return ParsePipelineConfigWithFormat(FormatAuto, raw, "")
}

func MergeBuildConfig(template, project *BuildConfig) *BuildConfig {
	if template == nil {
		return project
	}
	if project == nil {
		return template
	}
	merged := &BuildConfig{
		Name:               project.Name,
		Description:        project.Description,
		Parameters:         append([]BuildParameter{}, template.Parameters...),
		Approval:           template.Approval,
		Stages:             append([]Stage{}, template.Stages...),
		Triggers:           append([]Trigger{}, template.Triggers...),
		Environment:        map[string]string{},
		Artifacts:          append([]string{}, template.Artifacts...),
		Post:               clonePostSteps(template.Post),
		AgentRequirements:  append([]string{}, template.AgentRequirements...),
		RetentionCompleted: template.RetentionCompleted,
		TimeoutSec:         template.TimeoutSec,
		AllowLongRunning:   template.AllowLongRunning,
		DisableConcurrent:  template.DisableConcurrent,
		AbortPrevious:      template.AbortPrevious,
	}
	if merged.Name == "" {
		merged.Name = template.Name
	}
	if merged.Description == "" {
		merged.Description = template.Description
	}
	if project.Approval != nil {
		merged.Approval = project.Approval
	}
	for k, v := range template.Environment {
		merged.Environment[k] = v
	}
	for k, v := range project.Environment {
		merged.Environment[k] = v
	}
	for _, p := range project.Parameters {
		merged.Parameters = append(merged.Parameters, p)
	}
	if len(project.Stages) > 0 {
		merged.Stages = project.Stages
	}
	merged.Triggers = append(merged.Triggers, project.Triggers...)
	if len(project.Artifacts) > 0 {
		merged.Artifacts = project.Artifacts
	}
	if len(project.Post) > 0 {
		merged.Post = clonePostSteps(project.Post)
	}
	if len(project.AgentRequirements) > 0 {
		merged.AgentRequirements = project.AgentRequirements
	}
	if project.RetentionCompleted != 0 {
		merged.RetentionCompleted = project.RetentionCompleted
	}
	if project.TimeoutSec != 0 {
		merged.TimeoutSec = project.TimeoutSec
	}
	if project.AllowLongRunning {
		merged.AllowLongRunning = true
	}
	if project.DisableConcurrent {
		merged.DisableConcurrent = true
		merged.AbortPrevious = project.AbortPrevious
	}
	return merged
}

func clonePostSteps(source map[string][]Step) map[string][]Step {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string][]Step, len(source))
	for condition, steps := range source {
		cloned[condition] = append([]Step(nil), steps...)
	}
	return cloned
}

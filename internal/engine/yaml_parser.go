package engine

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type YAMLBuildConfig struct {
	Name        string                    `yaml:"name"`
	Description string                    `yaml:"description"`
	On          interface{}               `yaml:"on"`
	Env         map[string]string         `yaml:"env"`
	Jobs        map[string]YAMLJob        `yaml:"jobs"`
	Steps       []YAMLStep                `yaml:"steps"`
	RunsOn      string                    `yaml:"runs-on"`
}

type YAMLJob struct {
	Name     string            `yaml:"name"`
	Needs    []string          `yaml:"needs"`
	RunsOn   string            `yaml:"runs-on"`
	If       string            `yaml:"if"`
	Env      map[string]string `yaml:"env"`
	Steps    []YAMLStep        `yaml:"steps"`
	Timeout  string            `yaml:"timeout-minutes"`
	Parallel bool              `yaml:"parallel"`
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

func ParseYAMLConfig(yamlStr string) (*BuildConfig, error) {
	var yc YAMLBuildConfig
	if err := yaml.Unmarshal([]byte(yamlStr), &yc); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	cfg := &BuildConfig{
		Name:        yc.Name,
		Description: yc.Description,
		Environment: make(map[string]string),
		Triggers:    []Trigger{},
	}

	for k, v := range yc.Env {
		cfg.Environment[k] = v
	}

	cfg.Triggers = parseTriggers(yc.On)

	if len(yc.Jobs) > 0 {
		for jobKey, job := range yc.Jobs {
			stageName := jobKey
			if job.Name != "" {
				stageName = job.Name
			}
			mergedJobEnv := make(map[string]string)
			for k, v := range yc.Env {
				mergedJobEnv[k] = v
			}
			for k, v := range job.Env {
				mergedJobEnv[k] = v
			}
			stage := Stage{
				Name:     stageName,
				Parallel: job.Parallel,
				Steps:    convertSteps(mergedJobEnv, job.Steps),
			}
			cfg.Stages = append(cfg.Stages, stage)
		}
	} else if len(yc.Steps) > 0 {
		stage := Stage{
			Name:  "build",
			Steps: convertSteps(yc.Env, yc.Steps),
		}
		cfg.Stages = append(cfg.Stages, stage)
	}

	if len(cfg.Triggers) == 0 {
		cfg.Triggers = append(cfg.Triggers, Trigger{Type: "manual", Config: map[string]string{}})
	}

	return cfg, nil
}

func parseTriggers(on interface{}) []Trigger {
	var triggers []Trigger

	switch v := on.(type) {
	case string:
		triggers = append(triggers, triggerFromString(v))
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				triggers = append(triggers, triggerFromString(s))
			}
		}
	case map[string]interface{}:
		for key, val := range v {
			switch key {
			case "push", "webhook":
				triggers = append(triggers, Trigger{Type: "webhook", Config: map[string]string{}})
			case "schedule":
				if arr, ok := val.([]interface{}); ok {
					for _, item := range arr {
						if m, ok := item.(map[string]interface{}); ok {
							if cron, ok := m["cron"].(string); ok {
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

	hasManual := false
	for _, t := range triggers {
		if t.Type == "manual" {
			hasManual = true
			break
		}
	}
	if !hasManual {
		triggers = append(triggers, Trigger{Type: "manual", Config: map[string]string{}})
	}

	return triggers
}

func triggerFromString(s string) Trigger {
	switch s {
	case "push", "webhook":
		return Trigger{Type: "webhook", Config: map[string]string{}}
	case "schedule":
		return Trigger{Type: "schedule", Config: map[string]string{}}
	default:
		return Trigger{Type: "manual", Config: map[string]string{}}
	}
}

func convertSteps(parentEnv map[string]string, yamlSteps []YAMLStep) []Step {
	var steps []Step
	for _, ys := range yamlSteps {
		step := convertStep(ys)
		merged := make(map[string]string)
		for k, v := range parentEnv {
			merged[k] = v
		}
		for k, v := range ys.Env {
			merged[k] = v
		}
		if step.Config == nil {
			step.Config = make(map[string]string)
		}
		for k, v := range merged {
			step.Config["env."+k] = v
		}
		if ys.WorkingDir != "" {
			step.Config["working-directory"] = ys.WorkingDir
		}
		steps = append(steps, step)
	}
	return steps
}

func convertStep(ys YAMLStep) Step {
	step := Step{
		Name: ys.Name,
	}

	if ys.Uses != "" {
		if strings.HasPrefix(ys.Uses, "actions/checkout") {
			step.Type = "git"
			step.Command = "clone"
			step.Config = ys.With
		} else {
			step.Type = ys.Uses
			step.Config = ys.With
		}
		return step
	}

	if ys.Run != "" {
		step.Type = "shell"
		step.Command = ys.Run
		step.Shell = ys.Shell
		if step.Config == nil {
			step.Config = make(map[string]string)
		}
		return step
	}

	if ys.Type != "" {
		step.Type = ys.Type
		if ys.Command != "" {
			step.Command = ys.Command
		} else if ys.Run != "" {
			step.Command = ys.Run
		}
		step.Shell = ys.Shell
		if ys.With != nil {
			step.Config = ys.With
		} else {
			step.Config = make(map[string]string)
		}
		return step
	}

	if ys.Command != "" {
		step.Type = "shell"
		step.Command = ys.Command
		step.Shell = ys.Shell
		if step.Config == nil {
			step.Config = make(map[string]string)
		}
		return step
	}

	step.Type = "shell"
	return step
}

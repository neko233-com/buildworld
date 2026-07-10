package plugin

import (
	"context"

	"github.com/dop251/goja"
)

type PluginMeta struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author,omitempty"`
}

// StepContext is the execution context passed to a plugin-defined step.
type StepContext struct {
	Workspace string
	Branch    string
	Commit    string
	Config    map[string]string
	Env       map[string]string
	Outputs   map[string]string
	Logs      []string
	Failed    bool
	FailMsg   string
}

// StepHandler executes a plugin step. Returns error on failure.
type StepHandler func(ctx context.Context, sc *StepContext) error

// TriggerHandler evaluates whether a trigger should fire.
type TriggerHandler func(payload map[string]interface{}) bool

type Plugin struct {
	PluginMeta
	Path    string
	runtime *goja.Runtime

	// Registered step types (e.g. "github-notify", "docker-build").
	stepTypes map[string]StepHandler
	// Registered trigger types (e.g. "schedule", "webhook").
	triggerTypes map[string]TriggerHandler
}

// StepTypes returns the step type names this plugin registered.
func (p *Plugin) StepTypes() []string {
	var names []string
	for k := range p.stepTypes {
		names = append(names, k)
	}
	return names
}

// GetStep returns the handler for a step type, or nil if not found.
func (p *Plugin) GetStep(name string) StepHandler {
	return p.stepTypes[name]
}

// GetTrigger returns the handler for a trigger type, or nil if not found.
func (p *Plugin) GetTrigger(name string) TriggerHandler {
	return p.triggerTypes[name]
}

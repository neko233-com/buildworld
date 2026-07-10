package plugin

import (
	"context"

	"github.com/dop251/goja"
)

type UIExtensionPoint string

const (
	UIExtProjectTab     UIExtensionPoint = "project_tab"
	UIExtBuildDetail    UIExtensionPoint = "build_detail_panel"
	UIExtPipelineStep   UIExtensionPoint = "pipeline_step_config"
	UIExtDashboardWidget UIExtensionPoint = "dashboard_widget"
	UIExtGlobalMenu     UIExtensionPoint = "global_menu"
	UIExtSettingsTab    UIExtensionPoint = "settings_tab"
)

type UIExtension struct {
	Point     UIExtensionPoint `json:"point"`
	Name      string           `json:"name"`
	Label     string           `json:"label"`
	Component string           `json:"component"`
	Icon      string           `json:"icon,omitempty"`
}

type PluginMeta struct {
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Description  string         `json:"description"`
	Author       string         `json:"author,omitempty"`
	UIExtensions []UIExtension  `json:"ui_extensions,omitempty"`
}

type PluginStatus struct {
	Loaded   bool     `json:"loaded"`
	Error    string   `json:"error,omitempty"`
	Steps    []string `json:"steps"`
	Triggers []string `json:"triggers"`
}

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

type StepHandler func(ctx context.Context, sc *StepContext) error

type TriggerHandler func(payload map[string]interface{}) bool

type Plugin struct {
	PluginMeta
	Path    string
	runtime *goja.Runtime

	stepTypes    map[string]StepHandler
	triggerTypes map[string]TriggerHandler
	uiExtensions []UIExtension
	uiScript     string
	loadError    string
}

func (p *Plugin) StepTypes() []string {
	var names []string
	for k := range p.stepTypes {
		names = append(names, k)
	}
	return names
}

func (p *Plugin) TriggerTypes() []string {
	var names []string
	for k := range p.triggerTypes {
		names = append(names, k)
	}
	return names
}

func (p *Plugin) GetStep(name string) StepHandler {
	return p.stepTypes[name]
}

func (p *Plugin) GetTrigger(name string) TriggerHandler {
	return p.triggerTypes[name]
}

func (p *Plugin) UIExtensions() []UIExtension {
	return p.uiExtensions
}

func (p *Plugin) UIScript() string {
	return p.uiScript
}

func (p *Plugin) GetStatus() PluginStatus {
	status := PluginStatus{
		Loaded:   p.runtime != nil,
		Steps:    p.StepTypes(),
		Triggers: p.TriggerTypes(),
	}
	if p.loadError != "" {
		status.Error = p.loadError
	}
	return status
}

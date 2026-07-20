package plugin

import (
	"context"

	"github.com/dop251/goja"
)

type UIExtensionPoint string

const (
	UIExtProjectTab      UIExtensionPoint = "project_tab"
	UIExtBuildDetail     UIExtensionPoint = "build_detail_panel"
	UIExtPipelineStep    UIExtensionPoint = "pipeline_step_config"
	UIExtDashboardWidget UIExtensionPoint = "dashboard_widget"
	UIExtGlobalMenu      UIExtensionPoint = "global_menu"
	UIExtSettingsTab     UIExtensionPoint = "settings_tab"
)

type UIExtension struct {
	Point     UIExtensionPoint `json:"point"`
	Name      string           `json:"name"`
	Label     string           `json:"label"`
	Component string           `json:"component"`
	Icon      string           `json:"icon,omitempty"`
}

type PluginMeta struct {
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Description  string        `json:"description"`
	Author       string        `json:"author,omitempty"`
	UIExtensions []UIExtension `json:"ui_extensions,omitempty"`
}

// BinaryManifest is the portable contract for Go process plugins. It is data,
// never code: Buildworld validates it before launching the referenced binary.
type BinaryManifest struct {
	APIVersion  string `json:"api_version"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	// Source is the trusted GitHub repository from which Buildworld installed
	// this plugin. It lets a remote worker rebuild/download the same plugin for
	// its own platform without receiving arbitrary executable bytes from a build.
	Source         string          `json:"source,omitempty"`
	Entrypoint     string          `json:"entrypoint"`
	Package        string          `json:"package,omitempty"`
	ChecksumSHA256 string          `json:"checksum_sha256,omitempty"`
	Steps          []string        `json:"steps"`
	Releases       []BinaryRelease `json:"releases,omitempty"`
}

// BinaryReference is the small, versioned plugin contract carried with a
// remote build. The worker verifies the full manifest digest after resolving
// this server-approved GitHub source in its own plugin cache.
type BinaryReference struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	Source         string `json:"source"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

// BinaryRelease describes one immutable, platform-specific release asset.
// Prebuilt assets always require their own checksum before Buildworld executes
// them; source builds continue to record a checksum after compilation.
type BinaryRelease struct {
	GOOS           string `json:"goos"`
	GOARCH         string `json:"goarch"`
	URL            string `json:"url"`
	ChecksumSHA256 string `json:"checksum_sha256"`
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
	binary       *BinaryManifest
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

// InstallSource distinguishes immutable built-ins from portable GitHub binary
// plugins without exposing the private executable manifest to API callers.
func (p *Plugin) InstallSource() string {
	if p.binary != nil && p.binary.Source != "" {
		return p.binary.Source
	}
	return "builtin"
}

func (p *Plugin) GetStatus() PluginStatus {
	status := PluginStatus{
		Loaded:   p.runtime != nil || p.binary != nil,
		Steps:    p.StepTypes(),
		Triggers: p.TriggerTypes(),
	}
	if p.loadError != "" {
		status.Error = p.loadError
	}
	return status
}

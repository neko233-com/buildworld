package plugin

import (
	"context"
	"sort"
)

// PluginMeta is safe metadata returned to API clients. Executable source is
// never stored in this structure or accepted by the plugin API.
type PluginMeta struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author,omitempty"`
}

// BinaryManifest is the portable contract for an out-of-process Go plugin.
// BuildWorld validates it before launching the referenced executable.
type BinaryManifest struct {
	APIVersion  string `json:"api_version"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author,omitempty"`
	// Source is the trusted GitHub repository from which BuildWorld installed
	// this plugin. Remote workers resolve the same source independently.
	Source         string          `json:"source,omitempty"`
	Entrypoint     string          `json:"entrypoint"`
	Package        string          `json:"package,omitempty"`
	ChecksumSHA256 string          `json:"checksum_sha256,omitempty"`
	Steps          []string        `json:"steps,omitempty"`
	Hooks          []string        `json:"hooks,omitempty"`
	UI             []UIExtension   `json:"ui,omitempty"`
	Releases       []BinaryRelease `json:"releases,omitempty"`
}

// BinaryReference is the versioned contract carried with a remote build.
type BinaryReference struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	Source         string `json:"source"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

// BinaryRelease describes one immutable, platform-specific release asset.
type BinaryRelease struct {
	GOOS           string `json:"goos"`
	GOARCH         string `json:"goarch"`
	URL            string `json:"url"`
	ChecksumSHA256 string `json:"checksum_sha256"`
}

// UIExtension is a declarative, host-rendered action link. Plugins cannot
// inject HTML, JavaScript, styles, routes, or React components.
type UIExtension struct {
	Location     string `json:"location"`
	Label        string `json:"label"`
	URL          string `json:"url"`
	OpenInNewTab bool   `json:"open_in_new_tab,omitempty"`
}

type PluginStatus struct {
	Loaded       bool          `json:"loaded"`
	Steps        []string      `json:"steps"`
	Hooks        []string      `json:"hooks"`
	UIExtensions []UIExtension `json:"ui_extensions"`
}

// StepContext is serialized to an out-of-process plugin. It intentionally
// contains data only; no callback or host execution bridge is exposed.
type StepContext struct {
	Workspace   string
	ProjectID   int64
	ProjectName string
	BuildID     int64
	BuildNumber int
	Branch      string
	Commit      string
	Outcome     string
	Config      map[string]string
	Environment map[string]string
	Env         map[string]string
	Outputs     map[string]string
	Logs        []string
	Failed      bool
	FailMsg     string
}

type StepHandler func(ctx context.Context, sc *StepContext) error
type HookHandler func(ctx context.Context, sc *StepContext) error

type Plugin struct {
	PluginMeta
	Path      string
	stepTypes map[string]StepHandler
	hookTypes map[string]HookHandler
	binary    *BinaryManifest
}

func (p *Plugin) StepTypes() []string {
	names := make([]string, 0, len(p.stepTypes))
	for name := range p.stepTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p *Plugin) GetStep(name string) StepHandler {
	return p.stepTypes[name]
}

func (p *Plugin) HookTypes() []string {
	names := make([]string, 0, len(p.hookTypes))
	for name := range p.hookTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p *Plugin) GetHook(name string) HookHandler {
	return p.hookTypes[name]
}

func (p *Plugin) InstallSource() string {
	if p.binary == nil {
		return ""
	}
	return p.binary.Source
}

func (p *Plugin) GetStatus() PluginStatus {
	extensions := []UIExtension{}
	if p.binary != nil {
		extensions = append(extensions, p.binary.UI...)
	}
	return PluginStatus{Loaded: p.binary != nil, Steps: p.StepTypes(), Hooks: p.HookTypes(), UIExtensions: extensions}
}

package plugin

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
)

var pluginNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Loader owns installed out-of-process Go binary plugins. It never evaluates
// JavaScript or TypeScript and exposes no arbitrary host execution bridge.
type Loader struct {
	path           string
	plugins        map[string]*Plugin
	enabledPlugins map[string]bool
	mu             sync.RWMutex
}

func NewLoader(path string) *Loader {
	return &Loader{
		path:           path,
		plugins:        make(map[string]*Plugin),
		enabledPlugins: make(map[string]bool),
	}
}

func validatePluginName(name string) error {
	if !pluginNamePattern.MatchString(name) {
		return fmt.Errorf("invalid plugin name %q", name)
	}
	return nil
}

func (l *Loader) pluginPath(name string) (string, error) {
	if err := validatePluginName(name); err != nil {
		return "", err
	}
	root, err := filepath.Abs(l.path)
	if err != nil {
		return "", fmt.Errorf("resolve plugin root: %w", err)
	}
	return filepath.Join(root, name), nil
}

func (l *Loader) Load(name string) error {
	pluginPath, err := l.pluginPath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(pluginPath, "plugin-buildworld.json")); err != nil {
		return fmt.Errorf("read plugin-buildworld.json: %w", err)
	}
	return l.loadBinary(name, pluginPath)
}

func (l *Loader) Get(name string) *Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if !l.enabledPlugins[name] {
		return nil
	}
	return l.plugins[name]
}

func (l *Loader) getLoaded(name string) *Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.plugins[name]
}

// GetInstalled returns a loaded binary manifest regardless of activation
// state. Call Get when looking up an executable step for a build.
func (l *Loader) GetInstalled(name string) *Plugin {
	return l.getLoaded(name)
}

func (l *Loader) List() []*Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	list := make([]*Plugin, 0, len(l.plugins))
	for name, p := range l.plugins {
		if l.enabledPlugins[name] {
			list = append(list, p)
		}
	}
	return list
}

func (l *Loader) ListAll() []*Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	list := make([]*Plugin, 0, len(l.plugins))
	for _, p := range l.plugins {
		list = append(list, p)
	}
	return list
}

func (l *Loader) LookupStep(typeName string) StepHandler {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for name, p := range l.plugins {
		if !l.enabledPlugins[name] {
			continue
		}
		if handler := p.stepTypes[typeName]; handler != nil {
			return handler
		}
	}
	return nil
}

// RunHooks executes every enabled plugin registered for a lifecycle hook in
// deterministic plugin-name order. Multiple Jenkins-style publishers and
// notifiers may subscribe to the same build event.
func (l *Loader) RunHooks(ctx context.Context, hook string, sc *StepContext) error {
	if !isSupportedHook(hook) {
		return fmt.Errorf("unsupported plugin hook %q", hook)
	}
	type registeredHook struct {
		plugin  string
		handler HookHandler
	}
	l.mu.RLock()
	hooks := make([]registeredHook, 0)
	for name, p := range l.plugins {
		if l.enabledPlugins[name] {
			if handler := p.hookTypes[hook]; handler != nil {
				hooks = append(hooks, registeredHook{plugin: name, handler: handler})
			}
		}
	}
	l.mu.RUnlock()
	sort.Slice(hooks, func(i, j int) bool { return hooks[i].plugin < hooks[j].plugin })
	for _, registered := range hooks {
		if err := registered.handler(ctx, sc); err != nil {
			return fmt.Errorf("plugin %q hook %q: %w", registered.plugin, hook, err)
		}
	}
	return nil
}

func (l *Loader) LoadAll() error {
	entries, err := os.ReadDir(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(l.path, entry.Name(), "plugin-buildworld.json")
		if _, err := os.Stat(manifestPath); err != nil {
			continue
		}
		if err := l.Load(entry.Name()); err != nil {
			log.Printf("Warning: failed to load binary plugin %s: %v", entry.Name(), err)
		}
	}
	return nil
}

func (l *Loader) Unload(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.plugins, name)
	delete(l.enabledPlugins, name)
	return nil
}

func (l *Loader) Reload(name string) error {
	l.mu.RLock()
	enabled, known := l.enabledPlugins[name]
	l.mu.RUnlock()
	if !known {
		enabled = true
	}
	if err := l.Unload(name); err != nil {
		return err
	}
	if err := l.Load(name); err != nil {
		return err
	}
	l.SetEnabled(name, enabled)
	return nil
}

func (l *Loader) SetEnabled(name string, enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, loaded := l.plugins[name]; loaded {
		l.enabledPlugins[name] = enabled
	}
}

func (l *Loader) IsEnabled(name string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.enabledPlugins[name]
}

func (l *Loader) DeletePlugin(name string) error {
	pluginPath, err := l.pluginPath(name)
	if err != nil {
		return err
	}
	if err := l.Unload(name); err != nil {
		return err
	}
	return os.RemoveAll(pluginPath)
}

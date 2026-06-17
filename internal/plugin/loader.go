package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dop251/goja"
)

type Loader struct {
	path    string
	plugins map[string]*Plugin
	mu      sync.RWMutex
}

func NewLoader(path string) *Loader {
	return &Loader{
		path:    path,
		plugins: make(map[string]*Plugin),
	}
}

func (l *Loader) Load(name string) error {
	pluginPath := filepath.Join(l.path, name)

	metaPath := filepath.Join(pluginPath, "plugin.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("read plugin.json: %w", err)
	}

	meta := PluginMeta{}
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return fmt.Errorf("parse plugin.json: %w", err)
	}

	// Support both .ts and .js files
	scriptPath := filepath.Join(pluginPath, "index.ts")
	scriptData, err := os.ReadFile(scriptPath)
	if err != nil {
		// Fallback to .js
		scriptPath = filepath.Join(pluginPath, "index.js")
		scriptData, err = os.ReadFile(scriptPath)
		if err != nil {
			return fmt.Errorf("read index.ts/index.js: %w", err)
		}
	}

	vm := goja.New()
	_, err = vm.RunString(string(scriptData))
	if err != nil {
		return fmt.Errorf("execute plugin: %w", err)
	}

	plugin := &Plugin{
		PluginMeta: meta,
		Path:       pluginPath,
		runtime:    vm,
	}

	l.mu.Lock()
	l.plugins[name] = plugin
	l.mu.Unlock()
	return nil
}

func (l *Loader) Get(name string) *Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.plugins[name]
}

func (l *Loader) List() []*Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var list []*Plugin
	for _, p := range l.plugins {
		list = append(list, p)
	}
	return list
}

func (l *Loader) Unload(name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.plugins, name)
	return nil
}

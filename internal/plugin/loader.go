package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/yuin/gopher-lua"
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

	scriptPath := filepath.Join(pluginPath, "index.lua")
	scriptData, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("read index.lua: %w", err)
	}

	L := lua.NewState()
	defer L.Close()

	if err := L.DoString(string(scriptData)); err != nil {
		return fmt.Errorf("execute plugin: %w", err)
	}

	plugin := &Plugin{
		PluginMeta: meta,
		Path:       pluginPath,
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

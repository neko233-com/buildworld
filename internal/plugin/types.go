package plugin

import "github.com/dop251/goja"

type PluginMeta struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type Plugin struct {
	PluginMeta
	Path    string
	runtime *goja.Runtime
}

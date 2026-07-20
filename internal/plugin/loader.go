package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

func execCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", command)
	}
	return exec.Command("sh", "-c", command)
}

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

func (l *Loader) Load(name string) error {
	pluginPath := filepath.Join(l.path, name)
	if _, err := os.Stat(filepath.Join(pluginPath, "plugin-buildworld.json")); err == nil {
		return l.loadBinary(name, pluginPath)
	}

	metaPath := filepath.Join(pluginPath, "plugin.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("read plugin.json: %w", err)
	}

	meta := PluginMeta{}
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return fmt.Errorf("parse plugin.json: %w", err)
	}

	scriptData, err := readScript(pluginPath)
	if err != nil {
		return err
	}

	uiScriptData, _ := readUIScript(pluginPath)

	p := &Plugin{
		PluginMeta:   meta,
		Path:         pluginPath,
		runtime:      goja.New(),
		stepTypes:    make(map[string]StepHandler),
		triggerTypes: make(map[string]TriggerHandler),
		uiExtensions: meta.UIExtensions,
		uiScript:     string(uiScriptData),
	}

	l.exposeAPI(p)

	if _, err := p.runtime.RunString(string(scriptData)); err != nil {
		p.loadError = err.Error()
		p.runtime = nil
		return fmt.Errorf("execute plugin %s: %w", name, err)
	}

	l.mu.Lock()
	l.plugins[name] = p
	l.enabledPlugins[name] = true
	l.mu.Unlock()

	log.Printf("Plugin loaded: %s v%s (steps: %v)", meta.Name, meta.Version, p.StepTypes())
	return nil
}

func (l *Loader) exposeAPI(p *Plugin) {
	p.runtime.Set("registerStep", func(call goja.FunctionCall) goja.Value {
		typeName := call.Argument(0).String()
		fn, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			return p.runtime.ToValue(nil)
		}

		handler := func(ctx context.Context, sc *StepContext) error {
			jsCtx := p.runtime.NewObject()
			_ = jsCtx.Set("workspace", sc.Workspace)
			_ = jsCtx.Set("branch", sc.Branch)
			_ = jsCtx.Set("commit", sc.Commit)
			_ = jsCtx.Set("config", sc.Config)
			_ = jsCtx.Set("log", func(msg string) {
				sc.Logs = append(sc.Logs, msg)
			})
			_ = jsCtx.Set("env", func(key, val string) {
				if sc.Env == nil {
					sc.Env = make(map[string]string)
				}
				sc.Env[key] = val
			})
			_ = jsCtx.Set("output", func(key, val string) {
				if sc.Outputs == nil {
					sc.Outputs = make(map[string]string)
				}
				sc.Outputs[key] = val
			})
			_ = jsCtx.Set("fail", func(msg string) {
				sc.Failed = true
				sc.FailMsg = msg
			})

			_, err := fn(goja.Undefined(), jsCtx)
			if err != nil {
				return err
			}
			if sc.Failed {
				return fmt.Errorf("%s", sc.FailMsg)
			}
			return nil
		}

		p.stepTypes[typeName] = handler
		return p.runtime.ToValue(nil)
	})

	p.runtime.Set("registerTrigger", func(call goja.FunctionCall) goja.Value {
		typeName := call.Argument(0).String()
		fn, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			return p.runtime.ToValue(nil)
		}

		handler := func(payload map[string]interface{}) bool {
			result, err := fn(goja.Undefined(), p.runtime.ToValue(payload))
			if err != nil {
				return false
			}
			return result.ToBoolean()
		}

		p.triggerTypes[typeName] = handler
		return p.runtime.ToValue(nil)
	})

	p.runtime.Set("registerUI", func(call goja.FunctionCall) goja.Value {
		point := call.Argument(0).String()
		name := call.Argument(1).String()
		label := call.Argument(2).String()
		component := call.Argument(3).String()
		icon := ""
		if len(call.Arguments) > 4 {
			icon = call.Argument(4).String()
		}
		ext := UIExtension{
			Point:     UIExtensionPoint(point),
			Name:      name,
			Label:     label,
			Component: component,
			Icon:      icon,
		}
		p.uiExtensions = append(p.uiExtensions, ext)
		return p.runtime.ToValue(nil)
	})

	p.runtime.Set("http", func(call goja.FunctionCall) goja.Value {
		method := call.Argument(0).String()
		url := call.Argument(1).String()
		var opts map[string]interface{}
		if v := call.Argument(2); v != nil && v != goja.Undefined() && v != goja.Null() {
			_ = p.runtime.ExportTo(v, &opts)
		}

		var bodyReader io.Reader
		if body, ok := opts["body"].(string); ok && body != "" {
			bodyReader = bytes.NewReader([]byte(body))
		}

		req, err := http.NewRequest(strings.ToUpper(method), url, bodyReader)
		if err != nil {
			return p.runtime.ToValue(map[string]interface{}{"error": err.Error()})
		}

		if headers, ok := opts["headers"].(map[string]interface{}); ok {
			for k, v := range headers {
				req.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
		if req.Header.Get("Content-Type") == "" && bodyReader != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		timeout := 30 * time.Second
		if t, ok := opts["timeout"].(float64); ok && t > 0 {
			timeout = time.Duration(t) * time.Second
		}

		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			return p.runtime.ToValue(map[string]interface{}{"error": err.Error()})
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		return p.runtime.ToValue(map[string]interface{}{
			"status": resp.StatusCode,
			"body":   string(body),
		})
	})

	p.runtime.Set("exec", func(call goja.FunctionCall) goja.Value {
		command := call.Argument(0).String()
		cmd := execCommand(command)
		output, err := cmd.CombinedOutput()
		result := map[string]interface{}{
			"output": string(output),
		}
		if err != nil {
			result["error"] = err.Error()
		}
		return p.runtime.ToValue(result)
	})
}

func readScript(pluginPath string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(pluginPath, "index.js"))
	if err != nil {
		return nil, fmt.Errorf("read index.js in %s: %w", pluginPath, err)
	}
	return data, nil
}

func readUIScript(pluginPath string) ([]byte, error) {
	for _, name := range []string{"ui.js", filepath.Join("ui", "index.js")} {
		data, err := os.ReadFile(filepath.Join(pluginPath, name))
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("no ui script found")
}

func (l *Loader) GetSourceScript(name string) (script string, lang string, ok bool) {
	pluginPath := filepath.Join(l.path, name)
	for _, candidate := range []struct {
		file string
		lang string
	}{
		{"index.ts", "ts"},
		{"index.js", "js"},
	} {
		data, err := os.ReadFile(filepath.Join(pluginPath, candidate.file))
		if err == nil {
			return string(data), candidate.lang, true
		}
	}
	return "", "", false
}

func (l *Loader) GetSourceUIScript(name string) (script string, lang string, ok bool) {
	pluginPath := filepath.Join(l.path, name)
	for _, candidate := range []struct {
		file string
		lang string
	}{
		{"ui.tsx", "tsx"},
		{"ui.ts", "ts"},
		{"ui.js", "js"},
		{filepath.Join("ui", "index.js"), "js"},
	} {
		data, err := os.ReadFile(filepath.Join(pluginPath, candidate.file))
		if err == nil {
			return string(data), candidate.lang, true
		}
	}
	return "", "", false
}

func (l *Loader) WritePluginFiles(name, script, uiScript, scriptLang, sourceScript, sourceUIScript, uiLang string) error {
	if scriptLang == "" {
		scriptLang = "js"
	}
	pluginPath := filepath.Join(l.path, name)
	if err := os.MkdirAll(pluginPath, 0o755); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(pluginPath, "index.js"), []byte(script), 0o644); err != nil {
		return err
	}

	os.Remove(filepath.Join(pluginPath, "index.ts"))
	if scriptLang == "ts" && sourceScript != "" {
		if err := os.WriteFile(filepath.Join(pluginPath, "index.ts"), []byte(sourceScript), 0o644); err != nil {
			return err
		}
	}

	os.Remove(filepath.Join(pluginPath, "ui.js"))
	os.Remove(filepath.Join(pluginPath, "ui.ts"))
	os.Remove(filepath.Join(pluginPath, "ui.tsx"))
	os.RemoveAll(filepath.Join(pluginPath, "ui"))

	if uiScript != "" {
		if err := os.WriteFile(filepath.Join(pluginPath, "ui.js"), []byte(uiScript), 0o644); err != nil {
			return err
		}
		if uiLang == "tsx" && sourceUIScript != "" {
			if err := os.WriteFile(filepath.Join(pluginPath, "ui.tsx"), []byte(sourceUIScript), 0o644); err != nil {
				return err
			}
		} else if uiLang == "ts" && sourceUIScript != "" {
			if err := os.WriteFile(filepath.Join(pluginPath, "ui.ts"), []byte(sourceUIScript), 0o644); err != nil {
				return err
			}
		}
	}

	return nil
}

func (l *Loader) Get(name string) *Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if !l.enabledPlugins[name] {
		return nil
	}
	return l.plugins[name]
}

func (l *Loader) List() []*Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var list []*Plugin
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
	var list []*Plugin
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
		if h := p.stepTypes[typeName]; h != nil {
			return h
		}
	}
	return nil
}

func (l *Loader) LookupTrigger(typeName string) TriggerHandler {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for name, p := range l.plugins {
		if !l.enabledPlugins[name] {
			continue
		}
		if h := p.triggerTypes[typeName]; h != nil {
			return h
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
	for _, e := range entries {
		if e.IsDir() {
			if err := l.Load(e.Name()); err != nil {
				log.Printf("Warning: failed to load plugin %s: %v", e.Name(), err)
			}
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
	l.Unload(name)
	return l.Load(name)
}

func (l *Loader) SetEnabled(name string, enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enabledPlugins[name] = enabled
}

func (l *Loader) IsEnabled(name string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.enabledPlugins[name]
}

func (l *Loader) InstallPlugin(name, version, description, author, script, uiScript, scriptLang, sourceScript, sourceUIScript, uiLang string) error {
	if scriptLang == "" {
		scriptLang = "js"
	}
	pluginPath := filepath.Join(l.path, name)
	if err := os.MkdirAll(pluginPath, 0o755); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	meta := PluginMeta{
		Name:        name,
		Version:     version,
		Description: description,
		Author:      author,
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pluginPath, "plugin.json"), metaData, 0o644); err != nil {
		return err
	}

	return l.WritePluginFiles(name, script, uiScript, scriptLang, sourceScript, sourceUIScript, uiLang)
}

func (l *Loader) DeletePlugin(name string) error {
	pluginPath := filepath.Join(l.path, name)
	l.Unload(name)
	return os.RemoveAll(pluginPath)
}

func (l *Loader) GetPluginUI(name string) (string, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	p, ok := l.plugins[name]
	if !ok {
		return "", false
	}
	return p.uiScript, p.uiScript != ""
}

func (l *Loader) GetAllUIExtensions() map[string][]UIExtension {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make(map[string][]UIExtension)
	for name, p := range l.plugins {
		if !l.enabledPlugins[name] {
			continue
		}
		result[name] = p.uiExtensions
	}
	return result
}

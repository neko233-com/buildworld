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

// execCommand creates a shell command for cross-platform execution.
func execCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", command)
	}
	return exec.Command("sh", "-c", command)
}

// Loader loads plugins from disk and maintains a registry of step/trigger handlers.
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

// Load reads and executes a plugin from disk, registering its steps/triggers.
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

	// Support both .ts and .js entry files.
	scriptData, err := readScript(pluginPath)
	if err != nil {
		return err
	}

	p := &Plugin{
		PluginMeta:   meta,
		Path:         pluginPath,
		runtime:      goja.New(),
		stepTypes:    make(map[string]StepHandler),
		triggerTypes: make(map[string]TriggerHandler),
	}

	// Expose registerStep / registerTrigger to the JS runtime.
	l.exposeAPI(p)

	if _, err := p.runtime.RunString(string(scriptData)); err != nil {
		return fmt.Errorf("execute plugin %s: %w", name, err)
	}

	l.mu.Lock()
	l.plugins[name] = p
	l.mu.Unlock()

	log.Printf("Plugin loaded: %s v%s (steps: %v)", meta.Name, meta.Version, p.StepTypes())
	return nil
}

// exposeAPI injects registerStep/registerTrigger into the goja runtime.
func (l *Loader) exposeAPI(p *Plugin) {
	// registerStep(typeName, gojaFunction)
	p.runtime.Set("registerStep", func(call goja.FunctionCall) goja.Value {
		typeName := call.Argument(0).String()
		fn, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			return p.runtime.ToValue(nil)
		}

		handler := func(ctx context.Context, sc *StepContext) error {
			// Build a JS context object to pass to the plugin function.
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

	// registerTrigger(typeName, gojaFunction)
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

	// http(method, url, options) → {status, body}
	// options: {headers: {}, body: "", timeout: 30}
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

	// exec(command) → {output, error}  — runs a shell command and captures output.
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
	for _, name := range []string{"index.js", "index.ts"} {
		data, err := os.ReadFile(filepath.Join(pluginPath, name))
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("read index.js/index.ts in %s", pluginPath)
}

// Get returns a loaded plugin by name.
func (l *Loader) Get(name string) *Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.plugins[name]
}

// List returns all loaded plugins.
func (l *Loader) List() []*Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var list []*Plugin
	for _, p := range l.plugins {
		list = append(list, p)
	}
	return list
}

// LookupStep finds a step handler across all loaded plugins.
func (l *Loader) LookupStep(typeName string) StepHandler {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, p := range l.plugins {
		if h := p.stepTypes[typeName]; h != nil {
			return h
		}
	}
	return nil
}

// LookupTrigger finds a trigger handler across all loaded plugins.
func (l *Loader) LookupTrigger(typeName string) TriggerHandler {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, p := range l.plugins {
		if h := p.triggerTypes[typeName]; h != nil {
			return h
		}
	}
	return nil
}

// LoadAll loads all plugins found in the loader's path directory.
func (l *Loader) LoadAll() error {
	entries, err := os.ReadDir(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no plugins dir — fine
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
	return nil
}

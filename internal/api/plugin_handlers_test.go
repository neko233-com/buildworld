package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/plugin"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestBinaryPluginLifecycleAPIHasNoScriptRuntimeEndpoints(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "alpha")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := "alpha"
	entryFile := entry
	if runtime.GOOS == "windows" {
		entryFile += ".exe"
	}
	if err := os.WriteFile(filepath.Join(pluginDir, entryFile), []byte("placeholder"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := plugin.BinaryManifest{
		APIVersion: plugin.BinaryAPIVersion,
		Name:       "alpha",
		Version:    "1.0.0",
		Source:     "https://github.com/acme/alpha",
		Entrypoint: entry,
		Steps:      []string{"alpha:deploy"},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin-buildworld.json"), manifestJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	loader := plugin.NewLoader(root)
	if err := loader.Load("alpha"); err != nil {
		t.Fatal(err)
	}
	data, err := store.New(filepath.Join(t.TempDir(), "plugins.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	stored, err := data.CreatePlugin("alpha", "1.0.0", "test", "acme", "{}", pluginDir, manifest.Source)
	if err != nil {
		t.Fatal(err)
	}
	user, err := data.CreateUser("admin", "admin@example.test", "unused", "admin")
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWT("binary-plugin-api-test-secret")
	token, err := jwt.Generate(user.ID, "admin", user.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: data, Loader: loader, JWT: jwt})
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	listed := request(http.MethodGet, "/api/plugins/", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", listed.Code, listed.Body.String())
	}
	var plugins []map[string]any
	if err := json.Unmarshal(listed.Body.Bytes(), &plugins); err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Fatalf("listed plugins = %#v", plugins)
	}
	steps, ok := plugins[0]["steps"].([]any)
	if !ok || len(steps) != 1 || steps[0] != "alpha:deploy" {
		t.Fatalf("listed steps = %#v", plugins[0]["steps"])
	}
	for _, legacyField := range []string{"script_lang", "source_script", "source_ui_script", "triggers", "ui_extensions"} {
		if _, exists := plugins[0][legacyField]; exists {
			t.Fatalf("plugin response exposes legacy field %q", legacyField)
		}
	}

	disabled := request(http.MethodPut, "/api/plugins/"+jsonNumber(stored.ID)+"/enable", `{"enabled":false}`)
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status = %d: %s", disabled.Code, disabled.Body.String())
	}
	if loader.Get("alpha") != nil {
		t.Fatal("disabled plugin remains executable")
	}
	reloaded := request(http.MethodPost, "/api/plugins/alpha/reload", "")
	if reloaded.Code != http.StatusOK {
		t.Fatalf("reload status = %d: %s", reloaded.Code, reloaded.Body.String())
	}
	if loader.Get("alpha") != nil {
		t.Fatal("reload changed disabled state")
	}

	for _, legacyPath := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/plugins/", `{"name":"script","script":"registerStep('x',()=>{})"}`},
		{http.MethodGet, "/api/plugins/alpha/source", ""},
		{http.MethodPut, "/api/plugins/alpha/source", `{}`},
		{http.MethodGet, "/api/plugins/alpha/ui.js", ""},
		{http.MethodGet, "/api/plugins/ui-extensions", ""},
	} {
		if response := request(legacyPath.method, legacyPath.path, legacyPath.body); response.Code == http.StatusOK || response.Code == http.StatusCreated {
			t.Fatalf("legacy endpoint %s %s remains active", legacyPath.method, legacyPath.path)
		}
	}

	removed := request(http.MethodDelete, "/api/plugins/"+jsonNumber(stored.ID), "")
	if removed.Code != http.StatusOK {
		t.Fatalf("delete status = %d: %s", removed.Code, removed.Body.String())
	}
	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Fatalf("plugin directory remains after delete: %v", err)
	}
	if _, err := data.GetPlugin(stored.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("plugin database row remains: %v", err)
	}
}

func jsonNumber(value int64) string {
	return fmt.Sprint(value)
}

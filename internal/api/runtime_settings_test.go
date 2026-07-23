package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestAgentEnrollmentTokenIsPersistedAndMasked(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := &config.Config{Server: config.ServerConfig{Port: 6050}}
	handler := &handlers{d: Deps{Cfg: cfg, Store: database}}

	generated := httptest.NewRecorder()
	handler.generateAgentToken(generated, httptest.NewRequest("POST", "/api/agents/generate-token", nil))
	if generated.Code != 200 {
		t.Fatalf("generate token status = %d, body = %s", generated.Code, generated.Body.String())
	}
	var tokenResponse map[string]string
	if err := json.NewDecoder(generated.Body).Decode(&tokenResponse); err != nil {
		t.Fatal(err)
	}
	token := tokenResponse["token"]
	if !strings.HasPrefix(token, "bw_enroll_") || len(token) < 70 {
		t.Fatalf("generated token has unexpected format: %q", token)
	}
	if cfg.Workers.EnrollmentToken != token {
		t.Fatal("generated token was not applied to the live configuration")
	}

	settings := httptest.NewRecorder()
	handler.getGlobalSettings(settings, httptest.NewRequest("GET", "/api/settings", nil))
	var settingsResponse map[string]string
	if err := json.NewDecoder(settings.Body).Decode(&settingsResponse); err != nil {
		t.Fatal(err)
	}
	if settingsResponse[agentEnrollmentTokenSetting] != "" {
		t.Fatal("settings response exposed the enrollment secret")
	}
	if settingsResponse[agentEnrollmentTokenSetting+"_configured"] != "true" {
		t.Fatalf("configured status = %q, want true", settingsResponse[agentEnrollmentTokenSetting+"_configured"])
	}

	restarted := &config.Config{Server: config.ServerConfig{Port: 6050}}
	if err := ApplyStoredSettings(restarted, database); err != nil {
		t.Fatal(err)
	}
	if restarted.Workers.EnrollmentToken != token {
		t.Fatal("stored enrollment token was not restored")
	}
}

func TestUpdateGlobalSettingsRejectsUnknownKeys(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	handler := &handlers{d: Deps{Cfg: &config.Config{}, Store: database}}
	request := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"unknown_setting":"value"}`))
	response := httptest.NewRecorder()
	handler.updateGlobalSettings(response, request)
	if response.Code != 400 {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}

func TestControlPlanePortCannotBeOverriddenByStoredSettings(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SetEnvVar("system", nil, "port", "6050", false, ""); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Server: config.ServerConfig{Port: 6050}}
	if err := ApplyStoredSettings(cfg, database); err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != config.ControlPlanePort {
		t.Fatalf("runtime port = %d, want %d", cfg.Server.Port, config.ControlPlanePort)
	}

	handler := &handlers{d: Deps{Cfg: cfg, Store: database}}
	settings := httptest.NewRecorder()
	handler.getGlobalSettings(settings, httptest.NewRequest("GET", "/api/settings", nil))
	var values map[string]string
	if err := json.NewDecoder(settings.Body).Decode(&values); err != nil {
		t.Fatal(err)
	}
	if values["port"] != "8080" {
		t.Fatalf("settings port = %q, want 8080", values["port"])
	}

	response := httptest.NewRecorder()
	handler.updateGlobalSettings(response, httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"port":"6050"}`)))
	if response.Code != 400 || !strings.Contains(response.Body.String(), "fixed at 8080") {
		t.Fatalf("override status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestResourceSettingsDefaultLowAndHotReloadWithoutRestart(t *testing.T) {
	defaults := defaultGlobalSettings(nil)
	if defaults["cpu_limit_percent"] != "25" || defaults["background_mode"] != "true" ||
		defaults["local_agent_concurrency"] != "1" || defaults["build_concurrency"] != "2" ||
		defaults["port"] != "8080" {
		t.Fatalf("low-resource defaults = %#v", defaults)
	}
	for _, removed := range []string{"tls", "logs_path"} {
		if _, exists := defaults[removed]; exists {
			t.Fatalf("unsupported runtime setting %q is still advertised", removed)
		}
	}

	database, err := store.New(filepath.Join(t.TempDir(), "resource-settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	originalProcs := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(originalProcs)
	runner := engine.NewBuildRunner(database, nil, filepath.Join(t.TempDir(), "workspaces"), nil)
	handler := &handlers{d: Deps{Cfg: &config.Config{}, Store: database, Runner: runner}}
	request := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{
		"cpu_limit_percent":"10",
		"background_mode":"false",
		"local_agent_concurrency":"1",
		"build_concurrency":"1"
	}`))
	response := httptest.NewRecorder()

	handler.updateGlobalSettings(response, request)

	if response.Code != 200 {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	policy := runner.ExecutionPolicySnapshot()
	if policy.CPUPercent != 10 || policy.BackgroundMode || policy.MaxConcurrentBuilds != 1 || policy.MaxConcurrentLocalBuilds != 1 {
		t.Fatalf("hot-reloaded policy = %#v", policy)
	}
	expectedProcs := runtime.NumCPU() / 10
	if expectedProcs < 1 {
		expectedProcs = 1
	}
	if runtime.GOMAXPROCS(0) != expectedProcs {
		t.Fatalf("GOMAXPROCS = %d, want %d", runtime.GOMAXPROCS(0), expectedProcs)
	}
}

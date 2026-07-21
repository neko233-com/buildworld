package config

import (
	"os"
	"reflect"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	yaml := `
server:
  host: "0.0.0.0"
  port: 6050
database:
  path: "./data/test.db"
`
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(yaml)
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 6050 {
		t.Errorf("Port = %d, want 6050", cfg.Server.Port)
	}
}

func TestConfigValidation(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() should fail on empty config")
	}
}

func TestAutomationConfigOnlyContainsWebhookSignatureSecret(t *testing.T) {
	typeInfo := reflect.TypeOf(AutomationConfig{})
	if typeInfo.NumField() != 1 {
		t.Fatalf("AutomationConfig fields = %d, want only webhook signature secret", typeInfo.NumField())
	}
	field := typeInfo.Field(0)
	if field.Name != "GitHubWebhookSecret" || field.Tag.Get("yaml") != "github_webhook_secret" {
		t.Fatalf("AutomationConfig field = %s (%q), want GitHubWebhookSecret", field.Name, field.Tag.Get("yaml"))
	}
}

func TestPluginsConfigOnlyContainsPath(t *testing.T) {
	typeInfo := reflect.TypeOf(PluginsConfig{})
	if typeInfo.NumField() != 1 {
		t.Fatalf("PluginsConfig fields = %d, want only path", typeInfo.NumField())
	}
	field := typeInfo.Field(0)
	if field.Name != "Path" || field.Tag.Get("yaml") != "path" {
		t.Fatalf("PluginsConfig field = %s (%q), want Path", field.Name, field.Tag.Get("yaml"))
	}
}

func TestLoadConfigAllFields(t *testing.T) {
	yaml := `
server:
  host: "127.0.0.1"
  port: 8080
database:
  path: "./data/test.db"
auth:
  jwt_secret: "secret123"
plugins:
  path: "./plugins"
storage:
  build_temp: "./build-temp"
  artifacts: "./artifacts"
workers:
  local:
    max_concurrent_builds: 8
    workspace: "./local-workspace"
    pool: "release"
    labels: ["linux", "docker"]
`
	tmpFile, err := os.CreateTemp("", "config-all-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(yaml)
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "127.0.0.1")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Database.Path != "./data/test.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "./data/test.db")
	}
	if cfg.Auth.JWTSecret != "secret123" {
		t.Errorf("Auth.JWTSecret = %q, want %q", cfg.Auth.JWTSecret, "secret123")
	}
	if cfg.Plugins.Path != "./plugins" {
		t.Errorf("Plugins.Path = %q, want %q", cfg.Plugins.Path, "./plugins")
	}
	if cfg.Storage.BuildTemp != "./build-temp" {
		t.Errorf("Storage.BuildTemp = %q, want %q", cfg.Storage.BuildTemp, "./build-temp")
	}
	if cfg.Storage.Artifacts != "./artifacts" {
		t.Errorf("Storage.Artifacts = %q, want %q", cfg.Storage.Artifacts, "./artifacts")
	}
	if cfg.Workers.Local.MaxConcurrentBuilds != 8 {
		t.Errorf("Workers.Local.MaxConcurrentBuilds = %d, want 8", cfg.Workers.Local.MaxConcurrentBuilds)
	}
	if cfg.Workers.Local.Workspace != "./local-workspace" {
		t.Errorf("Workers.Local.Workspace = %q, want %q", cfg.Workers.Local.Workspace, "./local-workspace")
	}
	if len(cfg.Workers.Local.Labels) != 2 {
		t.Errorf("len(Workers.Local.Labels) = %d, want 2", len(cfg.Workers.Local.Labels))
	}
	if cfg.Workers.Local.Pool != "release" {
		t.Errorf("Workers.Local.Pool = %q, want release", cfg.Workers.Local.Pool)
	}
}

func TestLoadConfigMinimalFields(t *testing.T) {
	yaml := `
server:
  port: 3000
database:
  path: "./data/min.db"
`
	tmpFile, err := os.CreateTemp("", "config-min-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(yaml)
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
	if cfg.Database.Path != "./data/min.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "./data/min.db")
	}
	if cfg.Server.Host != "" {
		t.Errorf("Server.Host = %q, want empty", cfg.Server.Host)
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	invalidYAMLs := []string{
		"{invalid yaml content",
		"server:\n  port: [not, a, number]",
	}

	for i, content := range invalidYAMLs {
		tmpFile, err := os.CreateTemp("", "config-invalid-*.yaml")
		if err != nil {
			t.Fatal(err)
		}
		tmpFile.WriteString(content)
		tmpFile.Close()

		_, err = Load(tmpFile.Name())
		os.Remove(tmpFile.Name())

		if err == nil {
			t.Errorf("case %d: Load() should fail on invalid YAML", i)
		}
	}
}

func TestLoadConfigMissingRequiredFields(t *testing.T) {
	testCases := []struct {
		name string
		yaml string
	}{
		{
			name: "missing server.port",
			yaml: `
database:
  path: "./data/test.db"
`,
		},
		{
			name: "missing database.path",
			yaml: `
server:
  port: 8080
`,
		},
		{
			name: "both missing",
			yaml: `
server:
  host: "0.0.0.0"
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "config-missing-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpFile.Name())
			tmpFile.WriteString(tc.yaml)
			tmpFile.Close()

			_, err = Load(tmpFile.Name())
			if err == nil {
				t.Error("Load() should fail with missing required fields")
			}
		})
	}
}

func TestLoadConfigExtraFieldsIgnored(t *testing.T) {
	yaml := `
server:
  port: 8080
database:
  path: "./data/test.db"
unknown_section:
  some_key: "some_value"
another_unknown:
  nested:
    key: "value"
`
	tmpFile, err := os.CreateTemp("", "config-extra-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString(yaml)
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Database.Path != "./data/test.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "./data/test.db")
	}
}

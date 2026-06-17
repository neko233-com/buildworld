package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	yaml := `
server:
  host: "0.0.0.0"
  port: 6050
  tls: false
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

func TestLoadConfigAllFields(t *testing.T) {
	yaml := `
server:
  host: "127.0.0.1"
  port: 8080
  tls: true
database:
  path: "./data/test.db"
auth:
  jwt_secret: "secret123"
  oauth:
    github:
      client_id: "id123"
      client_secret: "secret456"
plugins:
  path: "./plugins"
  hot_reload: true
storage:
  workspace: "./workspace"
  artifacts: "./artifacts"
  logs: "./logs"
git:
  ssh_key_path: "/path/to/ssh"
  known_hosts: "/path/to/hosts"
workers:
  local:
    enabled: true
    max_concurrent_builds: 8
    workspace: "./local-workspace"
    labels: ["linux", "docker"]
  remote:
    - name: "worker-1"
      address: "10.0.0.1:9090"
      token: "tok123"
      labels: ["gpu"]
      max_concurrent_builds: 4
    - name: "worker-2"
      address: "10.0.0.2:9090"
      token: "tok456"
      labels: ["cpu"]
      max_concurrent_builds: 2
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
	if !cfg.Server.TLS {
		t.Error("Server.TLS = false, want true")
	}
	if cfg.Database.Path != "./data/test.db" {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "./data/test.db")
	}
	if cfg.Auth.JWTSecret != "secret123" {
		t.Errorf("Auth.JWTSecret = %q, want %q", cfg.Auth.JWTSecret, "secret123")
	}
	if cfg.Auth.OAuth.GitHub.ClientID != "id123" {
		t.Errorf("Auth.OAuth.GitHub.ClientID = %q, want %q", cfg.Auth.OAuth.GitHub.ClientID, "id123")
	}
	if cfg.Auth.OAuth.GitHub.ClientSecret != "secret456" {
		t.Errorf("Auth.OAuth.GitHub.ClientSecret = %q, want %q", cfg.Auth.OAuth.GitHub.ClientSecret, "secret456")
	}
	if cfg.Plugins.Path != "./plugins" {
		t.Errorf("Plugins.Path = %q, want %q", cfg.Plugins.Path, "./plugins")
	}
	if !cfg.Plugins.HotReload {
		t.Error("Plugins.HotReload = false, want true")
	}
	if cfg.Storage.Workspace != "./workspace" {
		t.Errorf("Storage.Workspace = %q, want %q", cfg.Storage.Workspace, "./workspace")
	}
	if cfg.Storage.Artifacts != "./artifacts" {
		t.Errorf("Storage.Artifacts = %q, want %q", cfg.Storage.Artifacts, "./artifacts")
	}
	if cfg.Storage.Logs != "./logs" {
		t.Errorf("Storage.Logs = %q, want %q", cfg.Storage.Logs, "./logs")
	}
	if cfg.Git.SSHKeyPath != "/path/to/ssh" {
		t.Errorf("Git.SSHKeyPath = %q, want %q", cfg.Git.SSHKeyPath, "/path/to/ssh")
	}
	if cfg.Git.KnownHosts != "/path/to/hosts" {
		t.Errorf("Git.KnownHosts = %q, want %q", cfg.Git.KnownHosts, "/path/to/hosts")
	}
	if !cfg.Workers.Local.Enabled {
		t.Error("Workers.Local.Enabled = false, want true")
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
	if len(cfg.Workers.Remote) != 2 {
		t.Fatalf("len(Workers.Remote) = %d, want 2", len(cfg.Workers.Remote))
	}
	if cfg.Workers.Remote[0].Name != "worker-1" {
		t.Errorf("Workers.Remote[0].Name = %q, want %q", cfg.Workers.Remote[0].Name, "worker-1")
	}
	if cfg.Workers.Remote[0].Address != "10.0.0.1:9090" {
		t.Errorf("Workers.Remote[0].Address = %q, want %q", cfg.Workers.Remote[0].Address, "10.0.0.1:9090")
	}
	if cfg.Workers.Remote[0].Token != "tok123" {
		t.Errorf("Workers.Remote[0].Token = %q, want %q", cfg.Workers.Remote[0].Token, "tok123")
	}
	if cfg.Workers.Remote[1].Name != "worker-2" {
		t.Errorf("Workers.Remote[1].Name = %q, want %q", cfg.Workers.Remote[1].Name, "worker-2")
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
	if cfg.Server.TLS {
		t.Error("Server.TLS = true, want false")
	}
	if len(cfg.Workers.Remote) != 0 {
		t.Errorf("len(Workers.Remote) = %d, want 0", len(cfg.Workers.Remote))
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

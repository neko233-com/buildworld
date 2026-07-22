package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ControlPlanePort is the fixed local HTTP port used by BuildWorld.
const ControlPlanePort = 8700

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	Auth       AuthConfig       `yaml:"auth"`
	Plugins    PluginsConfig    `yaml:"plugins"`
	Storage    StorageConfig    `yaml:"storage"`
	Workers    WorkersConfig    `yaml:"workers"`
	Automation AutomationConfig `yaml:"automation"`
}

type AutomationConfig struct {
	GitHubWebhookSecret string `yaml:"github_webhook_secret"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type AuthConfig struct {
	JWTSecret string `yaml:"jwt_secret"`
}

type PluginsConfig struct {
	Path string `yaml:"path"`
}

type StorageConfig struct {
	BuildTemp string `yaml:"build_temp"`
	Artifacts string `yaml:"artifacts"`
}

type WorkersConfig struct {
	Local           LocalWorkerConfig `yaml:"local"`
	EnrollmentToken string            `yaml:"enrollment_token"`
}

type LocalWorkerConfig struct {
	MaxConcurrentBuilds int      `yaml:"max_concurrent_builds"`
	Workspace           string   `yaml:"workspace"`
	Pool                string   `yaml:"pool"`
	Labels              []string `yaml:"labels"`
}

// EnforceControlPlanePort keeps installed clients, workers, and server
// lifecycle commands on one stable endpoint regardless of persisted settings.
func (c *Config) EnforceControlPlanePort() {
	if c != nil {
		c.Server.Port = ControlPlanePort
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port == 0 {
		return fmt.Errorf("server.port is required")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}
	return nil
}

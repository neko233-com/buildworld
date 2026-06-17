package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Plugins  PluginsConfig  `yaml:"plugins"`
	Storage  StorageConfig  `yaml:"storage"`
	Git      GitConfig      `yaml:"git"`
	Workers  WorkersConfig  `yaml:"workers"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	TLS  bool   `yaml:"tls"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type AuthConfig struct {
	JWTSecret string      `yaml:"jwt_secret"`
	OAuth     OAuthConfig `yaml:"oauth"`
}

type OAuthConfig struct {
	GitHub GitHubOAuth `yaml:"github"`
}

type GitHubOAuth struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type PluginsConfig struct {
	Path      string `yaml:"path"`
	HotReload bool   `yaml:"hot_reload"`
}

type StorageConfig struct {
	Workspace string `yaml:"workspace"`
	Artifacts string `yaml:"artifacts"`
	Logs      string `yaml:"logs"`
}

type GitConfig struct {
	SSHKeyPath string `yaml:"ssh_key_path"`
	KnownHosts string `yaml:"known_hosts"`
}

type WorkersConfig struct {
	Local  LocalWorkerConfig    `yaml:"local"`
	Remote []RemoteWorkerConfig `yaml:"remote"`
}

type LocalWorkerConfig struct {
	Enabled             bool     `yaml:"enabled"`
	MaxConcurrentBuilds int      `yaml:"max_concurrent_builds"`
	Workspace           string   `yaml:"workspace"`
	Labels              []string `yaml:"labels"`
}

type RemoteWorkerConfig struct {
	Name               string   `yaml:"name"`
	Address            string   `yaml:"address"`
	Token              string   `yaml:"token"`
	Labels             []string `yaml:"labels"`
	MaxConcurrentBuilds int      `yaml:"max_concurrent_builds"`
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

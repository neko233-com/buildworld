package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
)

const agentEnrollmentTokenSetting = "agent_enrollment_token"

const (
	defaultHTTPPort       = 8700
	legacyDefaultHTTPPort = 7777
)

// ApplyStoredSettings restores settings that must be known before the build
// runner is created. Config.yaml remains the bootstrap fallback.
func ApplyStoredSettings(cfg *config.Config, data *store.Store) error {
	if cfg == nil || data == nil {
		return nil
	}
	values, err := data.ListEnvVars("system", nil)
	if err != nil {
		return fmt.Errorf("load stored runtime settings: %w", err)
	}
	for _, value := range values {
		switch value.Name {
		case "host":
			cfg.Server.Host = value.Value
		case "port":
			if parsed, parseErr := strconv.Atoi(value.Value); parseErr == nil && parsed > 0 && parsed <= 65535 &&
				!(parsed == legacyDefaultHTTPPort && cfg.Server.Port == defaultHTTPPort) {
				cfg.Server.Port = parsed
			}
		case "tls":
			if parsed, parseErr := strconv.ParseBool(value.Value); parseErr == nil {
				cfg.Server.TLS = parsed
			}
		case "artifacts_path":
			cfg.Storage.Artifacts = value.Value
		case "logs_path":
			cfg.Storage.Logs = value.Value
		case "build_temp_path":
			cfg.Storage.BuildTemp = value.Value
		case "local_agent_concurrency":
			if parsed, parseErr := strconv.Atoi(value.Value); parseErr == nil && parsed > 0 {
				cfg.Workers.Local.MaxConcurrentBuilds = parsed
			}
		case agentEnrollmentTokenSetting:
			cfg.Workers.EnrollmentToken = value.Value
		}
	}
	return nil
}

func defaultGlobalSettings(cfg *config.Config) map[string]string {
	settings := map[string]string{
		"host":                    "0.0.0.0",
		"port":                    strconv.Itoa(defaultHTTPPort),
		"tls":                     "false",
		"build_timeout":           "1800",
		"build_concurrency":       "2",
		"local_agent_concurrency": "1",
		"cpu_limit_percent":       "25",
		"background_mode":         "true",
		"retry_policy":            "failed_once",
		"artifacts_path":          "./artifacts",
		"logs_path":               "./logs",
		"build_temp_path":         "./build_temp",
		"go_validation_enabled":   "true",
		"go_version":              "1.26",
		"go_checks":               "fmt,vet,test,build",
		"node_validation_enabled": "true",
		"node_version":            "24",
		"node_package_manager":    "npm",
		"node_checks":             "install,lint,typecheck,test,build",
		"validation_fail_fast":    "true",
	}
	if cfg == nil {
		return settings
	}
	if cfg.Server.Host != "" {
		settings["host"] = cfg.Server.Host
	}
	if cfg.Server.Port > 0 {
		settings["port"] = strconv.Itoa(cfg.Server.Port)
	}
	settings["tls"] = strconv.FormatBool(cfg.Server.TLS)
	if cfg.Storage.Artifacts != "" {
		settings["artifacts_path"] = cfg.Storage.Artifacts
	}
	if cfg.Storage.Logs != "" {
		settings["logs_path"] = cfg.Storage.Logs
	}
	if cfg.Storage.BuildTemp != "" {
		settings["build_temp_path"] = cfg.Storage.BuildTemp
	}
	if cfg.Workers.Local.MaxConcurrentBuilds > 0 {
		settings["local_agent_concurrency"] = strconv.Itoa(cfg.Workers.Local.MaxConcurrentBuilds)
	}
	settings["agent_enrollment_token_configured"] = strconv.FormatBool(cfg.Workers.EnrollmentToken != "")
	return settings
}

func validateGlobalSetting(name, value string) error {
	switch name {
	case "host", "artifacts_path", "logs_path", "build_temp_path":
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot be empty", name)
		}
	case "port":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	case "build_timeout":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 30 || parsed > 86400 {
			return fmt.Errorf("build_timeout must be between 30 and 86400 seconds")
		}
	case "build_concurrency", "local_agent_concurrency":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 256 {
			return fmt.Errorf("%s must be between 1 and 256", name)
		}
	case "cpu_limit_percent":
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 5 || parsed > 100 {
			return fmt.Errorf("cpu_limit_percent must be between 5 and 100")
		}
	case "tls", "go_validation_enabled", "node_validation_enabled", "validation_fail_fast", "background_mode":
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("%s must be true or false", name)
		}
	case "retry_policy":
		if value != "never" && value != "failed_once" && value != "failed_twice" {
			return fmt.Errorf("invalid retry_policy")
		}
	case "node_package_manager":
		if value != "npm" && value != "pnpm" && value != "yarn" {
			return fmt.Errorf("invalid node_package_manager")
		}
	case "go_version", "go_checks", "node_version", "node_checks":
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s cannot be empty", name)
		}
	default:
		return fmt.Errorf("unsupported setting %s", name)
	}
	return nil
}

package config

import (
	"os"
	"testing"
	"time"
)

func TestConfigWatcher(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	tmpFile.WriteString(`
server:
  port: 6050
database:
  path: "./data/test.db"
`)
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	_ = cfg

	reloaded := false
	watcher, err := Watch(tmpFile.Name(), func(newCfg *Config) {
		reloaded = true
		if newCfg.Server.Port != 7050 {
			t.Errorf("Port = %d, want 7050", newCfg.Server.Port)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()

	// Modify config
	os.WriteFile(tmpFile.Name(), []byte(`
server:
  port: 7050
database:
  path: "./data/test.db"
`), 0644)

	time.Sleep(500 * time.Millisecond)

	if !reloaded {
		t.Error("Config was not reloaded")
	}
}

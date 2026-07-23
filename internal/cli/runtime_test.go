package cli

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/config"
)

func TestEnsureConfigCreatesSelfContainedDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "buildworld.yaml")
	written, cfg, err := ensureConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if written != path {
		t.Fatalf("config path = %q, want %q", written, path)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("default port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Database.Path == "" || cfg.Storage.BuildTemp == "" || cfg.Plugins.Path == "" {
		t.Fatalf("default config is incomplete: %#v", cfg)
	}
	_, reloaded, err := ensureConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Database.Path != cfg.Database.Path {
		t.Fatalf("existing config was not preserved: %q != %q", reloaded.Database.Path, cfg.Database.Path)
	}
}

func TestEnsureConfigForcesFixedControlPlanePort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "buildworld.yaml")
	contents := "server:\n  host: 127.0.0.1\n  port: 6050\ndatabase:\n  path: buildworld.db\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	_, cfg, err := ensureConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != config.ControlPlanePort {
		t.Fatalf("runtime port = %d, want %d", cfg.Server.Port, config.ControlPlanePort)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != contents {
		t.Fatalf("ensureConfig rewrote user config: %q", written)
	}
}

func TestWaitForPortAvailableDoesNotDisruptOccupant(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	address := listener.Addr().(*net.TCPAddr)

	if waitForPortAvailable("127.0.0.1", address.Port, 20*time.Millisecond) {
		t.Fatal("occupied port reported available")
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("port probe disrupted unrelated listener: %v", err)
	}
	_ = connection.Close()
}

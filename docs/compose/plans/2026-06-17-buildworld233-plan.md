# buildworld233 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use compose:subagent (recommended) or compose:execute to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a modern CI/CD server (Jenkins alternative) in Go 1.26 with TypeScript DSL, plugin system, distributed workers, and React dashboard.

**Architecture:** Central scheduler server + distributed worker nodes. Server handles API, UI, storage, and scheduling. Workers execute builds via gRPC. Plugins run in-process via goja (TypeScript runtime). SQLite for metadata, local filesystem for artifacts.

**Tech Stack:** Go 1.26, SQLite, goja (TypeScript), gRPC, React/Vite, chi (HTTP), cobra (CLI), fsnotify (config reload)

---

## Phase 1: Foundation (Week 1-2)

### Task 1: Go Project Setup

**Covers:** [S8]

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Create: `Makefile`
- Create: `.gitignore`
- Create: `cmd/server/main.go`
- Create: `cmd/worker/main.go`
- Create: `cmd/cli/main.go`

- [ ] **Step 1: Initialize Go module**

```bash
cd D:\Code\neko233-Projects\buildworld233
go mod init github.com/neko233-com/buildworld233
```

- [ ] **Step 2: Create Makefile**

```makefile
.PHONY: build build-server build-worker build-cli test clean

build: build-server build-worker build-cli

build-server:
	go build -o bin/buildworld233.exe ./cmd/server

build-worker:
	go build -o bin/buildworld233-worker.exe ./cmd/worker

build-cli:
	go build -o bin/bwctl.exe ./cmd/cli

test:
	go test ./...

clean:
	rm -rf bin/
```

- [ ] **Step 3: Create .gitignore**

```
bin/
data/
*.db
config.yaml
.env
```

- [ ] **Step 4: Create minimal main.go files**

```go
// cmd/server/main.go
package main

import "fmt"

func main() {
	fmt.Println("buildworld233 server starting...")
}
```

```go
// cmd/worker/main.go
package main

import "fmt"

func main() {
	fmt.Println("buildworld233 worker starting...")
}
```

```go
// cmd/cli/main.go
package main

import "fmt"

func main() {
	fmt.Println("buildworld233 CLI")
}
```

- [ ] **Step 5: Build and verify**

```bash
make build
ls bin/
```

Expected: `buildworld233.exe`, `buildworld233-worker.exe`, `bwctl.exe`

- [ ] **Step 6: Commit**

```bash
git init
git add .
git commit -m "feat: initialize Go project structure"
```

---

### Task 2: Configuration System

**Covers:** [S10, S13]

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `config.yaml`

- [ ] **Step 1: Write the failing test**

```go
// internal/config/config_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/...
```

Expected: FAIL with "undefined: Config"

- [ ] **Step 3: Write minimal implementation**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"time"
	
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
	Workspace  string `yaml:"workspace"`
	Artifacts  string `yaml:"artifacts"`
	Logs       string `yaml:"logs"`
}

type GitConfig struct {
	SSHKeyPath  string `yaml:"ssh_key_path"`
	KnownHosts  string `yaml:"known_hosts"`
}

type WorkersConfig struct {
	Local  LocalWorkerConfig  `yaml:"local"`
	Remote []RemoteWorkerConfig `yaml:"remote"`
}

type LocalWorkerConfig struct {
	Enabled            bool     `yaml:"enabled"`
	MaxConcurrentBuilds int    `yaml:"max_concurrent_builds"`
	Workspace          string   `yaml:"workspace"`
	Labels             []string `yaml:"labels"`
}

type RemoteWorkerConfig struct {
	Name               string   `yaml:"name"`
	Address            string   `yaml:"address"`
	Token              string   `yaml:"token"`
	Labels             []string `yaml:"labels"`
	MaxConcurrentBuilds int    `yaml:"max_concurrent_builds"`
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

// Reload watches config file for changes
func (c *Config) Watch(path string, onChange func(*Config)) error {
	// Will implement with fsnotify in Task 3
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/config/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add configuration system with YAML support"
```

---

### Task 3: Config Hot-Reload

**Covers:** [S13]

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/watcher.go`
- Create: `internal/config/watcher_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/config/watcher_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/...
```

Expected: FAIL with "undefined: Watch"

- [ ] **Step 3: Write minimal implementation**

```go
// internal/config/watcher.go
package config

import (
	"log"
	"time"
	
	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher *fsnotify.Watcher
	stop    chan struct{}
}

func Watch(path string, onChange func(*Config)) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	
	if err := w.Add(path); err != nil {
		w.Close()
		return nil, err
	}
	
	watcher := &Watcher{
		watcher: w,
		stop:    make(chan struct{}),
	}
	
	go watcher.loop(path, onChange)
	
	return watcher, nil
}

func (w *Watcher) loop(path string, onChange func(*Config)) {
	var debounce *time.Timer
	
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(100*time.Millisecond, func() {
					cfg, err := Load(path)
					if err != nil {
						log.Printf("config reload failed: %v", err)
						return
					}
					log.Printf("config reloaded from %s", path)
					onChange(cfg)
				})
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("config watcher error: %v", err)
		case <-w.stop:
			return
		}
	}
}

func (w *Watcher) Stop() {
	close(w.stop)
	w.watcher.Close()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/config/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config hot-reload with fsnotify"
```

---

### Task 4: SQLite Database Layer

**Covers:** [S9]

**Files:**
- Create: `internal/store/sqlite.go`
- Create: `internal/store/models.go`
- Create: `internal/store/migrations.go`
- Create: `internal/store/sqlite_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/store/sqlite_test.go
package store

import (
	"os"
	"testing"
)

func TestSQLiteStore(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()
	
	store, err := New(tmpFile.Name())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()
	
	// Test user creation
	user, err := store.CreateUser("testuser", "test@example.com", "hashedpassword", "admin")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	
	if user.Username != "testuser" {
		t.Errorf("Username = %s, want testuser", user.Username)
	}
	
	// Test user retrieval
	fetched, err := store.GetUserByUsername("testuser")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}
	
	if fetched.ID != user.ID {
		t.Errorf("ID = %d, want %d", fetched.ID, user.ID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/store/...
```

Expected: FAIL with "undefined: New"

- [ ] **Step 3: Write models**

```go
// internal/store/models.go
package store

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	LastLogin    time.Time `json:"last_login,omitempty"`
}

type SSHKey struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Name        string    `json:"name"`
	PublicKey   string    `json:"public_key"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

type Project struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	RepoURL       string    `json:"repo_url"`
	RepoType      string    `json:"repo_type"`
	DefaultBranch string    `json:"default_branch"`
	Config        string    `json:"config"`
	CreatedBy     int64     `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Build struct {
	ID         int64      `json:"id"`
	ProjectID  int64      `json:"project_id"`
	Number     int        `json:"number"`
	Status     string     `json:"status"`
	Trigger    string     `json:"trigger"`
	Branch     string     `json:"branch,omitempty"`
	CommitSHA  string     `json:"commit_sha,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	DurationMs *int64     `json:"duration_ms,omitempty"`
	Log        string     `json:"log,omitempty"`
}

type Artifact struct {
	ID        int64     `json:"id"`
	BuildID   int64     `json:"build_id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type Worker struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Address             string    `json:"address"`
	TokenHash           string    `json:"-"`
	Labels              string    `json:"labels"`
	MaxConcurrentBuilds int       `json:"max_concurrent_builds"`
	Status              string    `json:"status"`
	LastHeartbeat       time.Time `json:"last_heartbeat"`
	CreatedAt           time.Time `json:"created_at"`
}

type Plugin struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description,omitempty"`
	Enabled     bool      `json:"enabled"`
	Config      string    `json:"config,omitempty"`
	InstalledAt time.Time `json:"installed_at"`
}
```

- [ ] **Step 4: Write migrations**

```go
// internal/store/migrations.go
package store

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		email TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'viewer',
		avatar_url TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_login DATETIME
	)`,
	`CREATE TABLE IF NOT EXISTS ssh_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id),
		name TEXT NOT NULL,
		public_key TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		repo_url TEXT NOT NULL,
		repo_type TEXT NOT NULL,
		default_branch TEXT DEFAULT 'main',
		config TEXT NOT NULL,
		created_by INTEGER REFERENCES users(id),
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS builds (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id),
		number INTEGER NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		trigger TEXT NOT NULL,
		branch TEXT,
		commit_sha TEXT,
		started_at DATETIME,
		finished_at DATETIME,
		duration_ms INTEGER,
		log TEXT,
		UNIQUE(project_id, number)
	)`,
	`CREATE TABLE IF NOT EXISTS artifacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		build_id INTEGER NOT NULL REFERENCES builds(id),
		name TEXT NOT NULL,
		path TEXT NOT NULL,
		size INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS workers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		address TEXT NOT NULL,
		token_hash TEXT NOT NULL,
		labels TEXT,
		max_concurrent_builds INTEGER DEFAULT 4,
		status TEXT DEFAULT 'offline',
		last_heartbeat DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE IF NOT EXISTS plugins (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		version TEXT NOT NULL,
		description TEXT,
		enabled BOOLEAN DEFAULT TRUE,
		config TEXT,
		installed_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
}
```

- [ ] **Step 5: Write store implementation**

```go
// internal/store/sqlite.go
package store

import (
	"database/sql"
	"fmt"
	
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	
	return store, nil
}

func (s *Store) migrate() error {
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateUser(username, email, passwordHash, role string) (*User, error) {
	result, err := s.db.Exec(
		"INSERT INTO users (username, email, password_hash, role) VALUES (?, ?, ?, ?)",
		username, email, passwordHash, role,
	)
	if err != nil {
		return nil, err
	}
	
	id, _ := result.LastInsertId()
	return s.GetUser(id)
}

func (s *Store) GetUser(id int64) (*User, error) {
	user := &User{}
	err := s.db.QueryRow(
		"SELECT id, username, email, password_hash, role, avatar_url, created_at, last_login FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role, &user.AvatarURL, &user.CreatedAt, &user.LastLogin)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Store) GetUserByUsername(username string) (*User, error) {
	user := &User{}
	err := s.db.QueryRow(
		"SELECT id, username, email, password_hash, role, avatar_url, created_at, last_login FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.Role, &user.AvatarURL, &user.CreatedAt, &user.LastLogin)
	if err != nil {
		return nil, err
	}
	return user, nil
}
```

- [ ] **Step 6: Add dependency and run test**

```bash
go get github.com/mattn/go-sqlite3
go test ./internal/store/...
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/
git commit -m "feat: add SQLite database layer with migrations"
```

---

### Task 5: CLI Framework

**Covers:** [S12]

**Files:**
- Create: `cmd/cli/main.go`
- Create: `internal/cli/root.go`
- Create: `internal/cli/start.go`
- Create: `internal/cli/status.go`

- [ ] **Step 1: Write root command**

```go
// internal/cli/root.go
package cli

import (
	"fmt"
	"os"
	
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "buildworld233",
	Short: "buildworld233 - A modern CI/CD server",
	Long:  `buildworld233 is a Jenkins alternative with TypeScript DSL, plugin system, and distributed workers.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(versionCmd)
}
```

- [ ] **Step 2: Write start command**

```go
// internal/cli/start.go
package cli

import (
	"fmt"
	
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the buildworld233 server",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		fmt.Printf("Starting buildworld233 on port %d...\n", port)
		// Will implement server start in Task 6
		return nil
	},
}

func init() {
	startCmd.Flags().IntP("port", "p", 6050, "Server port")
	rootCmd.AddCommand(startCmd)
}
```

- [ ] **Step 3: Write status command**

```go
// internal/cli/status.go
package cli

import (
	"fmt"
	
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show server status",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Server status: stopped")
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the buildworld233 server",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Stopping server...")
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("buildworld233 v0.1.0")
	},
}
```

- [ ] **Step 4: Update main.go**

```go
// cmd/cli/main.go
package main

import "github.com/neko233-com/buildworld233/internal/cli"

func main() {
	cli.Execute()
}
```

- [ ] **Step 5: Build and test**

```bash
go get github.com/spf13/cobra
go build -o bin/bwctl.exe ./cmd/cli
./bin/bwctl.exe version
./bin/bwctl.exe status
```

Expected: Version and status output

- [ ] **Step 6: Commit**

```bash
git add cmd/cli/ internal/cli/
git commit -m "feat: add CLI framework with cobra"
```

---

## Phase 2: Core Engine (Week 3-4)

### Task 6: HTTP Server

**Covers:** [S2]

**Files:**
- Create: `internal/api/router.go`
- Create: `internal/api/server.go`
- Create: `internal/api/server_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/api/server_test.go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	
	"github.com/neko233-com/buildworld233/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 6050},
	}
	
	router := NewRouter(cfg)
	
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	
	router.ServeHTTP(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	
	if resp["status"] != "ok" {
		t.Errorf("status = %s, want ok", resp["status"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/api/...
```

Expected: FAIL with "undefined: NewRouter"

- [ ] **Step 3: Write router**

```go
// internal/api/router.go
package api

import (
	"encoding/json"
	"net/http"
	
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	
	"github.com/neko233-com/buildworld233/internal/config"
)

func NewRouter(cfg *config.Config) http.Handler {
	r := chi.NewRouter()
	
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		})
	})
	
	return r
}
```

- [ ] **Step 4: Write server**

```go
// internal/api/server.go
package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
	
	"github.com/neko233-com/buildworld233/internal/config"
)

type Server struct {
	httpServer *http.Server
	cfg        *config.Config
}

func NewServer(cfg *config.Config) *Server {
	router := NewRouter(cfg)
	
	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
			Handler:      router,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		cfg: cfg,
	}
}

func (s *Server) Start() error {
	log.Printf("Starting server on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	log.Println("Stopping server...")
	return s.httpServer.Shutdown(ctx)
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go get github.com/go-chi/chi/v5
go test ./internal/api/...
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/api/
git commit -m "feat: add HTTP server with chi router"
```

---

### Task 7: Build Scheduler

**Covers:** [S3]

**Files:**
- Create: `internal/engine/scheduler.go`
- Create: `internal/engine/scheduler_test.go`
- Create: `internal/engine/task.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/engine/scheduler_test.go
package engine

import (
	"testing"
	"time"
)

func TestSchedulerEnqueue(t *testing.T) {
	scheduler := NewScheduler()
	
	task := &Task{
		ID:        "build-1",
		ProjectID: 1,
		BuildID:   1,
		Priority:  1,
		CreatedAt: time.Now(),
	}
	
	err := scheduler.Enqueue(task)
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	
	if scheduler.QueueLen() != 1 {
		t.Errorf("QueueLen = %d, want 1", scheduler.QueueLen())
	}
}

func TestSchedulerDequeue(t *testing.T) {
	scheduler := NewScheduler()
	
	task := &Task{
		ID:        "build-1",
		ProjectID: 1,
		BuildID:   1,
		Priority:  1,
		CreatedAt: time.Now(),
	}
	
	scheduler.Enqueue(task)
	
	dequeued, err := scheduler.Dequeue()
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	
	if dequeued.ID != "build-1" {
		t.Errorf("ID = %s, want build-1", dequeued.ID)
	}
	
	if scheduler.QueueLen() != 0 {
		t.Errorf("QueueLen = %d, want 0", scheduler.QueueLen())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/engine/...
```

Expected: FAIL with "undefined: NewScheduler"

- [ ] **Step 3: Write task model**

```go
// internal/engine/task.go
package engine

import "time"

type Task struct {
	ID          string
	ProjectID   int64
	BuildID     int64
	Priority    int
	Status      string
	AssignedTo  string
	CreatedAt   time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
}

type TaskStatus string

const (
	TaskStatusPending  TaskStatus = "pending"
	TaskStatusQueued   TaskStatus = "queued"
	TaskStatusRunning  TaskStatus = "running"
	TaskStatusSuccess  TaskStatus = "success"
	TaskStatusFailed   TaskStatus = "failed"
)
```

- [ ] **Step 4: Write scheduler**

```go
// internal/engine/scheduler.go
package engine

import (
	"container/heap"
	"fmt"
	"sync"
)

type Scheduler struct {
	mu    sync.Mutex
	queue taskQueue
}

func NewScheduler() *Scheduler {
	s := &Scheduler{}
	heap.Init(&s.queue)
	return s
}

func (s *Scheduler) Enqueue(task *Task) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	heap.Push(&s.queue, task)
	return nil
}

func (s *Scheduler) Dequeue() (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.queue.Len() == 0 {
		return nil, fmt.Errorf("queue is empty")
	}
	
	task := heap.Pop(&s.queue).(*Task)
	return task, nil
}

func (s *Scheduler) QueueLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.Len()
}

// taskQueue implements heap.Interface
type taskQueue []*Task

func (q taskQueue) Len() int { return len(q) }

func (q taskQueue) Less(i, j int) bool {
	return q[i].Priority > q[j].Priority
}

func (q taskQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *taskQueue) Push(x interface{}) {
	*q = append(*q, x.(*Task))
}

func (q *taskQueue) Pop() interface{} {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[0 : n-1]
	return item
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/engine/...
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/engine/
git commit -m "feat: add build scheduler with priority queue"
```

---

### Task 8: Shell Executor

**Covers:** [S3]

**Files:**
- Create: `internal/engine/executor.go`
- Create: `internal/engine/executor_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/engine/executor_test.go
package engine

import (
	"context"
	"testing"
)

func TestExecutorRun(t *testing.T) {
	executor := NewExecutor()
	
	ctx := context.Background()
	output, err := executor.Run(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	
	if output != "hello\n" {
		t.Errorf("output = %q, want %q", output, "hello\n")
	}
}

func TestExecutorRunError(t *testing.T) {
	executor := NewExecutor()
	
	ctx := context.Background()
	_, err := executor.Run(ctx, "false")
	if err == nil {
		t.Error("Run() should fail on 'false' command")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/engine/...
```

Expected: FAIL with "undefined: NewExecutor"

- [ ] **Step 3: Write executor**

```go
// internal/engine/executor.go
package engine

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type Executor struct {
	workspace string
}

func NewExecutor() *Executor {
	return &Executor{}
}

func (e *Executor) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command failed: %w, stderr: %s", err, stderr.String())
	}
	
	return stdout.String(), nil
}

func (e *Executor) RunWithOutput(ctx context.Context, name string, args ...string, onOutput func(string)) error {
	cmd := exec.CommandContext(ctx, name, args...)
	
	cmd.Stdout = &lineWriter{callback: onOutput}
	cmd.Stderr = &lineWriter{callback: onOutput}
	
	return cmd.Run()
}

type lineWriter struct {
	callback func(string)
}

func (w *lineWriter) Write(p []byte) (n int, err error) {
	w.callback(string(p))
	return len(p), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/engine/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/executor.go
git commit -m "feat: add shell executor"
```

---

## Phase 3: Worker System (Week 5-6)

### Task 9: gRPC Proto Definitions

**Covers:** [S3.5]

**Files:**
- Create: `proto/worker.proto`
- Create: `internal/rpc/generated/worker.pb.go`

- [ ] **Step 1: Create proto file**

```protobuf
// proto/worker.proto
syntax = "proto3";

package worker;

option go_package = "github.com/neko233-com/buildworld233/internal/rpc/generated";

service WorkerService {
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc ExecuteBuild(BuildRequest) returns (stream BuildResponse);
  rpc ReportStatus(StatusRequest) returns (StatusResponse);
}

message RegisterRequest {
  string worker_id = 1;
  string name = 2;
  string address = 3;
  string token = 4;
  repeated string labels = 5;
  int32 max_concurrent_builds = 6;
}

message RegisterResponse {
  bool success = 1;
  string message = 2;
}

message HeartbeatRequest {
  string worker_id = 1;
  float cpu_usage = 2;
  float memory_usage = 3;
  float disk_usage = 4;
  int32 active_builds = 5;
}

message HeartbeatResponse {
  bool acknowledged = 1;
}

message BuildRequest {
  string build_id = 1;
  string project_name = 2;
  string repo_url = 3;
  string branch = 4;
  string commit_sha = 5;
  string pipeline_config = 6;
}

message BuildResponse {
  string build_id = 1;
  string stage = 2;
  string step = 3;
  string output = 4;
  string status = 5;
  bool is_error = 6;
}

message StatusRequest {
  string worker_id = 1;
}

message StatusResponse {
  string worker_id = 1;
  string status = 2;
  int32 active_builds = 3;
  int32 max_builds = 4;
}
```

- [ ] **Step 2: Generate Go code**

```bash
protoc --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  proto/worker.proto
```

- [ ] **Step 3: Commit**

```bash
git add proto/ internal/rpc/
git commit -m "feat: add gRPC proto definitions for worker system"
```

---

### Task 10: Worker Daemon

**Covers:** [S3.5]

**Files:**
- Create: `internal/worker/daemon.go`
- Create: `internal/worker/executor.go`
- Create: `internal/worker/health.go`

- [ ] **Step 1: Create worker daemon**

```go
// internal/worker/daemon.go
package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	
	"github.com/google/uuid"
	"google.golang.org/grpc"
	
	"github.com/neko233-com/buildworld233/internal/config"
	pb "github.com/neko233-com/buildworld233/internal/rpc/generated"
)

type Daemon struct {
	id       string
	name     string
	server   string
	token    string
	labels   []string
	maxBuilds int
	
	conn     *grpc.ClientConn
	client   pb.WorkerServiceClient
	health   *HealthMonitor
	executor *Executor
	
	stopCh   chan struct{}
}

func NewDaemon(cfg *config.Config, serverAddr, token string) *Daemon {
	return &Daemon{
		id:        uuid.New().String(),
		name:      cfg.Workers.Local.Workspace,
		server:    serverAddr,
		token:     token,
		labels:    cfg.Workers.Local.Labels,
		maxBuilds: cfg.Workers.Local.MaxConcurrentBuilds,
		health:    NewHealthMonitor(),
		executor:  NewExecutor(),
		stopCh:    make(chan struct{}),
	}
}

func (d *Daemon) Start(ctx context.Context) error {
	log.Printf("Starting worker daemon %s", d.id)
	
	// Connect to server
	conn, err := grpc.Dial(d.server, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}
	d.conn = conn
	d.client = pb.NewWorkerServiceClient(conn)
	
	// Register with server
	err = d.register()
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}
	
	// Start heartbeat
	go d.heartbeatLoop(ctx)
	
	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	
	select {
	case <-sigCh:
		log.Println("Received shutdown signal")
	case <-ctx.Done():
		log.Println("Context cancelled")
	case <-d.stopCh:
		log.Println("Stop requested")
	}
	
	return d.Stop()
}

func (d *Daemon) Stop() error {
	log.Printf("Stopping worker daemon %s", d.id)
	
	if d.conn != nil {
		d.conn.Close()
	}
	
	return nil
}

func (d *Daemon) register() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	resp, err := d.client.Register(ctx, &pb.RegisterRequest{
		WorkerId:           d.id,
		Name:               d.name,
		Address:            d.server,
		Token:              d.token,
		Labels:             d.labels,
		MaxConcurrentBuilds: int32(d.maxBuilds),
	})
	
	if err != nil {
		return err
	}
	
	if !resp.Success {
		return fmt.Errorf("registration failed: %s", resp.Message)
	}
	
	log.Printf("Registered with server as %s", d.id)
	return nil
}

func (d *Daemon) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			d.sendHeartbeat()
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		}
	}
}

func (d *Daemon) sendHeartbeat() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	metrics := d.health.GetMetrics()
	
	_, err := d.client.Heartbeat(ctx, &pb.HeartbeatRequest{
		WorkerId:     d.id,
		CpuUsage:     metrics.CPU,
		MemoryUsage:  metrics.Memory,
		DiskUsage:    metrics.Disk,
		ActiveBuilds: int32(metrics.ActiveBuilds),
	})
	
	if err != nil {
		log.Printf("Heartbeat failed: %v", err)
	}
}
```

- [ ] **Step 2: Create health monitor**

```go
// internal/worker/health.go
package worker

import (
	"runtime"
	"sync"
	"time"
)

type Metrics struct {
	CPU          float64
	Memory       float64
	Disk         float64
	ActiveBuilds int
	Uptime       time.Duration
}

type HealthMonitor struct {
	mu          sync.RWMutex
	startTime   time.Time
	activeBuilds int
}

func NewHealthMonitor() *HealthMonitor {
	return &HealthMonitor{
		startTime: time.Now(),
	}
}

func (h *HealthMonitor) GetMetrics() Metrics {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	return Metrics{
		CPU:          0, // TODO: implement CPU monitoring
		Memory:       float64(m.Alloc) / float64(m.Sys),
		Disk:         0, // TODO: implement disk monitoring
		ActiveBuilds: h.activeBuilds,
		Uptime:       time.Since(h.startTime),
	}
}

func (h *HealthMonitor) IncrementBuilds() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.activeBuilds++
}

func (h *HealthMonitor) DecrementBuilds() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.activeBuilds--
}
```

- [ ] **Step 3: Update worker main.go**

```go
// cmd/worker/main.go
package main

import (
	"context"
	"flag"
	"log"
	
	"github.com/neko233-com/buildworld233/internal/config"
	"github.com/neko233-com/buildworld233/internal/worker"
)

func main() {
	server := flag.String("server", "http://localhost:6050", "Server address")
	token := flag.String("token", "", "Registration token")
	name := flag.String("name", "worker-1", "Worker name")
	flag.Parse()
	
	cfg := &config.Config{
		Workers: config.WorkersConfig{
			Local: config.LocalWorkerConfig{
				Enabled:            true,
				MaxConcurrentBuilds: 4,
				Labels:             []string{"default"},
			},
		},
	}
	
	daemon := worker.NewDaemon(cfg, *server, *token)
	
	if err := daemon.Start(context.Background()); err != nil {
		log.Fatalf("Failed to start worker: %v", err)
	}
}
```

- [ ] **Step 4: Build and test**

```bash
go get github.com/google/uuid google.golang.org/grpc
go build -o bin/buildworld233-worker.exe ./cmd/worker
```

- [ ] **Step 5: Commit**

```bash
git add internal/worker/ cmd/worker/
git commit -m "feat: add worker daemon with gRPC registration"
```

---

## Phase 4: Plugin System (Week 7-8)

### Task 11: Plugin Loader (goja)

**Covers:** [S4]

**Files:**
- Create: `internal/plugin/loader.go`
- Create: `internal/plugin/loader_test.go`
- Create: `internal/plugin/types.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/plugin/loader_test.go
package plugin

import (
	"testing"
)

func TestPluginLoader(t *testing.T) {
	loader := NewLoader("./test-plugins")
	
	err := loader.Load("test-plugin")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	
	plugin := loader.Get("test-plugin")
	if plugin == nil {
		t.Fatal("Get() returned nil")
	}
	
	if plugin.Name != "test-plugin" {
		t.Errorf("Name = %s, want test-plugin", plugin.Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/plugin/...
```

Expected: FAIL with "undefined: NewLoader"

- [ ] **Step 3: Write plugin types**

```go
// internal/plugin/types.go
package plugin

type Plugin struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Steps       map[string]Step   `json:"steps,omitempty"`
	Triggers    map[string]Trigger `json:"triggers,omitempty"`
}

type Step struct {
	Name        string
	Description string
	Execute     func(ctx StepContext) error
}

type Trigger struct {
	Name        string
	Description string
	Activate    func(config map[string]interface{}) error
}

type StepContext struct {
	Workspace  string
	Env        map[string]string
	Args       map[string]interface{}
	Logger     func(string)
}
```

- [ ] **Step 4: Write loader**

```go
// internal/plugin/loader.go
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/nicholasgasior/gopher-lua-luar/pkg/gopher-lua-luar"
	"github.com/yuin/gopher-lua"
)

type Loader struct {
	path    string
	plugins map[string]*Plugin
}

func NewLoader(path string) *Loader {
	return &Loader{
		path:    path,
		plugins: make(map[string]*Plugin),
	}
}

func (l *Loader) Load(name string) error {
	pluginPath := filepath.Join(l.path, name)
	
	// Read plugin metadata
	metaPath := filepath.Join(pluginPath, "plugin.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("read plugin.json: %w", err)
	}
	
	plugin := &Plugin{}
	if err := json.Unmarshal(metaData, plugin); err != nil {
		return fmt.Errorf("parse plugin.json: %w", err)
	}
	
	// Load plugin script
	scriptPath := filepath.Join(pluginPath, "index.lua")
	scriptData, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("read index.lua: %w", err)
	}
	
	// Execute script in Lua VM
	L := lua.NewState()
	defer L.Close()
	
	if err := L.DoString(string(scriptData)); err != nil {
		return fmt.Errorf("execute plugin: %w", err)
	}
	
	l.plugins[name] = plugin
	return nil
}

func (l *Loader) Get(name string) *Plugin {
	return l.plugins[name]
}

func (l *Loader) List() []*Plugin {
	var list []*Plugin
	for _, p := range l.plugins {
		list = append(list, p)
	}
	return list
}

func (l *Loader) Unload(name string) error {
	delete(l.plugins, name)
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go get github.com/yuin/gopher-lua
go test ./internal/plugin/...
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/plugin/
git commit -m "feat: add plugin loader with Lua runtime"
```

---

### Task 12: Plugin Hot-Reload

**Covers:** [S4]

**Files:**
- Create: `internal/plugin/hotreload.go`
- Create: `internal/plugin/hotreload_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/plugin/hotreload_test.go
package plugin

import (
	"os"
	"testing"
	"time"
)

func TestPluginHotReload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	
	// Create initial plugin
	pluginDir := tmpDir + "/test-plugin"
	os.MkdirAll(pluginDir, 0755)
	
	os.WriteFile(pluginDir+"/plugin.json", []byte(`{"name":"test","version":"1.0.0"}`), 0644)
	os.WriteFile(pluginDir+"/index.lua", []byte(`name = "test"`), 0644)
	
	loader := NewLoader(tmpDir)
	watcher, err := NewHotReload(loader)
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()
	
	// Start watching
	watcher.Watch("test-plugin")
	
	// Modify plugin
	os.WriteFile(pluginDir+"/index.lua", []byte(`name = "test-v2"`), 0644)
	
	time.Sleep(500 * time.Millisecond)
	
	// Plugin should be reloaded
	plugin := loader.Get("test-plugin")
	if plugin == nil {
		t.Error("Plugin should be loaded")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/plugin/...
```

Expected: FAIL with "undefined: NewHotReload"

- [ ] **Step 3: Write hot-reload**

```go
// internal/plugin/hotreload.go
package plugin

import (
	"log"
	"path/filepath"
	"time"
	
	"github.com/fsnotify/fsnotify"
)

type HotReload struct {
	watcher *fsnotify.Watcher
	loader  *Loader
	stop    chan struct{}
}

func NewHotReload(loader *Loader) (*HotReload, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	
	return &HotReload{
		watcher: w,
		loader:  loader,
		stop:    make(chan struct{}),
	}, nil
}

func (h *HotReload) Watch(pluginName string) {
	pluginPath := filepath.Join(h.loader.path, pluginName)
	h.watcher.Add(pluginPath)
	go h.loop()
}

func (h *HotReload) loop() {
	for {
		select {
		case event, ok := <-h.watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				pluginName := filepath.Base(filepath.Dir(event.Name))
				log.Printf("Plugin %s changed, reloading...", pluginName)
				h.loader.Unload(pluginName)
				h.loader.Load(pluginName)
			}
		case <-h.stop:
			return
		}
	}
}

func (h *HotReload) Stop() {
	close(h.stop)
	h.watcher.Close()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/plugin/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/plugin/hotreload.go
git commit -m "feat: add plugin hot-reload with fsnotify"
```

---

## Phase 5: Auth & VCS (Week 9-10)

### Task 13: JWT Authentication

**Covers:** [S5]

**Files:**
- Create: `internal/auth/jwt.go`
- Create: `internal/auth/jwt_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/auth/jwt_test.go
package auth

import (
	"testing"
	"time"
)

func TestJWTGenerate(t *testing.T) {
	jwt := NewJWT("secret")
	
	token, err := jwt.Generate(1, "admin", 24*time.Hour)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	
	if token == "" {
		t.Error("Token should not be empty")
	}
}

func TestJWTValidate(t *testing.T) {
	jwt := NewJWT("secret")
	
	token, _ := jwt.Generate(1, "admin", 24*time.Hour)
	
	claims, err := jwt.Validate(token)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	
	if claims.UserID != 1 {
		t.Errorf("UserID = %d, want 1", claims.UserID)
	}
	
	if claims.Role != "admin" {
		t.Errorf("Role = %s, want admin", claims.Role)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/auth/...
```

Expected: FAIL with "undefined: NewJWT"

- [ ] **Step 3: Write JWT implementation**

```go
// internal/auth/jwt.go
package auth

import (
	"fmt"
	"time"
	
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type JWT struct {
	secret []byte
}

func NewJWT(secret string) *JWT {
	return &JWT{secret: []byte(secret)}
}

func (j *JWT) Generate(userID int64, role string, duration time.Duration) (string, error) {
	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secret)
}

func (j *JWT) Validate(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.secret, nil
	})
	
	if err != nil {
		return nil, err
	}
	
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	
	return nil, fmt.Errorf("invalid token")
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go get github.com/golang-jwt/jwt/v5
go test ./internal/auth/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/auth/
git commit -m "feat: add JWT authentication"
```

---

### Task 14: Git Client

**Covers:** [S6]

**Files:**
- Create: `internal/git/client.go`
- Create: `internal/git/client_test.go`

- [ ] **Step 1: Write the failing test**

```bash
# Create test repo
mkdir -p /tmp/test-repo && cd /tmp/test-repo
git init
echo "test" > file.txt
git add . && git commit -m "init"
```

```go
// internal/git/client_test.go
package git

import (
	"testing"
)

func TestGitClone(t *testing.T) {
	client := NewClient()
	
	err := client.Clone("/tmp/test-repo", "/tmp/test-clone")
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	
	// Verify clone exists
	_, err = os.Stat("/tmp/test-clone/.git")
	if os.IsNotExist(err) {
		t.Error("Clone did not create .git directory")
	}
	
	os.RemoveAll("/tmp/test-clone")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/git/...
```

Expected: FAIL with "undefined: NewClient"

- [ ] **Step 3: Write git client**

```go
// internal/git/client.go
package git

import (
	"fmt"
	"os/exec"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Clone(url, dest string) error {
	cmd := exec.Command("git", "clone", url, dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w, output: %s", err, output)
	}
	return nil
}

func (c *Client) Pull(repoPath string) error {
	cmd := exec.Command("git", "-C", repoPath, "pull")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %w, output: %s", err, output)
	}
	return nil
}

func (c *Client) Checkout(repoPath, branch string) error {
	cmd := exec.Command("git", "-C", repoPath, "checkout", branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout failed: %w, output: %s", err, output)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/git/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/git/
git commit -m "feat: add Git client for clone/pull/checkout"
```

---

## Phase 6: Web UI (Week 11-14)

### Task 15: React/Vite Setup

**Covers:** [S7]

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.ts`
- Create: `web/src/App.tsx`
- Create: `web/src/main.tsx`

- [ ] **Step 1: Initialize React project**

```bash
cd web
npm create vite@latest . -- --template react-ts
npm install
```

- [ ] **Step 2: Install dependencies**

```bash
npm install react-router-dom axios @tanstack/react-query
npm install -D tailwindcss postcss autoprefixer
npx tailwindcss init -p
```

- [ ] **Step 3: Configure Tailwind**

```typescript
// web/tailwind.config.js
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {},
  },
  plugins: [],
}
```

```css
/* web/src/index.css */
@tailwind base;
@tailwind components;
@tailwind utilities;
```

- [ ] **Step 4: Create main app**

```tsx
// web/src/App.tsx
import { BrowserRouter, Routes, Route } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import Projects from './pages/Projects'
import Builds from './pages/Builds'

function App() {
  return (
    <BrowserRouter>
      <div className="min-h-screen bg-gray-100">
        <nav className="bg-white shadow">
          <div className="max-w-7xl mx-auto px-4">
            <div className="flex justify-between h-16">
              <div className="flex">
                <a href="/" className="flex items-center px-2 py-2 text-gray-900 font-bold">
                  buildworld233
                </a>
                <a href="/projects" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  Projects
                </a>
                <a href="/builds" className="flex items-center px-2 py-2 text-gray-600 hover:text-gray-900">
                  Builds
                </a>
              </div>
            </div>
          </div>
        </nav>
        <main className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/projects" element={<Projects />} />
            <Route path="/builds" element={<Builds />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}

export default App
```

```tsx
// web/src/main.tsx
import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
```

- [ ] **Step 5: Create page components**

```tsx
// web/src/pages/Dashboard.tsx
export default function Dashboard() {
  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">Dashboard</h1>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="bg-white p-6 rounded-lg shadow">
          <h2 className="text-lg font-semibold">Projects</h2>
          <p className="text-3xl font-bold text-blue-600">12</p>
        </div>
        <div className="bg-white p-6 rounded-lg shadow">
          <h2 className="text-lg font-semibold">Active Builds</h2>
          <p className="text-3xl font-bold text-green-600">3</p>
        </div>
        <div className="bg-white p-6 rounded-lg shadow">
          <h2 className="text-lg font-semibold">Workers</h2>
          <p className="text-3xl font-bold text-purple-600">5</p>
        </div>
      </div>
    </div>
  )
}
```

```tsx
// web/src/pages/Projects.tsx
export default function Projects() {
  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">Projects</h1>
      <div className="bg-white shadow rounded-lg">
        <table className="min-w-full">
          <thead>
            <tr className="border-b">
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Name</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Status</th>
              <th className="px-6 py-3 text-left text-sm font-medium text-gray-500">Last Build</th>
            </tr>
          </thead>
          <tbody>
            <tr className="border-b">
              <td className="px-6 py-4">my-app</td>
              <td className="px-6 py-4"><span className="text-green-600">Active</span></td>
              <td className="px-6 py-4">2 minutes ago</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  )
}
```

```tsx
// web/src/pages/Builds.tsx
export default function Builds() {
  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">Build History</h1>
      <div className="bg-white shadow rounded-lg p-6">
        <p className="text-gray-500">No builds yet</p>
      </div>
    </div>
  )
}
```

- [ ] **Step 6: Build and verify**

```bash
cd web
npm run build
```

Expected: Build succeeds

- [ ] **Step 7: Commit**

```bash
git add web/
git commit -m "feat: add React/Vite frontend with Tailwind"
```

---

## Phase 7: Polish & Release (Week 15-16)

### Task 16: GitHub Actions CI/CD

**Covers:** [S16]

**Files:**
- Create: `.github/workflows/test.yml`
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create test workflow**

```yaml
# .github/workflows/test.yml
name: Tests

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      
      - name: Run tests
        run: go test ./...
      
      - name: Build
        run: make build
```

- [ ] **Step 2: Create release workflow**

```yaml
# .github/workflows/release.yml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        goos: [linux, darwin, windows]
        goarch: [amd64, arm64]
    steps:
      - uses: actions/checkout@v4
      
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      
      - name: Build
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: |
          binary="buildworld233-${{ matrix.goos }}-${{ matrix.goarch }}"
          if [ "${{ matrix.goos }}" = "windows" ]; then
            binary="${binary}.exe"
          fi
          go build -o $binary ./cmd/server
          go build -o "${binary/233/233-worker}" ./cmd/worker
      
      - name: Upload to Release
        uses: softprops/action-gh-release@v1
        with:
          files: buildworld233-*
```

- [ ] **Step 3: Commit**

```bash
git add .github/
git commit -m "feat: add GitHub Actions CI/CD workflows"
```

---

### Task 17: Install Scripts

**Covers:** [S11]

**Files:**
- Create: `scripts/install.ps1`
- Create: `scripts/install.sh`
- Create: `scripts/install-worker.ps1`
- Create: `scripts/install-worker.sh`

- [ ] **Step 1: Create Windows server installer**

```powershell
# scripts/install.ps1
param([string]$Version = "latest")
$ErrorActionPreference = "Stop"
$BinaryName = "buildworld233"
$Repo = "neko233-com/buildworld233"

function Get-LatestVersion {
    try {
        $r = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
        return ($r.tag_name -replace '^[vV]', '')
    } catch {
        return "0.1.0"
    }
}

function Install-BuildWorld233 {
    param([string]$Ver)
    $arch = "amd64"
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { $arch = "arm64" }
    $asset = "$BinaryName-windows-$arch.exe"
    $url = "https://github.com/$Repo/releases/download/v$Ver/$asset"
    $installDir = Join-Path $env:LOCALAPPDATA "buildworld233"
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    $dest = Join-Path $installDir "$BinaryName.exe"
    Write-Host "Downloading $url ..."
    Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing
    Write-Host "Installed to $dest"
    Write-Host "Run: buildworld233 start"
}

if ($Version -eq "latest") { $Version = Get-LatestVersion }
$Version = $Version -replace '^[vV]', ''
Write-Host "Installing buildworld233 v$Version ..."
Install-BuildWorld233 -Ver $Version
```

- [ ] **Step 2: Create Linux/macOS server installer**

```bash
#!/bin/bash
# scripts/install.sh
set -e

REPO="neko233-com/buildworld233"
BINARY="buildworld233"
VERSION="${1:-latest}"

get_latest_version() {
    curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/' || echo "0.1.0"
}

install() {
    local ver="$1"
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)
    case "$arch" in
        x86_64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
    esac
    local asset="${BINARY}-${os}-${arch}"
    local url="https://github.com/$REPO/releases/download/v$ver/$asset"
    local install_dir="/usr/local/bin"
    echo "Downloading $url ..."
    sudo curl -fsSL "$url" -o "$install_dir/$BINARY"
    sudo chmod +x "$install_dir/$BINARY"
    echo "Installed to $install_dir/$BINARY"
    echo "Run: buildworld233 start"
}

if [ "$VERSION" = "latest" ]; then
    VERSION=$(get_latest_version)
fi
VERSION="${VERSION#v}"
echo "Installing buildworld233 v$VERSION ..."
install "$VERSION"
```

- [ ] **Step 3: Commit**

```bash
git add scripts/
git commit -m "feat: add install scripts for server and worker"
```

---

### Task 18: Final Integration

**Covers:** [S2, S3, S4, S5, S6, S7, S8, S9, S10, S11, S12, S13, S14, S15, S16, S17, S18]

- [ ] **Step 1: Update main server to use all components**

```go
// cmd/server/main.go
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	
	"github.com/neko233-com/buildworld233/internal/api"
	"github.com/neko233-com/buildworld233/internal/config"
	"github.com/neko233-com/buildworld233/internal/engine"
	"github.com/neko233-com/buildworld233/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Config file path")
	flag.Parse()
	
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	
	db, err := store.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	
	scheduler := engine.NewScheduler()
	server := api.NewServer(cfg)
	
	// Start config watcher
	watcher, err := config.Watch(*configPath, func(newCfg *config.Config) {
		log.Println("Config reloaded")
		cfg = newCfg
	})
	if err != nil {
		log.Printf("Failed to watch config: %v", err)
	}
	defer watcher.Stop()
	
	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()
	
	<-sigCh
	log.Println("Shutting down...")
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	server.Stop(ctx)
}
```

- [ ] **Step 2: Run full test suite**

```bash
go test ./...
```

Expected: All tests pass

- [ ] **Step 3: Build binaries**

```bash
make build
```

Expected: All binaries built successfully

- [ ] **Step 4: Final commit**

```bash
git add .
git commit -m "feat: complete buildworld233 v1.0.0"
```

---

## Summary

**Total Tasks:** 18
**Estimated Duration:** 16 weeks
**Key Milestones:**
- Week 2: Foundation complete (config, database, CLI)
- Week 4: Core engine working (scheduler, executor)
- Week 6: Worker system operational
- Week 8: Plugin system functional
- Week 10: Auth and VCS integrated
- Week 14: Web UI complete
- Week 16: Release ready

**Dependencies:**
- Phase 1 → Phase 2 → Phase 3 (sequential)
- Phase 4 (plugins) can run parallel to Phase 3
- Phase 5 (auth) can run parallel to Phase 4
- Phase 6 (UI) depends on API from Phase 2
- Phase 7 (polish) depends on all previous phases

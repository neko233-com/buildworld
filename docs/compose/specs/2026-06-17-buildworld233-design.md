# buildworld233 Design Specification

**Date:** 2026-06-17
**Version:** 1.0.0
**Status:** Draft

---

## [S1] Problem Statement

Jenkins is the most widely used CI/CD server but suffers from:
- **XML-based configuration** — fragile, hard to version control
- **Groovy DSL** — steep learning curve, security concerns (CPS serialization)
- **Plugin sprawl** — 1800+ plugins, many abandoned, compatibility matrix is nightmarish
- **Java dependency** — heavy JVM footprint, slow startup
- **UI is dated** — clunky, not modern web standards
- **Account management** — limited, requires plugins for OAuth/LDAP
- **No built-in TypeScript support** — Groovy-only scripting

**buildworld233** aims to solve all these pain points while keeping Jenkins' proven concepts (pipelines, agents, workspaces, artifact storage).

---

## [S2] Solution Overview — High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          buildworld233 (Server)                             │
├──────────────┬──────────────┬──────────────┬───────────────┬───────────────┤
│   Web UI     │  REST API    │  WebSocket   │  CLI (bwctl)  │  Worker RPC   │
│  (React/Vite)│  (chi)       │  (build log  │  (cobra)      │  (gRPC)       │
│              │              │   streaming) │               │               │
├──────────────┴──────────────┴──────────────┴───────────────┴───────────────┤
│                          Core Engine                                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐ ┌────────────┐  │
│  │ Build    │ │ Pipeline │ │ Worker   │ │ Workspace    │ │ Task       │  │
│  │ Scheduler│ │ Executor │ │ Pool     │ │ Manager      │ │ Queue      │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────┘ └────────────┘  │
├─────────────────────────────────────────────────────────────────────────────┤
│                        Plugin System (goja)                                │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐ ┌────────────┐  │
│  │ Plugin   │ │ Plugin   │ │ Plugin   │ │ Plugin       │ │ Plugin     │  │
│  │ Loader   │ │ Registry │ │ Sandbox  │ │ Hot-Reload   │ │ Store      │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────┘ └────────────┘  │
├─────────────────────────────────────────────────────────────────────────────┤
│                        Storage Layer (SQLite)                              │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐ ┌────────────┐  │
│  │ Users    │ │ Projects │ │ Builds   │ │ Artifacts    │ │ Workers    │  │
│  │ & Auth   │ │ & Config │ │ History  │ │ & Logs       │ │ Registry   │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────┘ └────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              │               │               │
    ┌─────────▼───────┐ ┌────▼─────────────┐ ┌▼────────────────┐
    │  Local Worker   │ │  Remote Worker   │ │  Remote Worker  │
    │  (built-in)     │ │  (node-01)       │ │  (node-02)      │
    │                 │ │                  │ │                 │
    │  ┌───────────┐  │ │  ┌───────────┐   │ │  ┌───────────┐  │
    │  │ Worker    │  │ │  │ Worker    │   │ │  │ Worker    │  │
    │  │ Daemon    │  │ │  │ Daemon    │   │ │  │ Daemon    │  │
    │  └───────────┘  │ │  └───────────┘   │ │  └───────────┘  │
    │  ┌───────────┐  │ │  ┌───────────┐   │ │  ┌───────────┐  │
    │  │ Executor  │  │ │  │ Executor  │   │ │  │ Executor  │  │
    │  │ Pool      │  │ │  │ Pool      │   │ │  │ Pool      │  │
    │  └───────────┘  │ │  └───────────┘   │ │  └───────────┘  │
    └─────────────────┘ └──────────────────┘ └─────────────────┘
```

**Key decisions:**
- **Go 1.26** as the primary language (backend + build engine)
- **SQLite** for metadata storage (zero-config, embedded)
- **goja** for TypeScript DSL execution (in-process hot-reload)
- **React + Vite** for the web dashboard
- **RESTful JSON API** + WebSocket for real-time build logs
- **Direct shell execution** for build steps
- **Local + OAuth** authentication (GitHub/GitLab)
- **Default port: 6050**

---

## [S3] Core Engine Design

### Build Pipeline Model

```typescript
// Example buildworld233 pipeline (TypeScript DSL)
pipeline({
  name: "my-app-build",
  triggers: {
    github: { webhook: true },
    schedule: "0 2 * * *",  // cron
  },
  environment: {
    NODE_ENV: "production",
  },
  stages: [
    {
      name: "Checkout",
      steps: [
        git.clone("https://github.com/user/repo", { depth: 1 }),
      ],
    },
    {
      name: "Build",
      steps: [
        shell("npm ci"),
        shell("npm run build"),
      ],
    },
    {
      name: "Test",
      steps: [
        shell("npm test"),
      ],
      parallel: true,
    },
    {
      name: "Deploy",
      steps: [
        shell("aws s3 sync ./dist s3://my-bucket"),
      ],
      when: {
        branch: "main",
        condition: "success",
      },
    },
  ],
  post: {
    always: [
      shell("echo 'Build finished'"),
    ],
    failure: [
      notify.slack("#builds", "Build failed!"),
    ],
  },
});
```

### Build Execution Flow

```
Trigger (webhook/cron/manual)
  → Scheduler picks up build
  → Worker Pool assigns available worker
  → Allocates workspace on worker (clean or reuse)
  → Executes pipeline stages sequentially
  → Each stage runs steps in subprocesses on worker
  → Streams logs via WebSocket (worker → server → client)
  → Stores artifacts and metadata in SQLite (on server)
  → Runs post-build actions
  → Notifies (email/Slack/webhook)
```

---

## [S3.5] Worker Node System

### Overview

buildworld233 uses a **central scheduler + distributed worker** architecture:

- **Server** = Scheduler + API + UI + Storage (the brain)
- **Worker** = Build executor (the muscle)
- **Local worker** = Built-in, starts automatically with server
- **Remote workers** = Connect via gRPC, can be added on-demand

### Worker Configuration

```yaml
# config.yaml
workers:
  # Local worker (built-in, always available)
  local:
    enabled: true
    max_concurrent_builds: 4  # number of parallel builds
    workspace: "./data/workspaces"
    labels: ["linux", "amd64", "default"]
    
  # Remote worker registration
  remote:
    - name: "node-01"
      address: "192.168.1.100:7050"
      token: "worker-registration-token"
      labels: ["linux", "amd64", "gpu"]
      max_concurrent_builds: 8
      
    - name: "node-02"
      address: "192.168.1.101:7050"
      token: "worker-registration-token"
      labels: ["macos", "arm64", "ios"]
      max_concurrent_builds: 4
      
    - name: "windows-node"
      address: "192.168.1.102:7050"
      token: "worker-registration-token"
      labels: ["windows", "amd64", "dotnet"]
      max_concurrent_builds: 4
```

### Worker Node Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Worker Node                              │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────┐   │
│  │  gRPC Server (port 7050)                            │   │
│  │  - Register with server                             │   │
│  │  - Receive build tasks                              │   │
│  │  - Stream logs back to server                       │   │
│  │  - Report health status                             │   │
│  └─────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Build Executor                                     │   │
│  │  - Execute shell commands                           │   │
│  │  - Manage workspace directories                     │   │
│  │  - Capture stdout/stderr                            │   │
│  │  - Collect artifacts                                │   │
│  └─────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Health Monitor                                     │   │
│  │  - Report CPU/memory/disk usage                     │   │
│  │  - Report active build count                        │   │
│  │  - Auto-reconnect to server                         │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Worker Registration Flow

```
1. Worker starts → generates unique worker ID
2. Worker connects to server via gRPC
3. Worker sends registration:
   - worker_id, name, labels, max_concurrent_builds
   - public_key for authentication
4. Server validates token
5. Server adds worker to registry
6. Worker starts heartbeat (every 10s)
7. Server marks worker as "online"
```

### Build Task Assignment

```
1. Build triggered → Scheduler creates task
2. Scheduler checks worker labels (if pipeline specifies)
3. Scheduler finds workers with:
   - matching labels
   - available capacity (current_builds < max_concurrent_builds)
   - healthy status
4. Scheduler assigns task to best worker
5. Worker receives task via gRPC
6. Worker executes build
7. Worker streams logs back to server
8. Worker reports completion/failure
```

### Worker Label Matching

```typescript
// Pipeline can specify required worker labels
pipeline({
  name: "ios-build",
  worker: {
    labels: ["macos", "arm64", "ios"],  // must match
    prefer: ["gpu"],                     // optional preference
  },
  stages: [...],
});
```

### Worker Health Check

```go
type WorkerHealth struct {
    WorkerID        string
    Status          string  // online, offline, busy, disabled
    CPUUsage        float64
    MemoryUsage     float64
    DiskUsage       float64
    ActiveBuilds    int
    MaxBuilds       int
    LastHeartbeat   time.Time
    Uptime          time.Duration
}
```

### CLI Commands for Worker Management

```bash
# Server-side
buildworld233 worker list                    # List all workers
buildworld233 worker status <worker-id>      # Show worker status
buildworld233 worker enable <worker-id>      # Enable worker
buildworld233 worker disable <worker-id>     # Disable worker
buildworld233 worker remove <worker-id>      # Remove worker
buildworld233 worker generate-token          # Generate registration token

# Worker-side (standalone worker binary)
buildworld233-worker start --server <server-url> --token <token>
buildworld233-worker status
buildworld233-worker stop
```

### Worker Binary

```bash
# Build worker binary
go build -o buildworld233-worker ./cmd/worker

# Run worker
./buildworld233-worker \
  --server http://localhost:6050 \
  --token <registration-token> \
  --name "my-worker" \
  --labels "linux,amd64" \
  --max-builds 4
```

### Worker Database Schema

```sql
CREATE TABLE workers (
    id TEXT PRIMARY KEY,  -- UUID
    name TEXT NOT NULL,
    address TEXT NOT NULL,  -- host:port
    token_hash TEXT NOT NULL,
    labels TEXT,  -- JSON array
    max_concurrent_builds INTEGER DEFAULT 4,
    status TEXT DEFAULT 'offline',
    last_heartbeat DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE worker_builds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    worker_id TEXT NOT NULL REFERENCES workers(id),
    build_id INTEGER NOT NULL REFERENCES builds(id),
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME,
    status TEXT DEFAULT 'running'
);
```

### Fault Tolerance

- **Heartbeat timeout**: If no heartbeat for 30s → mark worker offline
- **Build failover**: If worker goes offline mid-build → reassign to another worker
- **Auto-reconnect**: Workers automatically reconnect to server
- **Graceful shutdown**: Worker finishes current builds before stopping

## [S4] Plugin System Design

### Plugin Architecture

```
plugins/
├── github/
│   ├── plugin.json          # Plugin metadata
│   ├── index.ts             # Plugin entry point
│   ├── triggers.ts          # GitHub webhook triggers
│   └── notifications.ts     # GitHub status checks
├── docker/
│   ├── plugin.json
│   ├── index.ts
│   └── steps.ts             # Docker build/push steps
└── slack/
    ├── plugin.json
    ├── index.ts
    └── notifications.ts
```

### Plugin Interface

```typescript
// Every plugin must export this interface
interface Plugin {
  name: string;
  version: string;
  description: string;
  
  // Lifecycle hooks
  onLoad?: () => Promise<void>;
  onUnload?: () => Promise<void>;
  
  // Registers pipeline steps
  steps?: Record<string, StepHandler>;
  
  // Registers triggers
  triggers?: Record<string, TriggerHandler>;
  
  // Registers notifications
  notifications?: Record<string, NotificationHandler>;
}
```

### Hot-Reload Mechanism

1. Plugin files watched by `fsnotify`
2. On change: unload old plugin → recompile TS → load new plugin
3. Zero downtime — running builds continue with old version
4. Plugin registry persists across reloads

---

## [S5] Account & Auth System

### User Model

```go
type User struct {
    ID           int64
    Username     string
    Email        string
    PasswordHash string     // bcrypt
    Role         Role       // admin, maintainer, developer, viewer
    SSHKeys      []SSHKey
    OAuthID      string     // GitHub/GitLab ID
    AvatarURL    string
    CreatedAt    time.Time
    LastLogin    time.Time
}
```

### RBAC Permissions

| Role | Create Project | Trigger Build | View Logs | Manage Users | Manage Plugins |
|------|---------------|---------------|-----------|--------------|----------------|
| Admin | ✅ | ✅ | ✅ | ✅ | ✅ |
| Maintainer | ✅ | ✅ | ✅ | ❌ | ❌ |
| Developer | ❌ | ✅ | ✅ | ❌ | ❌ |
| Viewer | ❌ | ❌ | ✅ | ❌ | ❌ |

### Auth Flow

```
1. Local: username/password → JWT token
2. OAuth: redirect to GitHub/GitLab → callback → JWT token
3. API: Bearer token in header
4. SSH: public key auth for git operations
```

---

## [S6] Git/SVN Integration & SSH

### Git Support

```go
type GitConfig struct {
    URL         string
    Branch      string
    Credentials *Credentials  // SSH key, token, or OAuth
    Depth       int
    Submodules  bool
    Shallow     bool
}
```

**Features:**
- Clone/fetch/push operations
- Webhook receivers for GitHub/GitLab/Bitbucket
- SSH key management per user
- OAuth token management
- Branch/PR/merge request triggers

### SVN Support

```go
type SVNConfig struct {
    URL         string
    Revision    string  // HEAD, specific revision, or branch
    Credentials *Credentials  // username/password or certificate
    Depth       string  // infinity, immediates, files
}
```

**Features:**
- Checkout/update operations
- Username/password auth
- Certificate-based auth
- Revision tracking

### SSH Key Management

```go
type SSHKey struct {
    ID          int64
    UserID      int64
    Name        string
    PublicKey   string
    Fingerprint string
    CreatedAt   time.Time
}
```

- Users can add multiple SSH keys
- Keys used for git clone/push operations
- Keys stored encrypted in SQLite

---

## [S7] Web UI Design

### Dashboard Layout

```
┌─────────────────────────────────────────────────────────────┐
│  buildworld233    [Projects] [Builds] [Plugins] [Settings]  │
├──────────────────┬──────────────────────────────────────────┤
│                  │                                          │
│  Sidebar         │  Main Content Area                      │
│  ─────────       │  ──────────────────                     │
│  ▸ My Projects   │  Build Status: ✅ Success               │
│  ▸ All Projects  │  Duration: 2m 34s                       │
│  ▸ Build History │  Triggered by: push to main             │
│  ▸ Agents        │                                          │
│  ▸ Users         │  Console Output:                         │
│                  │  ──────────────────                     │
│                  │  [Checkout] Clone repository...         │
│                  │  [Build] npm ci...                      │
│                  │  [Build] npm run build...               │
│                  │  [Test] npm test...                     │
│                  │  [Deploy] Uploading to S3...            │
│                  │                                          │
├──────────────────┴──────────────────────────────────────────┤
│  Status: 3 builds running | 12 queued | Last build: 2m ago │
└─────────────────────────────────────────────────────────────┘
```

### Key Pages

1. **Dashboard** — overview of recent builds, system status
2. **Project List** — all projects with search/filter
3. **Project Detail** — config, build history, settings
4. **Build Detail** — console output, artifacts, test results
5. **User Management** — CRUD users, roles, SSH keys
6. **Plugin Manager** — install/enable/disable plugins
7. **System Settings** — global config, notifications, agents
8. **Backup & Restore** — export/import data

### Real-time Features

- WebSocket for live build console output
- Auto-refresh build status
- Push notifications for build completion
- Live agent status monitoring

---

## [S8] Go Project Structure

```
buildworld233/
├── cmd/
│   ├── server/          # Main server binary
│   │   └── main.go
│   ├── worker/          # Worker node binary
│   │   └── main.go
│   └── cli/             # bwctl CLI tool
│       └── main.go
├── internal/
│   ├── engine/          # Build execution engine
│   │   ├── scheduler.go
│   │   ├── executor.go
│   │   ├── workspace.go
│   │   ├── pipeline.go
│   │   └── workerpool.go    # Worker pool management
│   ├── worker/          # Worker node logic
│   │   ├── daemon.go        # Worker daemon
│   │   ├── executor.go      # Build executor
│   │   ├── health.go        # Health monitoring
│   │   └── grpc.go          # gRPC server
│   ├── rpc/             # gRPC definitions
│   │   ├── proto/
│   │   │   └── worker.proto
│   │   └── generated/
│   │       └── worker.pb.go
│   ├── plugin/          # Plugin system
│   │   ├── loader.go
│   │   ├── registry.go
│   │   ├── sandbox.go
│   │   └── hotreload.go
│   ├── auth/            # Authentication & authorization
│   │   ├── jwt.go
│   │   ├── oauth.go
│   │   ├── rbac.go
│   │   └── ssh.go
│   ├── git/             # Git operations
│   │   ├── client.go
│   │   ├── webhook.go
│   │   └── credentials.go
│   ├── svn/             # SVN operations
│   │   ├── client.go
│   │   └── credentials.go
│   ├── api/             # REST API handlers
│   │   ├── router.go
│   │   ├── projects.go
│   │   ├── builds.go
│   │   ├── users.go
│   │   ├── plugins.go
│   │   └── workers.go      # Worker management API
│   ├── ws/              # WebSocket handler
│   │   └── handler.go
│   ├── store/           # Database layer
│   │   ├── sqlite.go
│   │   ├── migrations.go
│   │   └── models.go
│   ├── config/          # Configuration
│   │   └── config.go
│   ├── backup/          # Backup import/export
│   │   ├── export.go
│   │   └── import.go
│   └── update/          # Self-update mechanism
│       └── updater.go
├── pkg/                 # Public packages
│   ├── pipeline/        # Pipeline DSL types
│   │   └── types.go
│   └── types/           # Shared types
│       └── types.go
├── plugins/             # Built-in plugins
│   ├── github/
│   ├── docker/
│   ├── slack/
│   └── npm/
├── web/                 # Frontend (React/Vite)
│   ├── src/
│   │   ├── components/
│   │   ├── pages/
│   │   ├── hooks/
│   │   └── api/
│   ├── package.json
│   └── vite.config.ts
├── docs/                # Documentation site
│   ├── docs/
│   ├── docusaurus.config.js
│   └── package.json
├── scripts/             # Install scripts
│   ├── install.ps1      # Windows installer (server)
│   ├── install.sh       # Linux/macOS installer (server)
│   ├── install-worker.ps1  # Windows worker installer
│   └── install-worker.sh   # Linux/macOS worker installer
├── migrations/          # SQL migrations
│   ├── 001_initial.sql
│   └── ...
├── .github/
│   └── workflows/
│       ├── test.yml     # CI tests
│       └── release.yml  # Release builds
├── proto/               # gRPC proto definitions
│   └── worker.proto
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
├── Dockerfile.worker    # Worker Docker image
└── config.yaml          # Default config
```

---

## [S9] Database Schema (SQLite)

```sql
-- Users & Auth
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer',
    avatar_url TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_login DATETIME
);

CREATE TABLE ssh_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    public_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE oauth_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    provider TEXT NOT NULL,  -- github, gitlab
    provider_id TEXT NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Projects
CREATE TABLE projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    repo_url TEXT NOT NULL,
    repo_type TEXT NOT NULL,  -- git, svn
    default_branch TEXT DEFAULT 'main',
    config TEXT NOT NULL,  -- JSON pipeline config
    created_by INTEGER REFERENCES users(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Builds
CREATE TABLE builds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id),
    number INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    trigger TEXT NOT NULL,  -- manual, webhook, schedule
    branch TEXT,
    commit_sha TEXT,
    started_at DATETIME,
    finished_at DATETIME,
    duration_ms INTEGER,
    log TEXT,
    FOREIGN KEY (project_id) REFERENCES projects(id),
    UNIQUE(project_id, number)
);

-- Artifacts
CREATE TABLE artifacts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    build_id INTEGER NOT NULL REFERENCES builds(id),
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    size INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Plugins
CREATE TABLE plugins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    version TEXT NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    config TEXT,  -- JSON config
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Notifications
CREATE TABLE notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    build_id INTEGER NOT NULL REFERENCES builds(id),
    type TEXT NOT NULL,  -- email, slack, webhook
    status TEXT NOT NULL,  -- pending, sent, failed
    config TEXT NOT NULL,  -- JSON notification config
    sent_at DATETIME
);
```

---

## [S10] Deployment & Distribution

### Binary Distribution

- **Single binary** — Go compiles to a single executable
- **Cross-platform** — Linux, macOS, Windows
- **Embedded frontend** — React build embedded in Go binary via `embed`
- **SQLite embedded** — no external database required

### Configuration

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 6050  # default
  tls: false

database:
  path: "./data/buildworld233.db"

auth:
  jwt_secret: "auto-generated-on-first-run"
  oauth:
    github:
      client_id: ""
      client_secret: ""

plugins:
  path: "./plugins"
  hot_reload: true

storage:
  workspace: "./data/workspaces"
  artifacts: "./data/artifacts"
  logs: "./data/logs"

git:
  ssh_key_path: "./data/ssh"
  known_hosts: "./data/ssh/known_hosts"

# Worker configuration
workers:
  # Local worker (built-in, always available)
  local:
    enabled: true
    max_concurrent_builds: 4
    workspace: "./data/workspaces"
    labels: ["linux", "amd64", "default"]
    
  # Remote worker nodes
  remote:
    - name: "node-01"
      address: "192.168.1.100:7050"
      token: "worker-registration-token"
      labels: ["linux", "amd64", "gpu"]
      max_concurrent_builds: 8
```

### Docker Support

**Server Dockerfile:**
```dockerfile
FROM golang:1.26 AS builder
WORKDIR /app
COPY . .
RUN go build -o buildworld233 ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates git subversion
COPY --from=builder /app/buildworld233 /usr/local/bin/
EXPOSE 6050
CMD ["buildworld233"]
```

**Worker Dockerfile:**
```dockerfile
FROM golang:1.26 AS builder
WORKDIR /app
COPY . .
RUN go build -o buildworld233-worker ./cmd/worker

FROM alpine:latest
RUN apk --no-cache add ca-certificates git subversion
COPY --from=builder /app/buildworld233-worker /usr/local/bin/
EXPOSE 7050
CMD ["buildworld233-worker"]
```

**Docker Compose (server + local worker):**
```yaml
version: '3.8'
services:
  server:
    build: .
    ports:
      - "6050:6050"
    volumes:
      - ./data:/app/data
      - ./config.yaml:/app/config.yaml
    command: buildworld233 start
    
  worker:
    build:
      context: .
      dockerfile: Dockerfile.worker
    environment:
      - SERVER_URL=http://server:6050
      - WORKER_TOKEN=${WORKER_TOKEN}
    volumes:
      - ./data/workspaces:/app/workspaces
    command: buildworld233-worker start --server http://server:6050 --token ${WORKER_TOKEN}
```

### GitHub Actions Release

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
      - name: Upload to Release
        uses: softprops/action-gh-release@v1
        with:
          files: buildworld233-*
```

---

## [S11] Install Scripts

### Server - Windows (`scripts/install.ps1`)

```powershell
# buildworld233 server installer (Windows PowerShell)
# iwr -useb https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.ps1 | iex
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
    Write-Host "Add to PATH: $installDir"
    Write-Host "Run: buildworld233 start"
    Write-Host "Status: buildworld233 status"
    Write-Host "Enable boot autostart: buildworld233 enable-autostart"
    Write-Host "Change port: buildworld233 set-port 6050"
    Write-Host "Self update: buildworld233 update"
    Write-Host "Hot reload config: buildworld233 reload-config"
    Write-Host "Export backup: buildworld233 backup export --output ./backup.zip"
    Write-Host "Import backup: buildworld233 backup import --input ./backup.zip"
    Write-Host "Generate worker token: buildworld233 worker generate-token"
    Write-Host "List workers: buildworld233 worker list"
}

if ($Version -eq "latest") { $Version = Get-LatestVersion }
$Version = $Version -replace '^[vV]', ''
Write-Host "Installing buildworld233 v$VERSION ..."
Install-BuildWorld233 -Ver $Version
```

### Server - Linux/macOS (`scripts/install.sh`)

```bash
#!/bin/bash
# buildworld233 server installer (Linux/macOS)
# curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.sh | bash
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
    echo "Status: buildworld233 status"
    echo "Enable boot autostart: buildworld233 enable-autostart"
    echo "Change port: buildworld233 set-port 6050"
    echo "Self update: buildworld233 update"
    echo "Hot reload config: buildworld233 reload-config"
    echo "Export backup: buildworld233 backup export --output ./backup.zip"
    echo "Import backup: buildworld233 backup import --input ./backup.zip"
    echo "Generate worker token: buildworld233 worker generate-token"
    echo "List workers: buildworld233 worker list"
}

if [ "$VERSION" = "latest" ]; then
    VERSION=$(get_latest_version)
fi
VERSION="${VERSION#v}"
echo "Installing buildworld233 v$VERSION ..."
install "$VERSION"
```

### Worker - Windows (`scripts/install-worker.ps1`)

```powershell
# buildworld233 worker installer (Windows PowerShell)
# iwr -useb https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install-worker.ps1 | iex
param(
    [string]$Version = "latest",
    [string]$Server = "http://localhost:6050",
    [string]$Token = ""
)
$ErrorActionPreference = "Stop"
$BinaryName = "buildworld233-worker"
$Repo = "neko233-com/buildworld233"

function Get-LatestVersion {
    try {
        $r = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
        return ($r.tag_name -replace '^[vV]', '')
    } catch {
        return "0.1.0"
    }
}

function Install-BuildWorld233Worker {
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
    Write-Host "Run worker: buildworld233-worker start --server $Server --token $Token"
    Write-Host "Status: buildworld233-worker status"
    Write-Host "Stop: buildworld233-worker stop"
}

if ($Version -eq "latest") { $Version = Get-LatestVersion }
$Version = $Version -replace '^[vV]', ''
Write-Host "Installing buildworld233-worker v$Version ..."
Install-BuildWorld233Worker -Ver $Version

if ($Token -ne "") {
    Write-Host "Starting worker..."
    Start-Process -FilePath (Join-Path $env:LOCALAPPDATA "buildworld233\$BinaryName.exe") -ArgumentList "start", "--server", $Server, "--token", $Token
}
```

### Worker - Linux/macOS (`scripts/install-worker.sh`)

```bash
#!/bin/bash
# buildworld233 worker installer (Linux/macOS)
# curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install-worker.sh | bash -s -- --server http://localhost:6050 --token <token>
set -e

REPO="neko233-com/buildworld233"
BINARY="buildworld233-worker"
VERSION="latest"
SERVER="http://localhost:6050"
TOKEN=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --server) SERVER="$2"; shift 2 ;;
        --token) TOKEN="$2"; shift 2 ;;
        --version) VERSION="$2"; shift 2 ;;
        *) shift ;;
    esac
done

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
    echo "Run worker: buildworld233-worker start --server $SERVER --token $TOKEN"
    echo "Status: buildworld233-worker status"
    echo "Stop: buildworld233-worker stop"
}

if [ "$VERSION" = "latest" ]; then
    VERSION=$(get_latest_version)
fi
VERSION="${VERSION#v}"
echo "Installing buildworld233-worker v$VERSION ..."
install "$VERSION"

if [ -n "$TOKEN" ]; then
    echo "Starting worker..."
    buildworld233-worker start --server "$SERVER" --token "$TOKEN" &
fi
```

### Linux/macOS (`scripts/install.sh`)

```bash
#!/bin/bash
# buildworld233 installer (Linux/macOS)
# curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.sh | bash
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
    echo "Status: buildworld233 status"
    echo "Enable boot autostart: buildworld233 enable-autostart"
    echo "Change port: buildworld233 set-port 6050"
    echo "Self update: buildworld233 update"
    echo "Hot reload config: buildworld233 reload-config"
    echo "Export backup: buildworld233 backup export --output ./backup.zip"
    echo "Import backup: buildworld233 backup import --input ./backup.zip"
}

if [ "$VERSION" = "latest" ]; then
    VERSION=$(get_latest_version)
fi
VERSION="${VERSION#v}"
echo "Installing buildworld233 v$VERSION ..."
install "$VERSION"
```

---

## [S12] CLI Commands

```bash
buildworld233 start              # Start server
buildworld233 status             # Show status
buildworld233 stop               # Stop server
buildworld233 restart            # Restart server
buildworld233 enable-autostart   # Enable boot autostart
buildworld233 disable-autostart  # Disable boot autostart
buildworld233 set-port 6050      # Change port
buildworld233 reload-config      # Hot-reload YAML config
buildworld233 update             # Self-update from GitHub releases
buildworld233 reset-admin-password --password <NEW>
buildworld233 backup export --output ./backup.zip
buildworld233 backup import --input ./backup.zip
buildworld233 version            # Show version
buildworld233 help               # Show help
```

---

## [S13] YAML Config Hot-Reload

### Mechanism

1. `fsnotify` watches `config.yaml`
2. On change: validate → apply changes → log reload
3. Some settings require restart (port, TLS)
4. Most settings apply immediately (auth, plugins, storage paths)

### Reloadable Settings

| Setting | Requires Restart |
|---------|------------------|
| server.host | Yes |
| server.port | Yes |
| server.tls | Yes |
| database.path | Yes |
| auth.jwt_secret | No |
| auth.oauth.* | No |
| plugins.* | No |
| storage.* | No |
| git.* | No |

---

## [S14] Web UI One-Click Update

### Update Flow

1. Check GitHub releases API for latest version
2. Compare with current version
3. Download new binary to temp location
4. Replace current binary (atomic rename)
5. Restart server
6. Verify new version running

### UI

```
Settings → System → Update
┌─────────────────────────────────────┐
│  Current Version: v1.0.0            │
│  Latest Version: v1.1.0             │
│                                     │
│  [Check for Updates]                │
│  [Update Now]                       │
│                                     │
│  Auto-update: [Enabled]             │
│  Update channel: [Stable]           │
└─────────────────────────────────────┘
```

---

## [S15] Data Backup Import/Export

### Export

```bash
buildworld233 backup export --output ./backup-2024-01-15.zip
```

Contents:
```
backup-2024-01-15.zip
├── database/
│   └── buildworld233.db        # SQLite dump
├── config/
│   └── config.yaml             # Current config
├── plugins/
│   └── plugin-configs.json     # Plugin settings
├── users/
│   └── ssh-keys/               # User SSH public keys
└── metadata.json               # Backup info, version, timestamp
```

### Import

```bash
buildworld233 backup import --input ./backup-2024-01-15.zip
```

### Web UI

```
Settings → System → Backup & Restore
┌─────────────────────────────────────┐
│  Export Backup                      │
│  [Export to File]                   │
│                                     │
│  Import Backup                      │
│  [Choose File] [Import]             │
│                                     │
│  Auto-backup: [Daily] [Weekly]      │
│  Backup location: ./data/backups/   │
└─────────────────────────────────────┘
```

---

## [S16] Testing Strategy

### Unit Tests

- Go standard `testing` package + `testify` for assertions
- Mock interfaces for external dependencies (git, notification)
- In-memory SQLite for database tests

### Integration Tests

- Full API tests with `httptest`
- WebSocket tests for build log streaming
- Git/SVN client tests against local test repos

### E2E Tests

- Playwright for web UI testing
- Full build pipeline execution tests
- Plugin hot-reload tests

### CLI Tests

- Test all CLI commands
- Verify install scripts work

### Test Coverage Target

- Core engine: 80%+
- Auth: 90%+
- API handlers: 70%+
- Web UI: Critical paths only

### CI/CD

```yaml
# .github/workflows/test.yml
name: Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - run: go test ./...
      - run: go test -tags=integration ./...
      - name: E2E Tests
        run: |
          go build -o buildworld233 ./cmd/server
          ./buildworld233 start &
          npx playwright test
```

---

## [S17] Implementation Phases

### Phase 1: Foundation (Week 1-2)
- [ ] Go project setup with go.mod
- [ ] SQLite database layer with migrations
- [ ] Configuration system (YAML)
- [ ] CLI framework (cobra)
- [ ] Basic HTTP server (chi)

### Phase 2: Core Engine (Week 3-4)
- [ ] Build scheduler
- [ ] Pipeline executor
- [ ] Workspace manager
- [ ] Shell execution engine
- [ ] Build log streaming (WebSocket)

### Phase 3: Worker System (Week 5-6)
- [ ] gRPC proto definitions
- [ ] Worker daemon (buildworld233-worker)
- [ ] Worker registration & heartbeat
- [ ] Build task assignment
- [ ] Log streaming from worker to server
- [ ] Worker health monitoring
- [ ] Failover & auto-reconnect

### Phase 4: Plugin System (Week 7-8)
- [ ] Plugin loader (goja)
- [ ] Plugin registry
- [ ] Hot-reload mechanism
- [ ] Built-in plugins (GitHub, Docker, Slack)

### Phase 5: Auth & VCS (Week 9-10)
- [ ] User management (CRUD)
- [ ] JWT authentication
- [ ] OAuth (GitHub/GitLab)
- [ ] RBAC permissions
- [ ] Git client
- [ ] SVN client
- [ ] SSH key management

### Phase 6: Web UI (Week 11-14)
- [ ] React/Vite setup
- [ ] Dashboard
- [ ] Project management
- [ ] Build history & logs
- [ ] User management
- [ ] Plugin manager
- [ ] Worker management UI
- [ ] System settings
- [ ] Backup & restore UI

### Phase 7: Polish & Release (Week 15-16)
- [ ] Install scripts (.ps1, .sh) for server & worker
- [ ] Self-update mechanism
- [ ] Docker support (server + worker)
- [ ] Documentation site
- [ ] Automated testing
- [ ] GitHub Actions CI/CD
- [ ] v1.0.0 release

---

## [S18] Success Criteria

1. **Single binary** — No external dependencies required
2. **Zero-config startup** — Works out of the box
3. **Fast startup** — < 1 second to ready
4. **TypeScript pipelines** — Full TS support via goja
5. **Hot-reload plugins** — Zero downtime updates
6. **Modern UI** — React dashboard with real-time updates
7. **Complete backup** — Export/import all data
8. **One-click install** — .ps1 and .sh scripts for server & worker
9. **Self-updating** — CLI and UI update mechanisms
10. **80%+ test coverage** — Core engine and auth
11. **Distributed workers** — Local + remote worker nodes
12. **Worker label matching** — Route builds to specific workers
13. **Worker failover** — Automatic reassignment on failure
14. **Worker health monitoring** — Real-time status in UI

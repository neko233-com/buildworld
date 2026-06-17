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

## [S3.5] Worker Node System (Agent-First Architecture)

### Overview

buildworld233 uses an **agent-first architecture** where workers (agents) are the primary execution units:

- **Server** = Coordinator + API + UI + Storage (the brain)
- **Agent** = Build executor (the muscle) - this is the primary design concept
- **Local agent** = Built-in, starts automatically with server
- **Remote agents** = Connect via gRPC, can be added on-demand
- **Agent pool** = Dynamic scaling of agents based on workload

**Agent-First Principles:**
1. Agents are self-contained and autonomous
2. Agents can operate independently if server is temporarily unavailable
3. Agents report status proactively, not just on request
4. Agent discovery is automatic via registration
5. Agent health is continuously monitored

### Agent Configuration

```yaml
# config.yaml
agents:
  # Local agent (built-in, always available)
  local:
    enabled: true
    max_concurrent_builds: 4
    workspace: "./data/workspaces"
    labels: ["linux", "amd64", "default"]
    
  # Remote agent registration
  remote:
    - name: "agent-01"
      address: "192.168.1.100:7050"
      token: "agent-registration-token"
      labels: ["linux", "amd64", "gpu"]
      max_concurrent_builds: 8
      
    - name: "agent-02"
      address: "192.168.1.101:7050"
      token: "agent-registration-token"
      labels: ["macos", "arm64", "ios"]
      max_concurrent_builds: 4
```

### Agent Node Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Agent Node (buildworld233-agent)         │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Agent Controller                                    │   │
│  │  - Self-registration with server                     │   │
│  │  - Heartbeat management                              │   │
│  │  - Task queue management                             │   │
│  │  - Graceful shutdown                                 │   │
│  └─────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Build Executor                                     │   │
│  │  - Execute shell commands                           │   │
│  │  - Manage workspace directories                     │   │
│  │  - Capture stdout/stderr with rolling buffer        │   │
│  │  - Collect artifacts                                │   │
│  │  - Environment variable resolution                  │   │
│  └─────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Health Monitor                                     │   │
│  │  - CPU/memory/disk metrics                          │   │
│  │  - Active build tracking                            │   │
│  │  - Auto-reconnect to server                         │   │
│  │  - Offline queue for failed connections             │   │
│  └─────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Credential Vault                                   │   │
│  │  - Secure SSH key storage (encrypted at rest)       │   │
│  │  - SSH password authentication support              │   │
│  │  - Token-based auth for git servers                 │   │
│  │  - Environment variable secrets                     │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Agent Registration Flow

```
1. Agent starts → generates unique agent ID
2. Agent connects to server via gRPC
3. Agent sends registration:
   - agent_id, name, labels, max_concurrent_builds
   - public_key for authentication
4. Server validates token
5. Server adds agent to registry
6. Agent starts heartbeat (every 10s)
7. Server marks agent as "online"
8. Agent enters ready state, waiting for tasks
```

### Secure Credential Storage

```go
type CredentialVault struct {
    mu       sync.RWMutex
    store    *store.Store
    cipher   *Cipher
}

type StoredCredential struct {
    ID        int64     `json:"id"`
    Type      string    `json:"type"`  // ssh_key, ssh_password, token, oauth
    Name      string    `json:"name"`
    Encrypted []byte    `json:"encrypted"`  // AES-256-GCM encrypted
    Salt      []byte    `json:"salt"`       // For key derivation
    IV        []byte    `json:"iv"`         // Initialization vector
    CreatedAt time.Time `json:"created_at"`
    ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// SSH Password Storage
type SSHPasswordCredential struct {
    Username string `json:"username"`
    Password string `json:"password"`  // Encrypted
    Host     string `json:"host"`
    Port     int    `json:"port"`
}

// Encryption at rest using AES-256-GCM
func (v *CredentialVault) Encrypt(plaintext []byte) ([]byte, error) {
    // Generate random salt and IV
    // Derive key using PBKDF2
    // Encrypt using AES-256-GCM
    // Return encrypted data with salt and IV
}

func (v *CredentialVault) Decrypt(ciphertext []byte) ([]byte, error) {
    // Extract salt and IV
    // Derive key using PBKDF2
    // Decrypt using AES-256-GCM
    // Return plaintext
}
```

### SSH Authentication Methods

```go
type SSHAuthMethod struct {
    Type       string `json:"type"`
    // Key-based
    PrivateKey  string `json:"private_key,omitempty"`
    Passphrase  string `json:"passphrase,omitempty"`  // For encrypted keys
    // Password-based
    Username    string `json:"username,omitempty"`
    Password    string `json:"password,omitempty"`
    // Connection
    Host        string `json:"host"`
    Port        int    `json:"port"`
}

// Supported SSH auth methods:
// 1. Public key authentication (SSH key pair)
// 2. Password authentication (username + password)
// 3. Keyboard-interactive authentication
// 4. Certificate authentication (future)
```

### Agent Task Execution

```
1. Server assigns task to agent
2. Agent receives task via gRPC
3. Agent validates task permissions
4. Agent resolves environment variables:
   - ${global.VAR} → from server global env
   - ${project.VAR} → from project env
   - ${secret.VAR} → from credential vault
5. Agent clones repository (using stored credentials)
6. Agent executes build steps sequentially
7. Agent streams logs to server (rolling buffer)
8. Agent reports completion/failure
9. Agent cleans up workspace
```

### Agent Health Check

```go
type AgentHealth struct {
    AgentID       string
    Status        string  // online, offline, busy, disabled
    CPUUsage      float64
    MemoryUsage   float64
    DiskUsage     float64
    ActiveBuilds  int
    MaxBuilds     int
    LastHeartbeat time.Time
    Uptime        time.Duration
    Version       string
    OS            string
    Arch          string
}
```

### CLI Commands for Agent Management

```bash
# Server-side
buildworld233 agent list                    # List all agents
buildworld233 agent status <agent-id>       # Show agent status
buildworld233 agent enable <agent-id>       # Enable agent
buildworld233 agent disable <agent-id>      # Disable agent
buildworld233 agent remove <agent-id>       # Remove agent
buildworld233 agent generate-token          # Generate registration token

# Agent-side (standalone agent binary)
buildworld233-agent start --server <server-url> --token <token>
buildworld233-agent status
buildworld233-agent stop
```

### Agent Binary

```bash
# Build agent binary
go build -o buildworld233-agent ./cmd/agent

# Run agent
./buildworld233-agent \
  --server http://localhost:6050 \
  --token <registration-token> \
  --name "my-agent" \
  --labels "linux,amd64" \
  --max-builds 4
```

### Fault Tolerance

- **Heartbeat timeout**: If no heartbeat for 30s → mark agent offline
- **Build failover**: If agent goes offline mid-build → reassign to another agent
- **Auto-reconnect**: Agents automatically reconnect to server
- **Graceful shutdown**: Agent finishes current builds before stopping
- **Offline queue**: Tasks queued locally if server unreachable

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

### Supported Git Servers

buildworld233 supports all major Git hosting platforms:

| Platform | Webhook | OAuth | API | Status |
|----------|---------|-------|-----|--------|
| GitHub | ✅ | ✅ | ✅ | Built-in |
| GitLab | ✅ | ✅ | ✅ | Built-in |
| Gitee | ✅ | ✅ | ✅ | Built-in |
| Gitea | ✅ | ✅ | ✅ | Built-in |
| Custom | ✅ | ❌ | ❌ | URL-based |

```go
type GitServer struct {
    ID          int64     `json:"id"`
    Name        string    `json:"name"`
    Type        string    `json:"type"`  // github, gitlab, gitee, gitea, custom
    URL         string    `json:"url"`   // For custom servers
    APIURL      string    `json:"api_url,omitempty"`
    WebhookURL  string    `json:"webhook_url,omitempty"`
    OAuthID     string    `json:"oauth_id,omitempty"`
    OAuthSecret string    `json:"oauth_secret,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
}

type GitCredentials struct {
    ID        int64     `json:"id"`
    ServerID  int64     `json:"server_id"`
    UserID    int64     `json:"user_id"`
    Type      string    `json:"type"`  // token, ssh_key, ssh_password, oauth
    Username  string    `json:"username,omitempty"`
    Token     string    `json:"token,omitempty"`      // PAT or OAuth token
    SSHKey    string    `json:"ssh_key,omitempty"`    // Private key content
    Password  string    `json:"password,omitempty"`   // SSH password
    CreatedAt time.Time `json:"created_at"`
}
```

### SSH Support

SSH supports both key-based and password-based authentication:

```go
type SSHConfig struct {
    // Key-based auth
    PrivateKey     string `json:"private_key,omitempty"`
    PrivateKeyFile string `json:"private_key_file,omitempty"`
    
    // Password-based auth (for servers that support it)
    Username       string `json:"username,omitempty"`
    Password       string `json:"password,omitempty"`
    
    // Known hosts
    KnownHostsFile string `json:"known_hosts_file,omitempty"`
    StrictHostKey  bool   `json:"strict_host_key"`
}
```

**Features:**
- Clone/fetch/push operations
- Webhook receivers for all supported platforms
- SSH key management per user
- SSH password authentication
- OAuth token management
- Branch/PR/merge request triggers
- Custom git server URL recording

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

---

## [S6.5] Environment Variables & Parameterized Builds

### Environment Variable System

buildworld233 supports hierarchical environment variables with secure field types:

```go
type EnvVar struct {
    ID          int64  `json:"id"`
    Scope       string `json:"scope"`       // global, project
    Name        string `json:"name"`
    Value       string `json:"value"`
    IsSecret    bool   `json:"is_secret"`   // Hidden in UI after config
    Description string `json:"description,omitempty"`
}

// Variable syntax:
// ${global.FIELD_NAME}   - Global environment variable
// ${project.FIELD_NAME}  - Project-level environment variable
```

**Variable Resolution:**
1. Project variables override global variables
2. Secret fields are masked in logs and UI
3. Variables available in all pipeline steps

**Example:**
```yaml
# Global variables (Settings → Environment)
GLOBAL_TOKEN: ${global.TOKEN}          # Secret, hidden after config
GLOBAL_REGISTRY: ${global.REGISTRY}    # Visible

# Project variables (Project → Settings)
PROJECT_API_KEY: ${project.API_KEY}    # Secret
PROJECT_DEPLOY_PATH: ${project.PATH}   # Visible
```

### Parameterized Builds

Like Jenkins, builds can accept parameters for dynamic configuration:

```go
type BuildParameter struct {
    Name         string      `json:"name"`
    Type         string      `json:"type"`  // string, choice, boolean, password, text
    Description  string      `json:"description"`
    Default      interface{} `json:"default"`
    Required     bool        `json:"required"`
    Choices      []string    `json:"choices,omitempty"`  // For choice type
    IsSecret     bool        `json:"is_secret"`          // For password type
}

type PipelineConfig struct {
    // ... other fields
    Parameters []BuildParameter `json:"parameters"`
}
```

**Parameter Types:**
| Type | UI Widget | Example |
|------|-----------|---------|
| string | Text input | `branch: main` |
| choice | Dropdown | `environment: [dev, staging, prod]` |
| boolean | Checkbox | `deploy: true` |
| password | Password input (masked) | `api_key: ***` |
| text | Textarea | `changelog: Multi-line text` |

**Example Pipeline with Parameters:**
```typescript
pipeline({
  name: "deploy-app",
  parameters: [
    {
      name: "environment",
      type: "choice",
      description: "Target environment",
      choices: ["development", "staging", "production"],
      default: "staging",
      required: true,
    },
    {
      name: "version",
      type: "string",
      description: "Version to deploy",
      default: "latest",
    },
    {
      name: "api_key",
      type: "password",
      description: "API key for deployment",
      required: true,
      is_secret: true,
    },
    {
      name: "skip_tests",
      type: "boolean",
      description: "Skip test execution",
      default: false,
    },
  ],
  stages: [
    {
      name: "Deploy",
      steps: [
        shell("deploy.sh --env ${parameter.environment} --version ${parameter.version}"),
      ],
    },
  ],
});
```

**Parameter UI (Feishu-style):**
```
Build: deploy-app
┌─────────────────────────────────────────────────────────────┐
│  Parameters                                                 │
├─────────────────────────────────────────────────────────────┤
│  Environment *                                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ ▼ staging                                            │   │
│  │   development                                        │   │
│  │   staging                                            │   │
│  │   production                                         │   │
│  └─────────────────────────────────────────────────────┘   │
│  Target environment                                         │
│                                                             │
│  Version                                                    │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ latest                                               │   │
│  └─────────────────────────────────────────────────────┘   │
│  Version to deploy                                          │
│                                                             │
│  API Key *                                                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ ••••••••••••                                         │   │
│  └─────────────────────────────────────────────────────┘   │
│  API key for deployment (hidden after save)                 │
│                                                             │
│  ☐ Skip Tests                                               │
│  Skip test execution                                        │
│                                                             │
│  [Cancel]  [Build]                                          │
└─────────────────────────────────────────────────────────────┘
```

### Build Now = Execute Packaging

**Important clarification:** In buildworld233, "Build Now" means "Execute Packaging" (执行打包). This is the same as Jenkins' "Build Now" - it triggers a build execution with the current configuration and parameters.

---

## [S6.6] Built-in Development Environments

### Auto-installed Environments

buildworld233 can automatically install and configure development environments:

```go
type DevEnvironment struct {
    Name        string   `json:"name"`
    Version     string   `json:"version"`
    InstallCmd  string   `json:"install_cmd"`
    BinaryPath  string   `json:"binary_path"`
    EnvVars     []string `json:"env_vars"`
    AutoInstall bool     `json:"auto_install"`
}

var DefaultEnvironments = []DevEnvironment{
    {
        Name:        "JDK",
        Version:     "21",
        InstallCmd:  "https://adoptium.net/temurin/releases/",
        BinaryPath:  "/usr/lib/jvm/java-21",
        EnvVars:     ["JAVA_HOME=/usr/lib/jvm/java-21", "PATH=$JAVA_HOME/bin:$PATH"],
        AutoInstall: true,
    },
    {
        Name:        "Maven",
        Version:     "3.9.6",
        InstallCmd:  "https://maven.apache.org/download.cgi",
        BinaryPath:  "/opt/maven",
        EnvVars:     ["MAVEN_HOME=/opt/maven", "PATH=$MAVEN_HOME/bin:$PATH"],
        AutoInstall: true,
    },
    {
        Name:        "Gradle",
        Version:     "8.5",
        InstallCmd:  "https://gradle.org/releases/",
        BinaryPath:  "/opt/gradle",
        EnvVars:     ["GRADLE_HOME=/opt/gradle", "PATH=$GRADLE_HOME/bin:$PATH"],
        AutoInstall: true,
    },
    {
        Name:        "Node.js",
        Version:     "24",
        InstallCmd:  "https://nodejs.org/en/download/",
        BinaryPath:  "/usr/local/node",
        EnvVars:     ["NODE_HOME=/usr/local/node", "PATH=$NODE_HOME/bin:$PATH"],
        AutoInstall: true,
    },
    {
        Name:        "npm",
        Version:     "latest",
        InstallCmd:  "Bundled with Node.js",
        BinaryPath:  "/usr/local/node/bin/npm",
        EnvVars:     [],
        AutoInstall: true,
    },
}
```

### One-Click Environment Setup

```
Settings → Environments → Setup Wizard
┌─────────────────────────────────────────────────────────────┐
│  Development Environment Setup                              │
├─────────────────────────────────────────────────────────────┤
│  Detected: Windows 11, x64                                  │
│                                                             │
│  Available Environments:                                     │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ ☑ JDK 21 (Adoptium)           [Auto-detect source] │   │
│  │ ☑ Maven 3.9.6                  [Auto-detect source] │   │
│  │ ☑ Gradle 8.5                   [Auto-detect source] │   │
│  │ ☑ Node.js 24 LTS               [Auto-detect source] │   │
│  │ ☑ npm (bundled)                [Auto-detect source] │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  Download Source: [Auto-detect ▼]                           │
│  - Auto (recommended)                                       │
│  - China Mirror (npmmirror.com)                             │
│  - International (official)                                 │
│                                                             │
│  Install Location: [C:\\buildworld233\\env]                │
│                                                             │
│  [Install Selected]                                         │
└─────────────────────────────────────────────────────────────┘
```

### Auto-detect Download Source

```go
type DownloadSource struct {
    Name       string
    URL        string
    Region     string  // china, international
    AutoDetect bool
}

var DownloadSources = map[string][]DownloadSource{
    "jdk": {
        {Name: "Adoptium (International)", URL: "https://api.adoptium.net/v3/binary/latest/21/ga/windows/x64/jdk/hotspot/normal/eclipse", Region: "international"},
        {Name: "Adoptium (China Mirror)", URL: "https://mirrors.tuna.tsinghua.edu.cn/Adoptium/21/jdk/x64/windows/", Region: "china"},
    },
    "maven": {
        {Name: "Apache (International)", URL: "https://dlcdn.apache.org/maven/maven-3/3.9.6/binaries/apache-maven-3.9.6-bin.zip", Region: "international"},
        {Name: "Aliyun Mirror", URL: "https://mirrors.aliyun.com/apache/maven/maven-3/3.9.6/binaries/apache-maven-3.9.6-bin.zip", Region: "china"},
    },
    "node": {
        {Name: "Node.js (International)", URL: "https://nodejs.org/dist/v24.0.0/node-v24.0.0-x64.msi", Region: "international"},
        {Name: "Node.js (China Mirror)", URL: "https://npmmirror.com/mirrors/node/v24.0.0/node-v24.0.0-x64.msi", Region: "china"},
    },
    "gradle": {
        {Name: "Gradle (International)", URL: "https://services.gradle.org/distributions/gradle-8.5-bin.zip", Region: "international"},
        {Name: "TUNA Mirror", URL: "https://mirrors.tuna.tsinghua.edu.cn/gradle/gradle-8.5-bin.zip", Region: "china"},
    },
}

func DetectRegion() string {
    // Check timezone or try to access a known China-only endpoint
    // Return "china" or "international"
}
```

---

## [S6.7] Security & HTTPS

### Security Features

- **HTTPS/TLS** — Built-in TLS support
- **CORS** — Configurable CORS policies
- **Rate Limiting** — API rate limiting
- **CSRF Protection** — Cross-site request forgery protection
- **Input Validation** — All inputs sanitized
- **SQL Injection Prevention** — Parameterized queries
- **XSS Protection** — Content Security Policy headers

### proxysss Integration

buildworld233 integrates with neko233-com/proxysss for automatic HTTPS:

```yaml
# config.yaml
security:
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
  
  # proxysss integration for automatic HTTPS
  proxysss:
    enabled: true
    domain: "build.example.com"
    email: "admin@example.com"
    auto_renew: true
```

**One-click HTTPS Setup:**
```
Settings → Security → HTTPS Setup
┌─────────────────────────────────────────────────────────────┐
│  Automatic HTTPS with proxysss                              │
├─────────────────────────────────────────────────────────────┤
│  Domain: [build.example.com        ]                       │
│  Email:  [admin@example.com        ]                       │
│                                                             │
│  [Setup HTTPS]                                              │
│                                                             │
│  Status: ✅ Certificate issued, HTTPS enabled               │
│  Expires: 2026-09-15 (auto-renew enabled)                   │
└─────────────────────────────────────────────────────────────┘
```

---

## [S6.8] Multi-language Support (i18n)

### Supported Languages

- English (en)
- Chinese Simplified (zh-CN)
- Chinese Traditional (zh-TW) — future
- Japanese (ja) — future
- Korean (ko) — future

### Implementation

```typescript
// web/src/i18n/locales/en.json
{
  "dashboard": {
    "title": "Dashboard",
    "recentBuilds": "Recent Builds",
    "systemStatus": "System Status"
  },
  "projects": {
    "title": "Projects",
    "newProject": "New Project",
    "fromTemplate": "From Template"
  },
  "builds": {
    "title": "Builds",
    "buildNow": "Build Now",
    "buildHistory": "Build History"
  }
}

// web/src/i18n/locales/zh-CN.json
{
  "dashboard": {
    "title": "仪表盘",
    "recentBuilds": "最近构建",
    "systemStatus": "系统状态"
  },
  "projects": {
    "title": "项目",
    "newProject": "新建项目",
    "fromTemplate": "从模板创建"
  },
  "builds": {
    "title": "构建",
    "buildNow": "立即构建",
    "buildHistory": "构建历史"
  }
}
```

**Language Selector:**
```
Settings → General → Language
┌─────────────────────────────────────────────────────────────┐
│  Language                                                   │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ ▼ English                                            │   │
│  │   English                                            │   │
│  │   简体中文 (Chinese Simplified)                       │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

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
│  ▸ Users         │  Console Output (rolling):               │
│                  │  ──────────────────                     │
│                  │  [21:46:28] [Checkout] Clone...         │
│                  │  [21:46:30] [Build] npm ci...           │
│                  │  [21:46:45] [Build] npm run build...    │
│                  │  [21:47:12] [Test] npm test...          │
│                  │  [21:47:30] [Deploy] Uploading...       │
│                  │  ▼ Auto-scroll: ON                      │
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
9. **Real-time Monitor** — live build tracking, server status

### Rolling Logs (Jenkins-style)

```go
type RollingLog struct {
    BuildID   int64
    Lines     []LogLine
    MaxLines  int  // Default 10000, configurable
    Truncated bool
}

type LogLine struct {
    Timestamp time.Time
    Level     string  // info, warn, error
    Stage     string  // Checkout, Build, Test, Deploy
    Step      string
    Message   string
    IsError   bool
}
```

**Features:**
- Rolling buffer (configurable max lines, default 10000)
- Auto-scroll with toggle
- Search/filter within logs
- Download logs as text file
- Color-coded by level (info=gray, warn=yellow, error=red)
- Stage/step collapsible sections

### Real-time Build Tracking

```go
type BuildStatus struct {
    BuildID     int64     `json:"build_id"`
    ProjectID   int64     `json:"project_id"`
    Number      int       `json:"number"`
    Status      string    `json:"status"`  // pending, running, success, failed
    Stage       string    `json:"current_stage"`
    Step        string    `json:"current_step"`
    Progress    float64   `json:"progress"`  // 0.0 - 1.0
    StartedAt   time.Time `json:"started_at"`
    Duration    string    `json:"duration"`
    WorkerID    string    `json:"worker_id"`
    WorkerName  string    `json:"worker_name"`
}

type ServerStatus struct {
    Uptime          time.Duration `json:"uptime"`
    ActiveBuilds    int           `json:"active_builds"`
    QueuedBuilds    int           `json:"queued_builds"`
    TotalBuilds     int           `json:"total_builds"`
    Workers         []WorkerStatus `json:"workers"`
    CPUUsage        float64       `json:"cpu_usage"`
    MemoryUsage     float64       `json:"memory_usage"`
    DiskUsage       float64       `json:"disk_usage"`
}

type WorkerStatus struct {
    ID              string    `json:"id"`
    Name            string    `json:"name"`
    Status          string    `json:"status"`
    ActiveBuilds    int       `json:"active_builds"`
    MaxBuilds       int       `json:"max_builds"`
    CPU             float64   `json:"cpu"`
    Memory          float64   `json:"memory"`
    LastHeartbeat   time.Time `json:"last_heartbeat"`
}
```

### WebSocket Events

```typescript
// Client subscribes to build updates
ws.send(JSON.stringify({
  type: "subscribe",
  channel: "build:123"  // Build ID
}));

// Server sends real-time updates
ws.send(JSON.stringify({
  type: "build:status",
  data: {
    build_id: 123,
    status: "running",
    stage: "Build",
    step: "npm run build",
    progress: 0.45,
    log_line: "[21:46:45] Building for production..."
  }
}));

// Server sends log lines (rolling)
ws.send(JSON.stringify({
  type: "build:log",
  data: {
    build_id: 123,
    line: {
      timestamp: "2026-06-17T21:46:45Z",
      level: "info",
      stage: "Build",
      message: "Build completed successfully"
    }
  }
}));
```

### Real-time Monitor UI

```
Settings → Monitor
┌─────────────────────────────────────────────────────────────┐
│  Server Status                                              │
├─────────────────────────────────────────────────────────────┤
│  Uptime: 2h 34m 56s                                        │
│  CPU: 23%  |  Memory: 45%  |  Disk: 67%                    │
│  Active Builds: 3  |  Queued: 2  |  Total: 1,234          │
├─────────────────────────────────────────────────────────────┤
│  Workers                                                    │
├─────────────────────────────────────────────────────────────┤
│  🟢 worker-01 (192.168.1.100)  |  2/8 builds  |  CPU: 34% │
│  🟢 worker-02 (192.168.1.101)  |  1/4 builds  |  CPU: 12% │
│  🟡 worker-03 (192.168.1.102)  |  0/4 builds  |  Offline   │
├─────────────────────────────────────────────────────────────┤
│  Live Builds                                                │
├─────────────────────────────────────────────────────────────┤
│  #123 my-app        |  Build  |  45%  |  1m 23s           │
│  #456 api-server    |  Test   |  78%  |  3m 45s           │
│  #789 website       |  Deploy |  92%  |  5m 12s           │
└─────────────────────────────────────────────────────────────┘
```

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
    language TEXT DEFAULT 'en',
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
    provider TEXT NOT NULL,  -- github, gitlab, gitee
    provider_id TEXT NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Git Servers
CREATE TABLE git_servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL,  -- github, gitlab, gitee, gitea, custom
    url TEXT,  -- For custom servers
    api_url TEXT,
    webhook_url TEXT,
    oauth_id TEXT,
    oauth_secret TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE git_credentials (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES git_servers(id),
    user_id INTEGER NOT NULL REFERENCES users(id),
    type TEXT NOT NULL,  -- token, ssh_key, ssh_password, oauth
    username TEXT,
    token TEXT,
    ssh_key TEXT,
    password TEXT,
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
    parameters TEXT,  -- JSON parameters used for this build
    started_at DATETIME,
    finished_at DATETIME,
    duration_ms INTEGER,
    log TEXT,
    FOREIGN KEY (project_id) REFERENCES projects(id),
    UNIQUE(project_id, number)
);

-- Build Parameters
CREATE TABLE build_parameters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id),
    name TEXT NOT NULL,
    type TEXT NOT NULL,  -- string, choice, boolean, password, text
    description TEXT,
    default_value TEXT,
    choices TEXT,  -- JSON array for choice type
    required BOOLEAN DEFAULT FALSE,
    is_secret BOOLEAN DEFAULT FALSE,
    sort_order INTEGER DEFAULT 0,
    UNIQUE(project_id, name)
);

-- Environment Variables
CREATE TABLE env_vars (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scope TEXT NOT NULL,  -- global, project
    project_id INTEGER,  -- NULL for global
    name TEXT NOT NULL,
    value TEXT NOT NULL,
    is_secret BOOLEAN DEFAULT FALSE,
    description TEXT,
    UNIQUE(scope, project_id, name)
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

-- Dev Environments
CREATE TABLE dev_environments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    install_path TEXT,
    is_installed BOOLEAN DEFAULT FALSE,
    auto_install BOOLEAN DEFAULT TRUE,
    download_source TEXT,  -- china, international, auto
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Templates
CREATE TABLE templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    description TEXT,
    icon TEXT,
    difficulty TEXT,  -- beginner, intermediate, advanced
    config TEXT NOT NULL,  -- JSON pipeline config
    is_builtin BOOLEAN DEFAULT TRUE,
    created_by INTEGER REFERENCES users(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
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

## [S16] Pipeline Templates

### Template System

Templates provide pre-built pipeline configurations for common workflows. Users can:
- Browse templates in the UI
- One-click create project from template
- Customize templates before use
- Share custom templates

### Template Categories

```
templates/
├── languages/
│   ├── nodejs/
│   │   ├── node-typescript.json
│   │   ├── node-javascript.json
│   │   └── node-monorepo.json
│   ├── go/
│   │   ├── go-cli.json
│   │   ├── go-api.json
│   │   └── go-microservice.json
│   ├── python/
│   │   ├── python-django.json
│   │   ├── python-flask.json
│   │   └── python-data-science.json
│   ├── java/
│   │   ├── java-maven.json
│   │   ├── java-gradle.json
│   │   └── java-spring-boot.json
│   └── dotnet/
│       ├── dotnet-web-api.json
│       └── dotnet-console.json
├── platforms/
│   ├── docker/
│   │   ├── docker-build-push.json
│   │   └── docker-compose-deploy.json
│   ├── kubernetes/
│   │   ├── k8s-deploy.json
│   │   └── k8s-helm.json
│   └── cloud/
│       ├── aws-ecs.json
│       ├── aws-lambda.json
│       ├── gcp-run.json
│       └── azure-app-service.json
├── game-dev/
│   ├── unity-android.json
│   ├── unity-ios.json
│   ├── unity-webgl.json
│   ├── unreal-android.json
│   └── godot-export.json
├── frontend/
│   ├── react-vercel.json
│   ├── vue-netlify.json
│   ├── angular-firebase.json
│   └── nextjs-vercel.json
├── devops/
│   ├── terraform-plan.json
│   ├── terraform-apply.json
│   ├── ansible-deploy.json
│   └── monitoring-setup.json
└── templates/
    ├── ci-only.json
    ├── cd-only.json
    ├── full-pipeline.json
    └── custom-step.json
```

### Template JSON Format

```json
{
  "id": "node-typescript",
  "name": "Node.js TypeScript",
  "description": "Build, test, and deploy Node.js TypeScript projects",
  "category": "languages/nodejs",
  "icon": "nodejs",
  "difficulty": "beginner",
  "tags": ["node", "typescript", "npm", "javascript"],
  "stages": [
    {
      "name": "Checkout",
      "steps": [
        { "type": "git.clone", "config": { "depth": 1 } }
      ]
    },
    {
      "name": "Install",
      "steps": [
        { "type": "shell", "config": { "command": "npm ci" } }
      ]
    },
    {
      "name": "Lint",
      "steps": [
        { "type": "shell", "config": { "command": "npm run lint" } }
      ],
      "optional": true
    },
    {
      "name": "Test",
      "steps": [
        { "type": "shell", "config": { "command": "npm test" } }
      ]
    },
    {
      "name": "Build",
      "steps": [
        { "type": "shell", "config": { "command": "npm run build" } }
      ]
    }
  ],
  "variables": [
    { "name": "NODE_VERSION", "default": "20", "description": "Node.js version" },
    { "name": "BUILD_COMMAND", "default": "npm run build", "description": "Build command" },
    { "name": "TEST_COMMAND", "default": "npm test", "description": "Test command" }
  ]
}
```

### Template UI

```
Projects → New Project → From Template
┌─────────────────────────────────────────────────────────────┐
│  Search templates...                                        │
├─────────────────────────────────────────────────────────────┤
│  Categories: [All] [Languages] [Platforms] [Game Dev]      │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
│  │ Node.js  │ │ Go       │ │ Python   │ │ Java         │  │
│  │ TS       │ │ CLI      │ │ Django   │ │ Maven        │  │
│  │ Beginner │ │ Beginner │ │ Medium   │ │ Medium       │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────┘  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
│  │ Docker   │ │ K8s      │ │ Unity    │ │ React        │  │
│  │ Build    │ │ Deploy   │ │ Android  │ │ Vercel       │  │
│  │ Medium   │ │ Advanced │ │ Advanced │ │ Beginner     │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

---

## [S17] Visual Operations & Beginner Experience

### Visual Pipeline Editor

```
Pipeline Editor
┌─────────────────────────────────────────────────────────────┐
│  Pipeline: my-app-build                                     │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────┐  │
│  │ Checkout │ →  │ Install  │ →  │ Test     │ →  │Deploy│  │
│  │ git.clone│    │ npm ci   │    │ npm test │    │ aws  │  │
│  └──────────┘    └──────────┘    └──────────┘    └──────┘  │
│       +              +              +              +        │
│  [Add Step]     [Add Step]     [Add Step]     [Add Step]   │
├─────────────────────────────────────────────────────────────┤
│  Selected: Test                                             │
│  Command: [npm test                    ]                    │
│  Timeout: [300] seconds                                     │
│  [Delete] [Duplicate] [Move Left] [Move Right]             │
└─────────────────────────────────────────────────────────────┘
```

### Beginner Onboarding

1. **First Login Wizard**
   - Welcome screen
   - Create admin account (or use default root/root)
   - Connect GitHub/GitLab (optional)
   - Create first project from template
   - Run first build

2. **Guided Tours**
   - Interactive tooltips for first-time users
   - Step-by-step guides for common tasks
   - Contextual help buttons

3. **Quick Start Templates**
   - "Hello World" template for each language
   - Pre-configured with sensible defaults
   - One-click deploy to popular platforms

### Visual Dashboard

```
┌─────────────────────────────────────────────────────────────┐
│  buildworld233 Dashboard                                    │
├──────────────────┬──────────────────────────────────────────┤
│                  │                                          │
│  Quick Actions   │  Recent Builds                          │
│  ──────────────  │  ──────────────────                     │
│  [+ New Project] │  ✅ my-app #123 - 2m ago (success)     │
│  [Run Build]     │  ❌ api-server #45 - 5m ago (failed)   │
│  [View Logs]     │  ✅ website #67 - 1h ago (success)     │
│                  │  ⏳ mobile-app #89 - running            │
│  System Status   │                                          │
│  ──────────────  │  Worker Status                          │
│  Workers: 3/3    │  ──────────────────                     │
│  Builds: 2/10    │  🟢 worker-01: online (2 builds)       │
│  Queue: 0        │  🟢 worker-02: online (1 build)        │
│  Uptime: 99.9%   │  🟢 worker-03: online (0 builds)      │
│                  │                                          │
├──────────────────┴──────────────────────────────────────────┤
│  [Projects] [Builds] [Workers] [Plugins] [Settings]        │
└─────────────────────────────────────────────────────────────┘
```

---

## [S18] Testing Strategy

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
      - uses: actions/setup-node@v4
        with:
          node-version: '24'
          cache: 'npm'
          cache-dependency-path: web/package-lock.json
      - name: Run Go tests
        run: go test ./...
      - name: Run Go integration tests
        run: go test -tags=integration ./...
      - name: Build and test frontend
        working-directory: web
        run: |
          npm ci
          npm run build
          npm run test
      - name: E2E Tests
        run: |
          go build -o buildworld233 ./cmd/server
          ./buildworld233 start &
          sleep 2
          cd web && npx playwright test
```

---

## [S17] Implementation Phases

### Phase 1: Foundation (Week 1-2)
- [x] Go project setup with go.mod
- [x] SQLite database layer with migrations
- [x] Configuration system (YAML)
- [x] CLI framework (cobra)
- [x] Basic HTTP server (chi)

### Phase 2: Core Engine (Week 3-4)
- [x] Build scheduler
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
- [ ] Default admin account (root / root)

### Phase 6: Web UI (Week 11-14)
- [ ] React/Vite setup with Node 24 LTS
- [ ] Dashboard with visual overview
- [ ] Project management (visual CRUD)
- [ ] Build history & logs (real-time)
- [ ] User management (visual)
- [ ] Plugin manager (visual install/enable/disable)
- [ ] Worker management UI (visual)
- [ ] System settings (visual)
- [ ] Backup & restore UI (visual)
- [ ] Pipeline visual editor (drag-and-drop)
- [ ] Template gallery browser

### Phase 7: Templates & Visual (Week 15-16)
- [ ] Pipeline template system
- [ ] 50+ pre-built templates:
  - [ ] Node.js/TypeScript projects
  - [ ] Go projects
  - [ ] Python projects
  - [ ] Java/Maven projects
  - [ ] .NET projects
  - [ ] Docker build & push
  - [ ] Kubernetes deployment
  - [ ] AWS/GCP/Azure deployment
  - [ ] Unity game builds
  - [ ] React/Vue/Angular frontend
  - [ ] Static site generators
  - [ ] Database migrations
  - [ ] API testing
  - [ ] Security scanning
  - [ ] Performance testing
- [ ] Template customizer UI
- [ ] Template sharing/import

### Phase 8: CI/CD & Release (Week 17-18)
- [ ] GitHub Actions CI (Node 24 LTS)
- [ ] Automated test suite (100% pass required)
- [ ] Install scripts (.ps1, .sh) for server & worker
- [ ] Self-update mechanism
- [ ] Docker support (server + worker)
- [ ] Documentation site
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
15. **GitHub CI fully automated** — Node 24 LTS, tests must pass before merge
16. **Visual operations** — All features accessible via web UI (no CLI required)
17. **Massive templates** — Pre-built pipeline templates for common workflows
18. **Beginner friendly** — Zero-to-build in under 5 minutes
19. **Default admin** — root / root (change on first login)

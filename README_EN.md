<p align="center">
  <h1 align="center">BuildWorld</h1>
  <p align="center"><strong>Production CI/CD Server — A Modern Jenkins Alternative</strong></p>
  <p align="center">
    TypeScript DSL · Distributed Workers · React Dashboard · Lark Notifications · 28+ Plugins
  </p>
</p>

<p align="center">
  <a href="https://github.com/neko233-com/buildworld/releases"><img alt="GitHub Release" src="https://img.shields.io/github/v/release/neko233-com/buildworld"></a>
  <a href="https://github.com/neko233-com/buildworld/blob/main/README.md"><img alt="中文" src="https://img.shields.io/badge/中文-README.md-blue"></a>
</p>

---

## Quick Install

> Prerequisites: [GitHub CLI](https://cli.github.com/) installed and `gh auth login` completed.

<table>
<tr>
<td><strong>macOS / Linux</strong></td>
<td><strong>Windows PowerShell</strong></td>
</tr>
<tr>
<td>

```sh
gh api -H "Accept: application/vnd.github.raw+json" \
  repos/neko233-com/buildworld/contents/scripts/install.sh | sh
buildworld status
```

</td>
<td>

```powershell
& ([scriptblock]::Create((
  gh api -H "Accept: application/vnd.github.raw+json" `
    repos/neko233-com/buildworld/contents/scripts/install.ps1 | Out-String
)))
buildworld status
```

</td>
</tr>
</table>

Open `http://127.0.0.1:8080`, log in with `root` / `root`, and **change the password immediately**.

<details>
<summary>Pinned version / Air-gapped install</summary>

```sh
# macOS / Linux
sh install.sh 1.15.0

# Windows
.\install.ps1 -Version 1.15.0
```

For air-gapped environments, distribute the install scripts, bundle, and `checksums.txt` from `release/vX.Y.Z/` to a trusted internal endpoint.
</details>

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      BuildWorld Server                       │
│                    Go · Chi · SQLite · JWT                    │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
│  │ REST API │  │ WebSocket│  │  gRPC    │  │  Webhooks  │  │
│  │ /api/*   │  │ Live Logs│  │ Workers  │  │ GH/GL/Gitea│  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └─────┬──────┘  │
│       │              │              │               │         │
│  ┌────┴──────────────┴──────────────┴───────────────┴──────┐ │
│  │                    Build Engine                          │ │
│  │  Scheduler · Queue · Executor · Plugins · Logs · Notify  │ │
│  └────────────────────────┬────────────────────────────────┘ │
│                           │                                   │
│  ┌────────────────────────┴────────────────────────────────┐ │
│  │                  SQLite + Migrations                     │ │
│  └──────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
         │                              │
    REST / WS                        gRPC
         │                              │
┌────────┴────────┐          ┌──────────┴──────────┐
│   React SPA     │          │   Distributed       │
│   Dashboard     │          │   Workers           │
│   Vite + Monaco │          │   cmd/worker        │
└─────────────────┘          └─────────────────────┘
```

**Components:**

| Component | Directory | Responsibility |
|-----------|-----------|---------------|
| **Server** | `cmd/server` | HTTP API, WebSocket live logs, gRPC worker management, build engine |
| **CLI** | `cmd/cli` | User interaction: start / stop / restart / status / reset-password |
| **Worker** | `cmd/worker` + `sdk/` | Distributed build executor, auto-registers via gRPC |
| **Frontend** | `web/` | React SPA — Jenkins-style dashboard, Monaco editor, live logs |
| **Plugins** | `plugins/` | 28+ build/deploy/notify plugins, runtime dynamic loading |
| **Migrator** | `internal/jenkins` | Jenkinsfile → TypeScript DSL auto-conversion |

---

## Core Features

### Pipeline DSL

Two declarative pipeline formats — **TypeScript** and **YAML**:

```typescript
// TypeScript DSL — Monaco editor provides validation & completion
pipeline({
  stages: [
    stage("Build", async () => {
      await sh("go build -o app ./cmd/server");
    }),
    stage("Test", async () => {
      await sh("go test ./...");
    }),
    stage("Deploy", async () => {
      await notify({ type: "feishu", message: "Deploy complete" });
    }),
  ],
});
```

TypeScript pipelines run in a restricted sandbox — **no `eval`, no `new Function`**.

### Distributed Workers

- Built-in `builtin` executor: single-machine ready
- Remote workers auto-register via gRPC for horizontal scaling
- Worker SDK (`sdk/`) for custom executor implementations
- Isolated workspaces per build, auto-cleanup

### Web Dashboard

Jenkins-style but modern React dashboard:

- **Dashboard** — Project list, status overview, left-rail queue & recent builds
- **Build Queue** — Real-time queue, cancel & reorder
- **Build Detail** — Live log streaming, artifacts, env vars, changeset
- **Project Configure** — Visual pipeline config, triggers, credentials
- **Big Screen** — Monitoring mode
- **Credentials** — Username/password, SSH Key, API Token management
- **Audit Log** — Operation audit trail
- **Statistics** — Build stats & trend analysis

### Plugin Ecosystem

28+ out-of-the-box plugins covering mainstream tools & platforms:

| Category | Plugins |
|----------|---------|
| **Build Tools** | Go, Cargo, Gradle, Maven, npm, pip, .NET, Shell, PowerShell |
| **Container/Deploy** | Docker, Kubernetes, Helm, ArgoCD, Terraform, Ansible |
| **SCM** | GitHub, GitLab, Gitea, SVN, Mercurial |
| **Notifications** | Feishu (Lark), Slack, Discord, Telegram, Webhook |
| **Artifacts/Security** | S3, Vault, SonarQube |

### Jenkins Migration

Progressive migration with zero downtime:

1. **Import** — Auto-parse Jenkinsfile, generate TypeScript DSL
2. **Parallel Run** — Execute on both Jenkins and BuildWorld for the same commit
3. **Compare** — Diff env vars, artifacts, notifications, deploy results
4. **Cut Over** — Disable Jenkins triggers, enable BuildWorld triggers

### Lark (Feishu) Notifications

Native Go implementation, no external dependencies:

```json
{
  "webhook_url": "https://open.feishu.cn/open-apis/bot/v2/hook/REDACTED"
}
```

Supports build start, success, and failure events with rich card messages.

---

## Operations

```sh
buildworld start              # Start service
buildworld stop               # Stop service
buildworld restart            # Restart
buildworld status             # Check status
buildworld pause              # Pause (unloads background service)
buildworld resume             # Resume
buildworld enable-autostart   # Enable auto-start on boot
buildworld disable-autostart  # Disable auto-start
```

```sh
# Reset admin password
printf '%s\n' 'new-strong-password' | buildworld reset-root-password --password-stdin
```

On update failure, the previous bundle is preserved in `.previous` for rollback.

---

## Defaults

| Item | Default |
|------|---------|
| Console / API | `http://127.0.0.1:8080` |
| Default admin | `root` / `root` |
| Database | SQLite (`data/buildworld.db`) |
| macOS bundle | `~/.local/lib/buildworld` |
| macOS CLI | `~/.local/bin/buildworld` |
| Data & logs | `~/Library/Application Support/buildworld` (macOS) |

For LAN access, change `server.host` to a controlled NIC address or use a reverse proxy + TLS. **Never commit passwords, webhooks, or Git credentials.**

---

## Development

```bash
# Backend
go test ./...

# Frontend
cd web && npm ci && npm test -- --run && npm run build

# Docs
cd docs-site && npm ci && npm run build
```

### Local Release

```powershell
# Full verification gate
.\scripts\verify-local.ps1

# Six-platform bundle (Windows / Linux / macOS × amd64 / arm64)
.\scripts\release-local.ps1 -Version 1.15.0

# Publish to GitHub Release
.\scripts\publish-release-local.ps1 -Version 1.15.0 -Publish -ReplaceExisting
```

All CI, testing, packaging, and publishing run locally — GitHub Actions is not used.

---

## Project Structure

```
buildworld/
├── cmd/
│   ├── cli/          # CLI entry point
│   ├── server/       # Server entry point
│   └── worker/       # Distributed worker entry point
├── internal/         # Core logic (API · Engine · Store · Auth · Plugin)
├── web/              # React SPA (Vite · TypeScript · Monaco)
├── plugins/          # 28+ plugins (build · deploy · notify · SCM)
├── proto/            # gRPC protocol definitions
├── sdk/              # Worker SDK
├── scripts/          # Install · publish · verification scripts
├── docs-site/        # Docusaurus documentation site
└── integration/      # Integration tests
```

---

<p align="center">
  <a href="README.md">中文版本 →</a>
</p>

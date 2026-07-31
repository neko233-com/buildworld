---
sidebar_position: 1
---

# buildworld

A modern CI/CD server - Jenkins alternative with restricted TypeScript and
GitHub Actions-style YAML pipelines, a plugin system, and distributed workers.

## Features

- **TypeScript Pipelines** - Typed, comment-friendly configuration with syntax diagnostics and a restricted runtime
- **YAML Pipelines** - GitHub Actions-style YAML remains available for declarative workflows
- **Plugin System** - Checksummed, out-of-process Go binary plugins with explicit lifecycle controls
- **Agent-First Architecture** - Distributed build execution with agent pools and requirements
- **Git Integration** - Reusable VCS repository templates for Git URLs, credentials, branches, polling, and checkout
- **SSH Authentication** - Key and password-based SSH auth with credential management
- **Environment Variables** - Global and project-level variables
- **Parameterized Builds** - Build parameters with validation
- **Dev Environments** - Auto-setup JDK, Maven, Node.js, Gradle
- **Build Templates** - Reusable build templates with project overrides
- **Build Triggers** - Schedule (cron), VCS polling, and finish triggers
- **Artifact Management** - Secure artifact storage with SHA256 verification
- **Notifications** - Email, Feishu, and HTTP webhook notifications
- **Internationalization** - English and Chinese with automatic language detection
- **Modern UI** - React dashboard with real-time updates

Repository SSH keys are imported and managed as VCS credentials. BuildWorld
does not generate or retain a separate per-user SSH identity registry.

## Quick Start

```bash
# Install
gh api -H "Accept: application/vnd.github.raw+json" repos/neko233-com/buildworld/contents/scripts/install.sh | sh

# Start server
buildworld start

# Open browser
open http://localhost:8080
```

Default login: `root` / `root`

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    buildworld (Server)                    │
├──────────────┬──────────────┬──────────────┬───────────────┤
│   Web UI     │  REST API    │  WebSocket   │  CLI (bwctl)  │
│  (React/Vite)│  (chi)       │  (build log  │  (cobra)      │
│              │              │   streaming) │               │
├──────────────┴──────────────┴──────────────┴───────────────┤
│                     Core Engine                             │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
│  │ Build    │ │ Pipeline │ │ Agent    │ │ Workspace    │  │
│  │ Scheduler│ │ Executor │ │ Manager  │ │ Manager      │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────┘  │
├─────────────────────────────────────────────────────────────┤
│                Go Binary Plugin System                     │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
│  │ Plugin   │ │ Plugin   │ │ Plugin   │ │ Plugin       │  │
│  │ Loader   │ │ Registry │ │ Checksum │ │ Lifecycle    │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────┘  │
├─────────────────────────────────────────────────────────────┤
│                  Storage Layer (SQLite)                     │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
│  │ Users    │ │ Projects │ │ Builds   │ │ Artifacts    │  │
│  │ & Auth   │ │ & Config │ │ History  │ │ & Logs       │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Next Steps

- [Installation](./installation.md) - Install buildworld
- [Configuration](./configuration.md) - Configure your instance
- [Pipeline Guide](./pipelines.md) - Create build pipelines
- [Go binary plugins](./plugins.md) - Extend pipeline step types


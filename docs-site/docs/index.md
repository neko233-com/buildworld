---
sidebar_position: 1
---

# buildworld

A modern CI/CD server - Jenkins alternative with YAML/JSON pipelines, plugin system, and distributed workers.

## Features

- **YAML/JSON Pipelines** - Support GitHub Actions-style YAML and JSON pipeline definitions
- **Plugin System** - Hot-reloadable plugins with goja JS runtime
- **Agent-First Architecture** - Distributed build execution with agent pools and requirements
- **VCS Integration** - Git, SVN, Mercurial support with VCS Roots
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

## Quick Start

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.sh | bash

# Start server
buildworld start

# Open browser
open http://localhost:8700
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
│                   Plugin System (goja)                      │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐  │
│  │ Plugin   │ │ Plugin   │ │ Plugin   │ │ Plugin       │  │
│  │ Loader   │ │ Registry │ │ Sandbox  │ │ Hot-Reload   │  │
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

- [Installation](/installation) - Install buildworld
- [Configuration](/configuration) - Configure your instance
- [Pipeline Guide](/pipelines) - Create build pipelines
- [Plugin Development](/plugins) - Write custom plugins

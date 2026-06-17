---
sidebar_position: 1
---

# buildworld233

A modern CI/CD server - Jenkins alternative with TypeScript DSL, plugin system, and distributed workers.

## Features

- **TypeScript DSL** - Write build pipelines in TypeScript/JavaScript
- **Plugin System** - Hot-reloadable plugins with goja runtime
- **Agent-First Architecture** - Distributed build execution
- **Git/SVN Integration** - Support for GitHub, GitLab, Gitee, Gitea, and SVN
- **SSH Authentication** - Key and password-based SSH auth
- **Environment Variables** - Global and project-level variables
- **Parameterized Builds** - Build parameters with validation
- **Dev Environments** - Auto-setup JDK, Maven, Node.js, Gradle
- **Templates** - 8+ built-in pipeline templates
- **Modern UI** - React dashboard with real-time updates

## Quick Start

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.sh | bash

# Start server
buildworld233 start

# Open browser
open http://localhost:6050
```

Default login: `root` / `root`

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    buildworld233 (Server)                    │
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

- [Installation](/installation) - Install buildworld233
- [Configuration](/configuration) - Configure your instance
- [Pipeline Guide](/pipelines) - Create build pipelines
- [Plugin Development](/plugins) - Write custom plugins

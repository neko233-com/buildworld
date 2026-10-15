---
title: BuildWorld CI/CD Server
description: BuildWorld is a self-hosted CI/CD server and Jenkins alternative for build automation, TypeScript and YAML pipelines, and deployment workflows.
sidebar_position: 1
---

# BuildWorld

BuildWorld is an open-source, self-hosted CI/CD server and Jenkins alternative
for continuous integration, continuous delivery, build automation, pipelines
as code, and deployment workflows.

## Screenshots

[![BuildWorld build details with the execution timeline, retry chain, and live logs](/img/screenshots/build-details-2026-07-22.png)](/img/screenshots/build-details-2026-07-22.png)

Archived local desktop capture, **2026-07-22**, Chinese interface. Current releases have updated layouts and date formatting. [Screenshot notes and full-size image](./screenshots.md).

## Features

- **TypeScript Pipelines** - Typed, comment-friendly configuration with syntax diagnostics and a restricted runtime
- **YAML Pipelines** - GitHub Actions-style YAML remains available for declarative workflows
- **Plugin System** - Checksummed, out-of-process Go binary plugins with explicit lifecycle controls
- **Local-First Execution** - The built-in `builtin` executor runs builds on the server; remote Workers are optional
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

### macOS / Linux

```sh
curl -fsSL https://gh-proxy.com/https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.sh | sh
buildworld status
```

### Windows PowerShell (`install.ps1`)

```powershell
& ([scriptblock]::Create((Invoke-WebRequest -UseBasicParsing https://gh-proxy.com/https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.ps1).Content))
buildworld status
```

Open `http://127.0.0.1:8080`. Default login: `root` / `root`; change the password immediately after the first login. Both installers verify SHA-256 and add the CLI to your user PATH. [Installation options](./installation.md).

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


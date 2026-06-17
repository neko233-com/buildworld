# Changelog

All notable changes to buildworld233 will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.0] - 2026-06-18

### Added

#### GitHub Documentation Site
- Docusaurus-based documentation site
- Multi-language support (English/Chinese)
- Installation guide
- Configuration guide
- Pipeline guide
- Plugin development guide
- Agent configuration guide
- Templates documentation

#### Custom Account Presets
- Pre-configured user roles
- Default admin account (root/root)
- Role-based access control (admin, maintainer, developer, viewer)

## [1.4.0] - 2026-06-17

### Added

#### Development Environments
- JDK 21 auto-detection and setup
- Maven 3.9.6 auto-detection and setup
- Node.js 24 auto-detection and setup
- npm auto-detection and setup
- Gradle 8.5 auto-detection and setup
- Environment variable configuration

#### Pipeline Templates
- 8 built-in templates:
  - Node.js TypeScript
  - Go CLI Application
  - Python Django
  - Docker Build & Push
  - Kubernetes Deploy
  - Unity Android Build
  - React (Vercel)
- Template search and filtering
- Custom template creation
- Template export/import
- Template save/load to files

## [1.3.0] - 2026-06-17

### Added

#### Environment Variables System
- Global environment variables (${global.X})
- Project-level environment variables (${project.X})
- Variable resolution with project override
- Secret field masking in UI
- System environment variable loading

#### Parameterized Builds
- Build parameter types: string, choice, boolean, password, text
- Parameter validation
- Build configuration parsing
- Build history tracking
- Build number auto-increment

## [1.2.0] - 2026-06-17

### Added

#### SVN Support
- SVN client for checkout/update/commit operations
- SVN repository information retrieval
- SVN status checking

#### SSH Password Authentication
- SSH key pair generation (Ed25519)
- SSH public key retrieval
- SSH fingerprint generation
- Git clone with SSH password authentication
- Credential helper for password-based SSH auth

## [1.1.0] - 2026-06-17

### Added

#### TypeScript DSL Plugin System
- **TypeScript/JavaScript runtime via goja** - Full ES5.1+ support for plugin scripts
- Plugin scripts can now be written in `.js` or `.ts` files
- Hot-reload now works with JavaScript files
- All existing tests updated to use JavaScript syntax

### Changed
- Plugin loader now supports both `.js` and `.ts` file extensions
- Removed gopher-lua dependency, replaced with goja (pure Go, no CGO)

## [1.0.0] - 2026-06-17

### Added

#### Core Engine
- Build scheduler with priority queue
- Shell executor with output streaming
- Task queue management
- Rolling log buffer (configurable max lines)

#### Agent System (Agent-First Architecture)
- Agent daemon with gRPC registration
- Agent health monitoring (CPU, memory, disk)
- Agent heartbeat system
- Build task assignment to agents
- Agent label matching for task routing
- Agent failover and auto-reconnect
- Local agent (built-in, starts with server)
- Remote agent support via gRPC

#### Plugin System
- Plugin loader with Lua runtime
- Plugin hot-reload with fsnotify
- Per-plugin debounce timers
- Plugin registry
- Plugin lifecycle hooks

#### Authentication
- JWT token generation and validation
- Default admin account (root / root)
- RBAC roles: admin, maintainer, developer, viewer

#### Git Integration
- Git client for clone/pull/checkout
- Support for GitHub, GitLab, Gitee, Gitea
- Custom git server URL recording
- Webhook receivers

#### Configuration
- YAML configuration system
- Config validation
- Config hot-reload with fsnotify

#### Database
- SQLite database layer (embedded, zero-config)
- Auto-migration on startup
- Models for users, projects, builds, artifacts, agents, plugins

#### CLI
- CLI framework with cobra
- Commands: start, stop, status, version
- Agent management commands

#### HTTP Server
- REST API with chi router
- Health and version endpoints
- Request ID middleware
- Content-Type JSON middleware
- CORS support
- Method not allowed handling

#### Frontend
- React + Vite setup
- Tailwind CSS v4
- React Router for navigation
- Basic pages: Dashboard, Projects, Builds

#### CI/CD
- GitHub Actions test workflow
- GitHub Actions release workflow
- Cross-platform builds (Linux, macOS, Windows)
- AMD64 and ARM64 support

#### Installation
- Windows PowerShell install script
- Linux/macOS bash install script
- Worker/agent install scripts

#### Design Documentation
- Complete design specification
- Implementation plan with 18 tasks
- Database schema
- API design
- Worker/agent architecture

### Fixed

- Plugin hot-reload race condition (concurrent map writes)
- Per-plugin debounce timers to prevent cross-plugin interference
- CI workflow: removed E2E tests, added go vet and binary verification

### Security

- Thread-safe plugin loader with mutex
- Configurable JWT secret
- Role-based access control

---

## [Unreleased]

### Planned

#### Environment Variables
- Global environment variables (${global.X})
- Project-level environment variables (${project.X})
- Secret fields (hidden after configuration)

#### Parameterized Builds
- Visual parameter editor
- Parameter types: string, choice, boolean, password, text
- Build parameters UI (Feishu-style)

#### Dev Environments
- Built-in JDK 21, Maven, Node.js 24, npm, Gradle
- One-click environment setup
- Auto-detect download source (China/international)

#### Security
- HTTPS via proxysss integration
- Secure credential vault (AES-256-GCM encryption)
- SSH password storage

#### Multi-language
- English (en)
- Chinese Simplified (zh-CN)

#### Templates
- 50+ pre-built pipeline templates
- Template categories: languages, platforms, game-dev, frontend, devops
- Template customizer UI

#### Web UI
- Visual pipeline editor (drag-and-drop)
- Rolling logs viewer with auto-scroll
- Real-time build tracking
- Server monitor (CPU, memory, disk, agents)
- Backup & restore UI
- User management
- Plugin manager
- System settings

#### Documentation
- Docusaurus-like docs site
- API documentation
- User guides
- Plugin development guide

#### Installation
- One-click HTTPS setup
- Built-in dev environments
- Auto-update mechanism

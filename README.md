<p align="center">
  <h1 align="center">BuildWorld</h1>
  <p align="center"><strong>A self-hosted CI/CD server and modern Jenkins alternative</strong></p>
  <p align="center">Build automation · Pipeline as code · Deployment workflows · React dashboard</p>
</p>

<p align="center">
  <a href="https://github.com/neko233-com/buildworld"><img alt="GitHub repository" src="https://img.shields.io/badge/GitHub-neko233--com%2Fbuildworld-181717?logo=github"></a>
  <a href="https://github.com/neko233-com/buildworld/releases"><img alt="Latest release" src="https://img.shields.io/github/v/release/neko233-com/buildworld"></a>
  <a href="https://github.com/neko233-com/buildworld/stargazers"><img alt="GitHub stars" src="https://img.shields.io/github/stars/neko233-com/buildworld"></a>
  <a href="https://neko233-com.github.io/buildworld/"><img alt="Documentation" src="https://img.shields.io/badge/docs-online-blue"></a>
</p>

<p align="center"><a href="README.md">English</a> · <a href="README_CN.md">简体中文</a></p>

BuildWorld is an open-source, self-hosted **CI/CD server** for continuous integration, continuous delivery, build automation, and deployment workflows. Use it as a **Jenkins alternative** with a web dashboard, TypeScript and YAML pipelines, an extensible plugin system, and a built-in `builtin` executor for local builds.

**Repository:** [github.com/neko233-com/buildworld](https://github.com/neko233-com/buildworld) · **Documentation:** [neko233-com.github.io/buildworld](https://neko233-com.github.io/buildworld/)

---

## Features

- **Self-hosted CI/CD:** run the server and its SQLite database on infrastructure you control.
- **Pipeline as code:** define pipelines in TypeScript or YAML, with validation and completion in the Monaco editor.
- **Built-in execution:** the local `builtin` executor is the default; remote Workers remain an optional deployment choice.
- **Build dashboard:** review projects, queued and running builds, recent build results, logs, artifacts, and audit history.
- **Plugin ecosystem:** use build, deployment, source-control, notification, artifact, and security integrations.
- **Jenkins migration:** import Jenkinsfiles, generate TypeScript DSL, compare pipeline results, and migrate in stages.
- **Operational CLI:** start, pause, resume, restart, inspect, and recover the service from the command line.

## Quick install

The installer uses a GitHub mirror by default. Set `BUILDWORLD_GITHUB_MIRROR=off` to download directly from GitHub.

### macOS / Linux

```sh
curl -fsSL https://gh-proxy.com/https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.sh | sh
buildworld status
```

### Windows PowerShell (`install.ps1`)

Run this in PowerShell to download and run the Windows one-click installer:

```powershell
& ([scriptblock]::Create((
  (Invoke-WebRequest -UseBasicParsing `
    https://gh-proxy.com/https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.ps1).Content
)))
buildworld status
```

Open `http://127.0.0.1:8080` and sign in with `root` / `root`. **Change the default password immediately after the first login.**

<details>
<summary>Install a specific version or deploy offline</summary>

```sh
# macOS / Linux
sh install.sh 1.15.0
```

```powershell
# Windows
.\install.ps1 -Version 1.15.0
```

For an air-gapped environment, publish the install scripts, platform bundle, and `checksums.txt` from `release/vX.Y.Z/` to a trusted internal download host.
</details>

## Pipeline as code

BuildWorld supports declarative TypeScript and YAML pipelines. The TypeScript editor provides validation and completion; pipeline code runs in a restricted sandbox without `eval` or `new Function`.

```typescript
pipeline({
  stages: [
    stage("Build", async () => {
      await sh("go build -o app ./cmd/server");
    }),
    stage("Test", async () => {
      await sh("go test ./...");
    }),
    stage("Deploy", async () => {
      await notify({ type: "feishu", message: "Deployment complete" });
    }),
  ],
});
```

## Build dashboard

The React dashboard brings day-to-day CI/CD operations into one place:

- **Dashboard:** project list, build status, queue, and recent builds.
- **Builds in Progress:** inspect queued and running jobs and follow their live logs.
- **Build details:** review artifacts, environment variables, and changesets.
- **Project configuration:** manage pipeline definitions, triggers, and credentials.
- **Operations:** access the audit log, build statistics, and monitoring views.

## Plugins and integrations

BuildWorld includes plugins for common build tools and delivery systems. Available integrations include:

| Category | Examples |
|---|---|
| Build tools | Go, Cargo, Gradle, Maven, npm, pip, .NET, Shell, PowerShell |
| Containers and deployment | Docker, Kubernetes, Helm, Argo CD, Terraform, Ansible |
| Source control | GitHub, GitLab, Gitea, SVN, Mercurial |
| Notifications | Feishu (Lark), Slack, Discord, Telegram, Webhooks |
| Artifacts and security | S3, Vault, SonarQube |

## Migrate from Jenkins

Move pipelines incrementally:

1. Import a Jenkinsfile and generate a TypeScript pipeline.
2. Run the same commit through Jenkins and BuildWorld during evaluation.
3. Compare environment variables, artifacts, notifications, and deployment results.
4. Switch triggers after the results meet your acceptance criteria.

## Service operations

```sh
buildworld start
buildworld pause
buildworld resume
buildworld restart
buildworld status
buildworld enable-autostart
buildworld disable-autostart
```

Reset the administrator password without placing it in shell history:

```sh
printf '%s\n' 'new-strong-password' | buildworld reset-root-password --password-stdin
```

If an update fails, the previous bundle is retained in `.previous` for rollback.

## Defaults and security

| Setting | Default |
|---|---|
| Web console and API | `http://127.0.0.1:8080` |
| Initial administrator | `root` / `root` — change after first login |
| Database | SQLite at `data/buildworld.db` |
| Default executor | Local `builtin` executor |

For LAN access, bind `server.host` to a controlled network interface or use a reverse proxy with TLS. Never commit passwords, webhooks, tokens, or Git credentials.

## Development and local release

```sh
# Go backend
go test ./...

# Web frontend
cd web && npm ci && npm test -- --run && npm run build

# Documentation site
cd docs-site && npm ci && npm run build
```

```powershell
# Run the full local verification gate
.\scripts\verify-local.ps1

# Build bundles for Windows, Linux, and macOS (amd64 and arm64)
.\scripts\release-local.ps1 -Version 1.15.0

# Publish a verified bundle to GitHub Releases
.\scripts\publish-release-local.ps1 -Version 1.15.0 -Publish -ReplaceExisting
```

Verification, builds, packaging, and release uploads run locally. This project does not use GitHub Actions for CI or publishing.

## Repository layout

```text
buildworld/
├── cmd/          # CLI, server, and Worker entry points
├── internal/     # API, engine, storage, authentication, and plugins
├── web/          # React dashboard (Vite, TypeScript, Monaco)
├── plugins/      # Build, deployment, notification, and SCM integrations
├── proto/        # gRPC protocol definitions
├── sdk/          # Optional Worker SDK
├── scripts/      # Install, release, and verification scripts
├── docs-site/    # English and Simplified Chinese documentation
└── integration/  # Integration tests
```

## Links

- [BuildWorld on GitHub](https://github.com/neko233-com/buildworld)
- [Documentation](https://neko233-com.github.io/buildworld/)
- [Releases](https://github.com/neko233-com/buildworld/releases)
- [Issues and feature requests](https://github.com/neko233-com/buildworld/issues)
- [中文 README](README_CN.md)

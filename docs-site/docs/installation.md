---
sidebar_position: 2
---

# Installation

## Quick Install

The repository is currently private. Authenticate once with `gh auth login`;
the installer then reuses `GH_TOKEN`, `GITHUB_TOKEN`, or the GitHub CLI token.

### Linux/macOS

```bash
gh api -H "Accept: application/vnd.github.raw+json" repos/neko233-com/buildworld233/contents/scripts/install.sh | sh
```

### Windows (PowerShell)

```powershell
& ([scriptblock]::Create((gh api -H "Accept: application/vnd.github.raw+json" repos/neko233-com/buildworld233/contents/scripts/install.ps1 | Out-String)))
```

Both commands continue to work if the repository becomes public. The
installers also support anonymous public-release downloads.

## Manual Install

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/neko233-com/buildworld233/releases). Every release contains a checksum-verified CLI, server, worker, and web UI bundle.

### Available Binaries

| Platform | CLI | Server | Worker |
|----------|-----|--------|--------|
| Linux AMD64 | ✅ | ✅ | ✅ |
| Linux ARM64 | ✅ | ✅ | ✅ |
| macOS AMD64 | ✅ | ✅ | ✅ |
| macOS ARM64 | ✅ | ✅ | ✅ |
| Windows AMD64 | ✅ | ✅ | ✅ |
| Windows ARM64 | ✅ | ✅ | ✅ |

### Install to PATH

```bash
# The one-click installers add the CLI to your user PATH and verify SHA-256.
# Set BUILDWORLD_NO_AUTOSTART=1 (shell) or -NoAutostart (PowerShell) to skip
# background startup registration.
```

## Start Server

```bash
# Start server
buildworld start

# Enable silent per-user background startup (Windows Task Scheduler, launchd,
# or a systemd user service).
buildworld enable-autostart
```

## First Login

1. Open browser to `http://localhost:8700`
2. Login with default credentials:
   - Username: `root`
   - Password: `root`
3. **Change the default password immediately!**

Use a non-interactive, history-safe reset when necessary:

```bash
printf '%s\n' 'a-long-new-password' | buildworld reset-root-password --password-stdin
```

## Verify Installation

```bash
# Check server status
buildworld status

# Check version
buildworld version

# Lifecycle controls
buildworld pause
buildworld resume
buildworld restart
```

## Docker Installation

```bash
# Pull image
docker pull neko233/buildworld:latest

# Run container
docker run -d \
  -p 8700:8700 \
  -v buildworld-data:/data \
  --name buildworld \
  neko233/buildworld:latest
```

## Next Steps

- [Configuration](./configuration.md) - Configure your instance
- [Pipeline Guide](./pipelines.md) - Create build pipelines

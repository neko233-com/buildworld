---
sidebar_position: 2
---

# Installation

## Quick Install

The repository is public. The installers use the configured GitHub mirror by
default so they also work on machines that cannot reach GitHub directly.
Set `BUILDWORLD_GITHUB_MIRROR=off` to use GitHub directly.

### Linux/macOS

```bash
curl -fsSL https://gh-proxy.com/https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.sh | sh
```

### Windows (PowerShell)

```powershell
& ([scriptblock]::Create((Invoke-WebRequest -UseBasicParsing https://gh-proxy.com/https://raw.githubusercontent.com/neko233-com/buildworld/main/scripts/install.ps1).Content))
```

The mirror is only used to retrieve public release metadata and assets. The
installers still verify every downloaded file against the published SHA-256
manifest. Set `BUILDWORLD_GITHUB_MIRROR` to another trusted HTTPS mirror when
your network requires one.

## Manual Install

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/neko233-com/buildworld/releases). Every release contains a checksum-verified CLI, server, worker, and web UI bundle.

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

1. Open browser to `http://localhost:8080`
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
  -p 8080:8080 \
  -v buildworld-data:/data \
  --name buildworld \
  neko233/buildworld:latest
```

## Next Steps

- [Configuration](./configuration.md) - Configure your instance
- [Pipeline Guide](./pipelines.md) - Create build pipelines


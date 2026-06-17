---
sidebar_position: 2
---

# Installation

## Quick Install

### Linux/macOS

```bash
curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.sh | bash
```

### Windows (PowerShell)

```powershell
iwr -useb https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.ps1 | iex
```

## Manual Install

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/neko233-com/buildworld233/releases).

### Available Binaries

| Platform | Server | Agent |
|----------|--------|-------|
| Linux AMD64 | ✅ | ✅ |
| Linux ARM64 | ✅ | ✅ |
| macOS AMD64 | ✅ | ✅ |
| macOS ARM64 | ✅ | ✅ |
| Windows AMD64 | ✅ | ✅ |
| Windows ARM64 | ✅ | ✅ |

### Install to PATH

```bash
# Linux/macOS
sudo cp buildworld233 /usr/local/bin/
sudo cp buildworld233-agent /usr/local/bin/

# Windows
# Copy to a directory in your PATH
```

## Start Server

```bash
# Start server
buildworld233 start

# Start with custom port
buildworld233 start --port 8080

# Start as background service
buildworld233 enable-autostart
```

## First Login

1. Open browser to `http://localhost:6050`
2. Login with default credentials:
   - Username: `root`
   - Password: `root`
3. **Change the default password immediately!**

## Verify Installation

```bash
# Check server status
buildworld233 status

# Check version
buildworld233 version
```

## Docker Installation

```bash
# Pull image
docker pull neko233/buildworld233:latest

# Run container
docker run -d \
  -p 6050:6050 \
  -v buildworld233-data:/data \
  --name buildworld233 \
  neko233/buildworld233:latest
```

## Next Steps

- [Configuration](/configuration) - Configure your instance
- [Pipeline Guide](/pipelines) - Create build pipelines

---
sidebar_position: 3
---

# Configuration

## Configuration File

buildworld233 uses a YAML configuration file. Default location: `./config.yaml`

```yaml
server:
  host: "0.0.0.0"
  port: 6050
  tls: false

database:
  path: "./data/buildworld233.db"

auth:
  jwt_secret: "auto-generated-on-first-run"
  oauth:
    github:
      client_id: ""
      client_secret: ""

plugins:
  path: "./plugins"
  hot_reload: true

storage:
  workspace: "./data/workspaces"
  artifacts: "./data/artifacts"
  logs: "./data/logs"

agents:
  local:
    enabled: true
    max_concurrent_builds: 4
    workspace: "./data/workspaces"
    labels: ["linux", "amd64", "default"]
```

## Hot Reload

Configuration changes are automatically detected and applied:

```bash
# Watch for config changes
buildworld233 reload-config
```

Some settings require restart:
- `server.port`
- `server.tls`
- `database.path`

## Environment Variables

### Global Variables

```yaml
environment:
  global:
    REGISTRY: "registry.example.com"
    TOKEN:
      value: "secret123"
      secret: true
```

### Project Variables

```yaml
environment:
  project:
    API_KEY:
      value: "${global.TOKEN}"
      secret: true
```

## Agent Configuration

### Local Agent

```yaml
agents:
  local:
    enabled: true
    max_concurrent_builds: 4
    labels: ["linux", "amd64", "default"]
```

### Remote Agents

```yaml
agents:
  remote:
    - name: "agent-01"
      address: "192.168.1.100:7050"
      token: "agent-registration-token"
      labels: ["linux", "amd64", "gpu"]
      max_concurrent_builds: 8
```

## OAuth Configuration

### GitHub

```yaml
auth:
  oauth:
    github:
      client_id: "your-client-id"
      client_secret: "your-client-secret"
```

### GitLab

```yaml
auth:
  oauth:
    gitlab:
      client_id: "your-client-id"
      client_secret: "your-client-secret"
      url: "https://gitlab.com"
```

## Next Steps

- [Pipeline Guide](/pipelines) - Create build pipelines
- [Plugin Development](/plugins) - Write custom plugins
- [Agent Configuration](/agents) - Configure distributed agents

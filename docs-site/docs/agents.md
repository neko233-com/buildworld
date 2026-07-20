---
sidebar_position: 6
---

# Agent Configuration

## Overview

buildworld uses an agent-first architecture where agents (workers) execute builds. Agents can be local (built-in) or remote.

## Local Agent

The local agent starts automatically with the server:

```yaml
# config.yaml
agents:
  local:
    enabled: true
    max_concurrent_builds: 4
    workspace: "./data/workspaces"
    labels: ["linux", "amd64", "default"]
```

## Remote Agents

### Agent Installation

```bash
# Install agent
curl -fsSL https://raw.githubusercontent.com/neko233-com/buildworld233/main/scripts/install.sh | sh

# Start agent
buildworld-worker \
  --server http://localhost:8700 \
  --token <registration-token>
```

### Agent Configuration

```yaml
# config.yaml
agents:
  remote:
    - name: "agent-01"
      address: "192.168.1.100:7050"
      token: "agent-registration-token"
      labels: ["linux", "amd64", "gpu"]
      max_concurrent_builds: 8
      
    - name: "agent-02"
      address: "192.168.1.101:7050"
      token: "agent-registration-token"
      labels: ["macos", "arm64", "ios"]
      max_concurrent_builds: 4
```

## Agent Management

### CLI Commands

```bash
# List all agents
buildworld agent list

# Show agent status
buildworld agent status <agent-id>

# Enable agent
buildworld agent enable <agent-id>

# Disable agent
buildworld agent disable <agent-id>

# Remove agent
buildworld agent remove <agent-id>

# Generate registration token
buildworld agent generate-token
```

### Web UI

1. Go to Settings → Agents
2. View agent status and health
3. Enable/disable agents
4. Monitor active builds

## Agent Labels

Labels route builds to specific agents:

```typescript
// buildworld.config.ts
pipeline({
  name: "gpu-build",
  agent: {
    labels: ["gpu", "cuda"],  // Must match agent labels
  },
  stages: [...]
});
```

## Agent Health

Agents report health metrics:
- CPU usage
- Memory usage
- Disk usage
- Active build count

Health checks run every 10 seconds.

## Fault Tolerance

- **Heartbeat timeout**: 30 seconds → mark agent offline
- **Build failover**: Reassign to another agent if one goes offline
- **Auto-reconnect**: Agents automatically reconnect
- **Graceful shutdown**: Finish current builds before stopping

## Next Steps

- [Pipeline Guide](/pipelines) - Route builds to agents
- [Configuration](/configuration) - Configure agents

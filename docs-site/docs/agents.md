---
sidebar_position: 6
---

# Agents and workers

BuildWorld executes pipelines on its embedded local worker unless a pipeline
declares `agentRequirements`. A pipeline with requirements waits for a matching
remote worker; it does not fall back to local execution.

## Local execution

The local runner lives inside `buildworld-server`. The CLI-generated bootstrap
configuration includes this worker profile:

```yaml
workers:
  local:
    max_concurrent_builds: 4
    workspace: local
    pool: default
    labels: [go, nodejs, typescript]
```

Pipelines without `agentRequirements` use the embedded runner. Its effective
local concurrency is managed under **Settings → Runtime**, while total build
concurrency and timeout are under **Settings → Build policy**. Those runtime
values are stored in the BuildWorld database and take precedence when the
server starts.

## Start a remote worker

The release bundle contains `buildworld-worker` beside the CLI and server. The
default one-click locations are:

- Linux/macOS: `~/.local/lib/buildworld/buildworld-worker`
- Windows: `%LOCALAPPDATA%\BuildWorld\buildworld-worker.exe`

As an administrator, open **Settings → Agent automation**, generate an
enrollment token, and copy it immediately. Then run the worker on the remote
machine:

```bash
~/.local/lib/buildworld/buildworld-worker \
  --server http://buildworld.example:8700 \
  --token 'bw_enroll_…' \
  --listen :6051 \
  --advertise worker-01.example:6051 \
  --name worker-01 \
  --labels linux,amd64,go \
  --pool production \
  --max-builds 4 \
  --build-temp ./build_temp
```

`--advertise` must be an address that `buildworld-server` can reach. Allow
inbound TCP `6051` from the server, or choose another port in both `--listen`
and `--advertise`. The HTTP control plane remains on `8700`.

The v1 worker RPC transport is authenticated but not encrypted and does not
provide mTLS. Run it only on a trusted private network or VPN, bind/firewall the
worker port so only the BuildWorld server can reach it, and never expose the
worker or HTTP control-plane ports directly to the public Internet. Terminate
HTTPS for the control plane at a trusted reverse proxy when traffic leaves the
host.

The enrollment token authenticates automatic registration and remote build
dispatch. Rotating it prevents newly started workers with the previous token
from registering or receiving builds; restart workers with the new value.

## Route a TypeScript pipeline

Requirements match every declared label. Use `pool=<name>` to require a pool:

```typescript
import { definePipeline, shell, stage } from '@buildworld/pipeline'

export default definePipeline({
  name: 'linux-release',
  agentRequirements: ['linux', 'amd64', 'go', 'pool=production'],
  stages: [
    stage('Build', [
      shell('Compile', 'go build ./...'),
    ]),
  ],
})
```

For GitHub Actions-style YAML, `runs-on` maps to the same requirements:

```yaml
jobs:
  build:
    runs-on: [linux, amd64, go]
    steps:
      - run: go build ./...
```

## Registration and health

The worker registers through `POST /api/agents/auto-register`, then sends an
authenticated heartbeat every 10 seconds. BuildWorld marks a worker offline
after 30 seconds without a heartbeat. The **Agents** page shows its address,
labels, pool, active build count, capacity, and last heartbeat.

The current CLI does not manage agents. There are no `buildworld agent list`,
`enable`, or `generate-token` commands; use the Web UI or authenticated
`/api/agents` endpoints.

## Next steps

- [Pipeline Guide](./pipelines.md) - Route builds to remote workers
- [Configuration](./configuration.md) - Configure the server and local worker

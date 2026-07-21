---
sidebar_position: 3
---

# Configuration

## Configuration file

The `buildworld` CLI creates and uses a per-user configuration by default:

| Platform | Default path |
| --- | --- |
| Linux | `$XDG_CONFIG_HOME/buildworld/config.yaml`, or `~/.config/buildworld/config.yaml` when unset |
| macOS | `~/Library/Application Support/buildworld/config.yaml` |
| Windows | `%AppData%\buildworld\config.yaml` |

Pass `--config /path/to/config.yaml` before a CLI subcommand to use a different
file. When `buildworld-server` is started directly, its `-config` default is
`./config.yaml`.

This example contains the current bootstrap fields:

```yaml
server:
  host: 127.0.0.1
  port: 8700

database:
  path: "./data/buildworld.db"

auth:
  # Leave empty to create and persist a local signing secret.
  jwt_secret: ""

plugins:
  path: "./plugins"

storage:
  build_temp: "./build_temp"
  artifacts: "./artifacts"

workers:
  # Required only for remote worker auto-registration and dispatch.
  enrollment_token: ""
  local:
    max_concurrent_builds: 4
    workspace: local
    pool: default
    labels: [go, nodejs, typescript]

automation:
  github_webhook_secret: ""
```

Relative paths are resolved by the server process. The one-click installer
generates absolute per-user data paths, which avoids dependence on a service
manager's working directory.

## Apply changes

Treat `config.yaml` as startup configuration. After editing it, apply the
change with:

```bash
buildworld restart
buildworld status
```

There is no `buildworld reload-config` command. Build execution limits such as
local concurrency and CPU budget are managed in **Settings → Runtime**. Default
timeout, total concurrency, retry policy, and fail-fast validation are under
**Settings → Build policy**. They are applied to the running build scheduler.

Listening address, port, database, storage paths, plugin path, and worker
bootstrap settings require a restart. BuildWorld serves HTTP; terminate HTTPS
at a trusted reverse proxy and restrict direct access to the control-plane port.

`plugins.path` stores installed, validated Go binary plugins. BuildWorld does
not scan this directory for JavaScript or TypeScript plugin source.

`config.yaml` itself is not watched. Frontend reload is separate: Vite reloads
`web` source files during development, and the BuildWorld server watches its
disk-backed `web/dist` bundle for HTML, CSS, and JavaScript output changes.
CSS updates are applied in place; other frontend assets reload the open page.

## Variables and secrets

Global and project variables are database-backed settings, not nested
`environment.global` or `environment.project` keys in `config.yaml`. An
administrator can manage them through the authenticated `/api/env-vars/`
endpoint. Secret values are masked when the API lists them and when build
output is streamed, persisted, summarized, or sent to notifications:

```json
{
  "scope": "project",
  "project_id": 42,
  "name": "DEPLOY_TOKEN",
  "value": "secret-value",
  "is_secret": true,
  "description": "Deployment credential"
}
```

Send that object with `POST /api/env-vars/`, list values with
`GET /api/env-vars/?scope=project&project_id=42`, and delete one with
`DELETE /api/env-vars/{id}`.

BuildWorld masks secrets in API responses, logs, WebSocket output, failures,
and notifications. The SQLite database still stores the underlying secret so
builds can use it; it is not application-layer encrypted. BuildWorld sets the
database file to owner-only permissions on supported Unix systems. Protect the
service account, use full-disk encryption, and encrypt/restrict every backup.

Pipeline-local non-secret values belong directly in the pipeline:

```typescript
import { definePipeline, shell, stage } from '@buildworld/pipeline'

export default definePipeline({
  environment: {
    REGISTRY: 'registry.example.com',
  },
  stages: [
    stage('Build', [
      shell('Show registry', 'echo "${env.REGISTRY}"'),
    ]),
  ],
})
```

## Remote workers

Remote workers register dynamically with an enrollment token. Generate or
rotate that credential in **Settings → Agent automation**, then start
`buildworld-worker` with `--server`, `--token`, `--listen`, and a reachable
`--advertise` address. Workers register dynamically; there is no static remote
worker list in `config.yaml`.

See [Agents and workers](./agents.md) for the complete command and routing
syntax.

## Signed repository webhooks

Signed GitHub, GitLab, and Gitea push automation uses
`automation.github_webhook_secret`. An empty secret disables all three
receivers. The GitHub endpoint is:

```text
POST /api/webhooks/github
```

The corresponding GitLab and Gitea endpoints are `/api/webhooks/gitlab` and
`/api/webhooks/gitea`. Pushes only create builds for matching repositories;
commit messages cannot start server commands outside project pipelines.

## Lifecycle and password recovery

```bash
buildworld start
buildworld status
buildworld pause
buildworld resume
buildworld restart

printf '%s\n' 'a-long-new-password' |
  buildworld reset-root-password --password-stdin
```

## Next steps

- [Pipeline Guide](./pipelines.md) - Create build pipelines
- [Go binary plugins](./plugins.md) - Extend pipeline step types
- [Agents and workers](./agents.md) - Configure distributed execution

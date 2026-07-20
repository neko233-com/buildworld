# Buildworld Markdown Pipelines

Buildworld uses a readable Markdown pipeline instead of a Groovy Jenkinsfile. The
file is conventionally named `pipeline.buildworld.md` and is stored with the
project. `###` headings become graph nodes. By default they run in document
order; `- needs:` makes the graph explicit while keeping the document readable.

~~~~md
# Release pipeline

## Variables

- APP_NAME: portal
- NODE_ENV: production
- REGISTRY: registry.internal/portal

## Pipeline

### Checkout

```default shell
git clone $REPOSITORY_URL .
git checkout $GIT_REF
```

### Build package

- needs: Checkout

```default shell
npm ci
npm run build
```

```macos shell
./scripts/sign-macos.sh
```

### Publish notification

- needs: Build package

```default notify
channel: release-room
event: build.completed
```
~~~~

## Artifacts

- dist/*.zip
- reports/junit.xml

## Agents

- labels: macos, signing
- pool: release

## Retention

- completed: 60

## Toolchains

- go: 1.26
- node: 20, 22

## Execution model

- `default` runs on every worker, with its native shell (`cmd` on Windows and
  `sh` on macOS/Linux) unless a fence specifies a shell such as `powershell` or
  `bash`.
- `macos` and `windows` fences are platform additions. Buildworld executes the
  shared `default` script first and then the matching addition. There is no
  platform `if/else` in the pipeline.
- Global variables are managed by the server and injected for every project.
  Project variables then override a global variable of the same name. Use them
  as `${global.NAME}`, `${project.NAME}`, `${parameter.NAME}`, or `${env.NAME}`.
- The visual graph uses document order for automatic layout. Changing a heading
  or adding a code fence updates the graph immediately. Add `- needs: Checkout,
  Test` directly below a `###` heading to declare one or more prerequisite
  nodes. The parser rejects missing nodes, duplicate names, and dependency
  cycles, then runs the graph in stable topological order. Graph editing writes
  the same Markdown document rather than creating a second source of truth.
- A fence header may select a runtime, for example `default shell go@1.26` or
  `default shell node@22`. With exactly one configured version, `go` or `node`
  selects it automatically. Buildworld injects the resolved value as
  `BUILDWORLD_RUNTIME_GO`, `BUILDWORLD_RUNTIME_NODE`, and so on.
- A node can pass a key/value output to following nodes with an explicit marker:
  `echo "::buildworld:set IMAGE=registry.internal/app:42"`. Later scripts read
  it as `${build.IMAGE}`. This keeps node dependencies declarative.
- Use a `tail` node for a persistent development process. It is intentionally a
  long-running build node: its process output remains in the live build log
  until Buildworld stops the task or the server shuts down.
- Declare files to retain in `## Artifacts`. Patterns are collected after a
  successful build; a remote worker streams them back before its temporary
  workspace is removed.
- `## Agents` is optional. Without it, Buildworld uses the embedded local
  executor. Add `- labels: macos, signing` to require labels, or `- pool:
  release` to select a registered pool. These rules schedule the whole Markdown
  pipeline without introducing platform conditionals into a script.

## Retention and scheduling

Completed builds use an LRU retention policy. The default is the newest 30
completed builds per project. Pinned builds and anything still queued or running
are excluded. Add a `## Retention` section with `- completed: 60` to override
the project default; use a negative value to disable cleanup for a project.

Schedules are just as readable. Add one cron trigger under `## Schedule`; the
project schedule editor writes the same Markdown, retaining this document as the
source of truth for the visual graph and execution:

~~~~md
## Schedule

- cron: "0 2 * * *"
~~~~

The server is the default local executor and can also schedule registered remote
workers by labels/pools. `buildworld-worker` registers itself and sends a
heartbeat, so a macOS or Windows machine can execute the matching additions.
Remote dispatch uses the versioned `bytemsg233/v3` protobuf binary contract. The
server refuses an incompatible worker protocol rather than risking a partial
decode. Artifacts configured in the pipeline are streamed back in checksummed
512 KiB frames before the remote workspace is deleted (256 MiB maximum per
file). Configure `workers.enrollment_token`, then start workers with distinct
ports when sharing a computer: `buildworld-worker --listen :6051` and
`buildworld-worker --listen :6052`. On another machine, set `--advertise
10.0.0.24:6051` to the address reachable by `buildworld-server`.

Every remote execution also carries a structured `bytemsg233` protocol envelope
(name, major/minor version, and optional capabilities). Workers reject another
major version before executing any node, and the server rejects a response that
does not identify itself as the same contract. The same
`workers.enrollment_token` is attached as gRPC dispatch metadata, so an exposed
worker port cannot execute a pipeline without the server's shared enrollment
credential. Keep this token non-empty in every remote-worker deployment.

## Shared notifications

Notification channels are global and reusable across projects. Supported channel
types are `feishu`, `discord`, `wecom`, `telegram`, `email`, and generic
`webhook`. Configure channel conditions for `running`, `success`, or `failed` to
receive `build.started` and `build.completed` events. Feishu uses interactive
cards, Discord uses embeds, WeCom uses its markdown renderer, and Telegram uses
HTML-formatted messages.

## GitHub Actions trigger

Configure `automation.github_webhook_secret` and point a GitHub repository
webhook to `/api/webhooks/github`. A push still starts the matched Buildworld
project. When its commit message includes `[buildworld:restart]`, the server also
starts the configured `automation.dev_restart_command`. The incoming commit only
selects the marker; it cannot provide a command to execute.

---
sidebar_position: 4
---

# Pipeline Guide

BuildWorld accepts a declarative TypeScript pipeline or GitHub Actions-style
YAML. TypeScript is recommended for people and AI agents: Monaco provides
comments, completion, syntax diagnostics, and live server validation before the
configuration can be saved.

The TypeScript file is parsed as data by an AST allowlist. It is not executed as
JavaScript. Dynamic code (`eval`, `Function`, functions, loops, timers,
promises, Node.js APIs, file access, and network access) is rejected.

## TypeScript pipeline

```typescript
import {
  definePipeline,
  parameter,
  shell,
  stage,
  trigger,
} from '@buildworld/pipeline'

export default definePipeline({
  name: 'my-app-build',
  environment: {
    NODE_ENV: 'production',
  },
  parameters: [
    parameter('environment', 'choice', {
      choices: ['staging', 'production'],
      default: 'staging',
      required: true,
    }),
  ],
  triggers: [
    trigger('vcs', { branch: 'main' }),
  ],
  stages: [
    stage('Checkout', [
      shell('Checkout', 'git clone "$GIT_REPO_URL" .'),
    ]),
    stage('Build', [
      shell('Install', 'npm ci'),
      shell('Compile', 'npm run build'),
    ]),
    stage('Test', [
      shell('Test', 'npm test'),
    ], { dependsOn: ['Build'] }),
  ],
  post: {
    always: [
      shell('Finish', "echo 'Build finished'"),
    ],
  },
})
```

Use the same `@buildworld/pipeline` declarations distributed in
`sdk/pipeline/index.d.ts` for external editors and AI agents.

## Long-running service logs

`watchService` implements the Jenkins `tail -f` pattern without embedding an
unbounded shell loop in the configuration parser. The build remains running,
new log bytes stream continuously, a heartbeat is printed periodically, and
the build fails with the final log lines if the PID exits. The watcher makes
the pipeline long-running automatically; an explicit pipeline or stage
`timeoutSec` still applies. It reconnects when the log is missing, truncated,
or replaced. Use `initialLines: 0` to skip existing history and follow only new
bytes.

`pollSeconds` controls only PID liveness checks. Log following runs on an
independent near-real-time cadence and immediately schedules catch-up reads
while bytes remain, so a Jenkins monitor's `sleep 5` does not delay or throttle
its former `tail -f` stream.

Live output is sent over WebSocket before durable persistence. BuildWorld
batches SQLite writes and retains the newest 1,000,000 characters per build so
an intentional observer cannot grow the control database forever. If older
history is removed, the viewer and downloaded file contain a visible
`[buildworld]` truncation marker; download responses also set
`X-BuildWorld-Log-Truncated` and
`X-BuildWorld-Log-Retention-Characters`.

```typescript
import { definePipeline, shell, stage, watchService } from '@buildworld/pipeline'

export default definePipeline({
  name: 'Game server',
  stages: [
    stage('Start', [
      shell('Start server', './start-server.sh'),
    ]),
    stage('Observe', [
      watchService('Live server log', {
        targetDir: '/srv/game-server',
        pidFile: 'game-server.pid.txt',
        logFile: 'logs/server.log',
        port: 10101,
        heartbeatSeconds: 30,
        pollSeconds: 5,
        initialLines: 30,
      }),
    ]),
  ],
})
```

Canceling the build stops only the observer. It does not terminate the service
process. The PID and log files must resolve inside `targetDir`.

## YAML pipeline

```yaml
name: my-app-build

env:
  NODE_ENV: production

jobs:
  build:
    runs-on: [linux, amd64]
    steps:
      - uses: actions/checkout@v4
      - name: Install
        run: npm ci
      - name: Compile
        run: npm run build

  test:
    needs: build
    if: success()
    timeout-minutes: 15
    steps:
      - name: Test
        run: npm test
        working-directory: .
```

`needs` accepts one job or a list. BuildWorld validates dependencies, rejects
cycles, and runs jobs in dependency order. `actions/checkout` is the supported
built-in `uses` action; unknown actions are rejected instead of downloading and
executing third-party code.

## Helper API

| Helper | Purpose |
| --- | --- |
| `definePipeline(config)` | Define the pipeline and global policies. |
| `stage(name, steps, options?)` | Create a stage; `steps` can be one step or an array. |
| `step(name, type, command, options?)` | Create an explicit step. |
| `shell`, `script`, `tail` | Create command steps. |
| `git(name, options?)` | Create the controlled Git step. |
| `notify(name, options?)` | Create the native notification step. |
| `watchService(name, options)` | Follow a service PID and log continuously. |
| `trigger(type, config?)` | Declare a build trigger. |
| `parameter(name, type, options?)` | Declare a typed build parameter. |

The complete human/agent reference is served by a running BuildWorld instance
at `/script-api.html`. It is intentionally omitted from application navigation
and marked `noindex`.

## Next steps

- [Configuration](./configuration.md) - Configure variables and settings
- [Jenkins migration on macOS](./jenkins-migration-macos.md) - Translate and verify Jenkins jobs
- [Agents](./agents.md) - Configure build agents

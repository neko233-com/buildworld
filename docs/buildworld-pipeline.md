# BuildWorld Pipelines

BuildWorld v1 accepts exactly two pipeline source formats:

- restricted TypeScript DSL (recommended);
- GitHub Actions-style YAML with a non-empty `jobs` map and job-local `steps`.

JSON, Markdown, and legacy top-level `stages` YAML are rejected. Validation runs
before save, template use, scheduling, or queueing.

## TypeScript DSL

TypeScript gives people and AI agents comments, completion, diagnostics, and a
single typed API from `sdk/pipeline/index.d.ts`.

```typescript
import {
  definePipeline,
  parameter,
  shell,
  stage,
  trigger,
  watchService,
} from '@buildworld/pipeline'

export default definePipeline({
  name: 'game-server',
  environment: { NODE_ENV: 'production' },
  parameters: [
    parameter('environment', 'choice', {
      choices: ['staging', 'production'],
      default: 'staging',
      required: true,
    }),
  ],
  triggers: [trigger('vcs', { branch: 'main' })],
  stages: [
    stage('Build', [
      shell('Install', 'npm ci'),
      shell('Compile', 'npm run build'),
    ]),
    stage('Observe', [
      watchService('Follow server log', {
        targetDir: '/srv/game-server',
        pidFile: 'server.pid',
        logFile: 'server.log',
        port: 8700,
        heartbeatSeconds: 900,
        pollSeconds: 5,
        initialLines: 0,
      }),
    ], { dependsOn: ['Build'] }),
  ],
})
```

Source is parsed into declarative data through an AST allowlist. BuildWorld does
not run it as JavaScript. `eval`, `Function`, user-defined functions, loops,
timers, promises, Node.js APIs, file access, and network access are rejected.
Shell commands run only when explicitly declared in pipeline steps.

`watchService` is the native Jenkins `tail -f` plus PID-monitoring equivalent.
It keeps build running, streams appended bytes, emits low-frequency heartbeats,
and fails with
final log lines when process exits. Canceling build stops observer only, not
deployed service.

## GitHub Actions-style YAML

```yaml
name: game-server

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
```

`needs` accepts one job or a list. BuildWorld validates unknown dependencies,
duplicates, and cycles, then executes jobs in stable dependency order. Only
controlled built-in `uses` actions are accepted; BuildWorld never downloads and
executes an arbitrary third-party action.

## Validation and migration

Web editors validate source continuously through `POST /api/pipeline-validation`.
Save and run stay disabled until server validation succeeds. Response includes
format, stage/step counts, parameters, and long-running status.

Jenkins import translates Jenkinsfiles directly to restricted TypeScript, then
validates translated output. It does not create an intermediate JSON or Markdown
pipeline.

Complete helper reference is served by a running instance at
`/script-api.html`. Public documentation continues at
`docs-site/docs/pipelines.md` and `docs-site/docs/typescript-pipelines.md`.

# BuildWorld Pipelines

BuildWorld accepts the following pipeline source formats:

- restricted TypeScript DSL (recommended);
- GitHub Actions-style YAML with a non-empty `jobs` map and job-local `steps`;
- supported declarative Jenkinsfile pipelines, including SCM parameter metadata.

JSON, Markdown, and legacy top-level `stages` YAML are rejected. Validation runs
before save, template use, scheduling, or queueing.

## Release channel and environment

Release pipelines declare public `RELEASE_CHANNEL` choices and a public
`RELEASE_ENVIRONMENT` string or choice parameter. Each scoped release project
also owns a **public, project-scoped** environment variable named
`RELEASE_TARGETS_JSON`. It contains the release tool's controlled, deployable
target matrix, using this data-only schema:

```json
{"schemaVersion":1,"targets":[{"channel":"alpha","environment":"test","channelLabel":"Alpha","environmentLabel":"Test"},{"channel":"alpha","environment":"release","channelLabel":"Alpha"},{"channel":"third-store","environment":"audit-2","channelLabel":"Third store"}]}
```

Both channel and environment codes must match `^[a-z][a-z0-9-]{0,31}$`.
The catalog is at most 64 KiB and contains 1–512 distinct pairs, optional labels up to 80 characters,
and no credentials or other fields. A channel's label must be consistent across
its entries. Obtain the catalog from the release tool's validated configuration
export; retained history does not establish deployment authority. Global or
secret variables cannot supply this catalog. Missing, malformed, or mixed-mode
metadata prevents scoped releases from entering the queue.

The matrix supplies the authoritative channel candidates, replacing the
pipeline's bootstrap choices. It also supplies each channel's environments, so
adding a valid target requires updating controlled configuration and this public
catalog, without changing UI code or the Jenkinsfile's channel enumeration.
Parameter declarations, types, and non-secret requirements remain mandatory.
The downstream release tool must independently validate the full target binding.

The parameterized build form presents searchable, linked selectors. Search
matches labels and codes, with arrow-key navigation, Enter selection, Escape
close, and normal Tab focus movement. The initial channel uses a valid pipeline
default or the first configured channel. Its environment prefers configured
`test`, then a valid pipeline default, then its first configured environment.
Only exact configured pairs are accepted; the matrix does not create a Cartesian
product. A historical shortcut retains its exact channel, environment, and
version. Removed targets stay visibly unavailable and cannot be submitted.
Changing the target clears the historical version to prevent restarting the
previous target's version in a different target.

For example, a Jenkinsfile can declare:

```groovy
parameters {
    choice(name: 'RELEASE_CHANNEL', choices: ['wechat', 'taptap'], description: 'Release channel')
    string(name: 'RELEASE_ENVIRONMENT', defaultValue: 'test', description: 'Configured release environment')
    choice(name: 'RELEASE_ACTION', choices: ['restart-version', 'publish-latest'], description: 'Release action')
    string(name: 'RELEASE_VERSION', defaultValue: '', description: 'Required for historical version restart')
    booleanParam(name: 'CONFIRM_PUBLISH_LATEST', defaultValue: false, description: 'Explicit latest publication confirmation')
}
```

The form reads parameter declarations and the validated catalog from
`POST /api/projects/{id}/validate`, which returns `release_targets` and
`release_targets_required` with the ordinary pipeline metadata. Configure the
public catalog before enabling this contract on an existing scoped project.
API callers use `POST /api/projects/{id}/builds` with:

```json
{"branch":"main","parameters":{"RELEASE_CHANNEL":"wechat","RELEASE_ENVIRONMENT":"test","RELEASE_ACTION":"restart-version","RELEASE_VERSION":"0.1.234"}}
```

API creation, token triggers, historical retries, startup builds, and queued
execution validate the selected pair against current project metadata. Removing
a target also invalidates pending or retried requests for it. Validated release
parameters remain authoritative when shell environments merge pipeline, global,
project, stage, and step settings; configured variables cannot substitute another target,
action, version, or latest-publication confirmation. Ordinary Jenkins environment
precedence is unchanged.

The project page shows every target recorded in the retained build history,
including the latest attempt and last successful pipeline completion time.
`GET /api/projects/{id}/release-targets` exposes only public target names and
bounded build summaries, without logs, credentials, or other parameters. It
continues to work when SCM is unavailable. The history query accepts
`release_channel` and `release_environment` filters and applies them before
pagination, including builds older than the project's recent history window.

Only records with both explicit string parameters are classified. Legacy or
incomplete records remain unclassified. A successful publication requires
`status=success` and a valid completion time; a newer failed, queued, or running
attempt does not overwrite that target's last successful publication. Statistics
refer to the whole release pipeline's result and do not independently prove
application health or user traffic. A pipeline must report failure if its
deployment or required health verification fails.

`restart-version` requires a non-empty `RELEASE_VERSION`. `publish-latest`
requires the literal boolean `CONFIRM_PUBLISH_LATEST=true`; all trigger paths
share this validation. The browser hides the raw confirmation flag and asks
for a second confirmation showing the selected channel, environment, branch,
replacement, and service restart impact before it sends that flag. Replaying
an older latest-publication build is rejected because its earlier confirmation
must not authorize another latest publication. Start a new parameterized build
to confirm the new operation.

Shared physical SDK/Gateway pipelines declare `RELEASE_SHARED_SERVICE` as a
public `string`, with an explicit configured service ID matching
`^[a-z][a-z0-9-]{0,31}$`. This is a separate target contract: such pipelines must
not also declare or submit `RELEASE_CHANNEL` or `RELEASE_ENVIRONMENT`, including
blank values. A service ID is mandatory even when the pipeline default is
missing; an undeclared request parameter never selects shared mode. The same
latest-publication confirmation and historical-version guards apply to both
contracts. Shared component builds retain ordinary project build history and
do not fabricate per-game channel/environment publication records. Shared jobs
do not require `RELEASE_TARGETS_JSON` and reject a scoped catalog declaration;
their generic service-ID form remains separate from channel/environment selectors.

```json
{"branch":"main","parameters":{"RELEASE_SHARED_SERVICE":"sf-minigame-shared","RELEASE_ACTION":"publish-latest","CONFIRM_PUBLISH_LATEST":true}}
```

The declaration selects the target mode before queueing. This avoids treating
a physical shared service as one game target while keeping incomplete or
mixed declarations fail-closed; target membership remains authoritative in
the release tool's controlled service profile.

After successful deployment and required verification, the release tool should
emit one complete public marker line:

```text
::buildworld:release-result {"channel":"wechat","environment":"test","action":"publish-latest","version":"0.1.234","branch":"release/wechat","component":"game-server"}
```

BuildWorld persists these results separately from bounded logs. The marker's
channel, environment, and action must match the persisted request parameters;
a historical restart's version must match too. The branch belongs to the actual
release artifact and is never inferred from BuildWorld's SCM checkout branch.
Multiple components can be recorded independently. Version shortcuts are only
shown after a successful completed build with a non-ambiguous version result
(or an explicit historical restart version). Legacy records with no version
evidence do not gain an inferred version. Shortcuts prefill the selected target
and version on the parameter page; the operator still explicitly queues it.

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

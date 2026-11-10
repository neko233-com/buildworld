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
{"schemaVersion":1,"targets":[{"channel":"alpha","environment":"test","channelLabel":"Alpha","environmentLabel":"Test","appId":"alpha-test-id"},{"channel":"alpha","environment":"release","channelLabel":"Alpha","appId":"alpha-release-id"},{"channel":"third-store","environment":"audit-2","channelLabel":"Third store","appId":"third-store-audit-id"}]}
```

Both channel and environment codes must match `^[a-z][a-z0-9-]{0,31}$`.
The catalog is at most 64 KiB and contains 1–512 distinct pairs, optional labels up to 80 characters,
and an optional public `appId` up to 128 characters per target. AppIds are display metadata; they do not
replace the protected channel credentials or determine runtime login routing. When a project uses
different platform identities for each target, assign each pair its corresponding distinct AppId.
The catalog contains no credentials or other fields. A channel's label must be consistent across
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

The project-specific HTTP trigger accepts the same parameter object in its
POST body. Keep its generated opaque URL private; callers cannot use this
endpoint to bypass declared parameter validation or the configured release
target catalog:

```http
POST <project-http-trigger-url>
Content-Type: application/json

{"branch":"main","parameters":{"RELEASE_CHANNEL":"taptap","RELEASE_ENVIRONMENT":"test","RELEASE_ACTION":"publish-latest","CONFIRM_PUBLISH_LATEST":true}}
```

The AppId is resolved from the selected controlled target and is not a trigger
parameter. This keeps the displayed identity, server-side credentials, and
published target bound together.

API creation, token triggers, historical retries, startup builds, and queued
execution validate the selected pair against current project metadata. Removing
a target also invalidates pending or retried requests for it. Validated release
parameters remain authoritative when shell environments merge pipeline, global,
project, stage, and step settings; configured variables cannot substitute another target,
action, version, or latest-publication confirmation. Ordinary Jenkins environment
precedence is unchanged.

The project page combines the configured target catalog with retained build history,
showing each target's optional AppId, latest attempt, and last successful pipeline completion time.
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

Scoped pipelines may declare `RELEASE_BRANCH` as an optional public string.
The parameter page then selects completed successful release records from
`GET /api/projects/{id}/release-records`, filtered by the configured channel and
environment. Records use `release/<channel>/<environment>/<component>/<version>`;
the engine checks the exact target and restart action and derives `RELEASE_VERSION`.
Changing the target clears both values. Tooling is checked out from `main`; the
selected release branch identifies the historical artifact, not the pipeline source.
The deployment tool still checks the published artifact and exact Git record before
any mutation, so a catalog entry alone never authorizes an unavailable artifact.

Result markers also carry `sourceCommit`, `configCommit`, `configSnapshotId`,
`configVersion`, `sdkVersion`, and `gatewayVersion`. Configuration identity comes
from the packaged snapshot and its fixed commit marker, including on historical
restarts; it is never refreshed from the latest configuration repository. The
project view and branch selector display that recorded identity. Legacy results
without these fields remain explicitly unknown. Migration 15 preserves existing
results and removes the historical `codex/` release prefix. Successful completed
records are deduplicated by branch; failed or running builds never replace them.

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

Jenkins `when` supports `allOf`, `anyOf`, `not`, branch and tag patterns,
`buildingTag()`, `changeset`, `changelog`, environment equality (including literal
`ignoreCase`), `equals`, `changeRequest`, and a side-effect-free expression subset.
Branch and tag patterns support `GLOB`, `EQUALS` and `REGEXP` comparators; Go's
regular-expression syntax is narrower than Java's. Expressions support `params`
and `env`, `currentBuild.number`, boolean and ordered comparisons, equality and
`toString().toBoolean()`; numeric parameters keep JSON precision, while nonempty
strings are true under Groovy truth. Pipeline environment is visible to `when`;
the stage's own environment applies after its condition succeeds. `branch`
uses `BRANCH_NAME`, falling back to `GIT_LOCAL_BRANCH` when it is empty. For tag
pushes and explicitly selected `refs/tags/...` refs on a configured repository,
the short tag name is exposed as `TAG_NAME` and `BRANCH_NAME`; `GIT_COMMIT` is
the pinned commit, with annotated tag objects verified and peeled. Inline jobs
retain their existing workspace setup. Other tag conditions require a populated
`TAG_NAME` in the build environment.

BuildWorld's explicit `::buildworld:set NAME=value` output markers update the
environment seen by later stages and their `when` conditions. Stage-local
environment declarations do not leak into later conditions.

`changeset` and `changelog` use the persisted changes from the build's actual
SCM checkout, compared with the previous observed checkout of the same project,
repository and branch. The first observed checkout has no changes. With
`skipDefaultCheckout()`, conditions before explicit `checkout scm` see no
changelog. If earlier builds exist but their checkout history is unavailable,
the condition fails instead of assuming an empty history; the current checkout
establishes the baseline for the next build. An SCM build of an explicit
`refs/pull/<number>/head` or `refs/merge-requests/<number>/head` is treated as a
change-request head: `BRANCH_NAME` is `PR-<number>` or `MR-<number>` respectively,
`CHANGE_ID` is the number, and `changeset`/`changelog` inspect
the captured checkouts of earlier builds of that same project, repository and
PR head as well as this build's checkout. Earlier builds' changes remain
visible before this build checks out. Builds without a checkout record that
fact; older or incomplete snapshots fail closed instead of guessing. The total
aggregated history is limited to 16 MiB. Manually supplying `CHANGE_ID` alone
does not identify a PR head. A project may explicitly enable same-repository
PR builds in its trigger settings. Verified GitHub `pull_request`, Gitea
`pull_request` / `pull_request_sync`, and GitLab `Merge Request Hook` deliveries
create builds pinned to the event's head commit; duplicate deliveries remain
idempotent. GitLab update events trigger only when `oldrev` identifies an actual
code change. The event supplies `CHANGE_TARGET`, `CHANGE_BRANCH`, and
`CHANGE_TITLE`; GitLab supplies `CHANGE_AUTHOR` only when the event actor is
also the MR author. External fork requests do not run automatically. Merge-ref
strategies are not implemented.
Git collection excludes merge commits, records at most 1024 commits and fails
if its output exceeds 16 MiB. Regular expressions use Go syntax rather than
Java's complete regular-expression language.

SCM checkout records use database migration 17. Older BuildWorld binaries reject
databases with a newer migration ledger; verify a data-preserving downgrade or
restore procedure before deploying this version. An installation-directory
rollback alone does not roll back the database schema.
The updated system-update helper compares a read-only fingerprint of the
migration ledger before activation and after a failed start. It refuses to
restore the previous binary if that ledger changed, the database cannot be
inspected, or the new process cannot be stopped. This protects the database and
both installations for manual recovery; it does not itself downgrade the schema
or guarantee reversal of external build actions. Safe inspection requires an
absolute `database.path`, because a service and detached updater can have
different working directories. The helper comes from the installed version, so
an older installation does not gain this protection until it is upgraded.

`changeRequest()` evaluates the existing `CHANGE_ID` and related `CHANGE_*`
environment values, including named filters and `EQUALS`, `GLOB` or `REGEXP`
comparators. The opt-in webhook integration described above supplies verified
metadata for supported change requests; other source integrations must supply
their own values.
The [Jenkins implementation](https://github.com/jenkinsci/pipeline-model-definition-plugin/blob/master/pipeline-model-definition/src/main/java/org/jenkinsci/plugins/pipeline/modeldefinition/when/impl/ChangeRequestConditional.java)
currently accepts a `url` filter but does not check it, so BuildWorld rejects
that filter rather than allowing a guard to be silently ignored. Groovy literal escapes are supported in condition strings;
interpolated strings and regular-expression constructs unsupported by Go are
rejected instead of being evaluated as a different condition.

Unsupported conditions and arbitrary Groovy are rejected rather than executed
unconditionally. Literal `beforeOptions`, `beforeInput` and `beforeAgent` are
accepted for the supported local executor; `beforeOptions true` evaluates the
condition before starting a stage timeout. Stage input and specific stage
agents remain unsupported, so their distinct lifecycle behavior is not available.
Early evaluation with root `agent none` is rejected because per-stage agent
allocation is not modeled.
`triggeredBy` supports literal `UserIdCause`, `TimerTrigger`, `SCMTrigger` and
`BuildUpstreamCause` for authenticated manual/API, scheduled, SCM polling and
upstream builds respectively. User detail uses the authenticated username saved
at creation. A historical user build without saved identity fails when detail
is requested. Retry, webhook and other sources fail the condition evaluation
instead of guessing a Jenkins cause; other cause patterns remain unsupported.
Conditional Jenkins stages cannot be dispatched to remote workers.
Re-import previously converted pipelines from the Jenkinsfile so conditions
discarded by older versions are restored.

The migration preview may provide a best-effort conversion with warnings.
Live Jenkinsfile execution refuses semantic conversion warnings, including
unsupported Groovy conditions, directories, omitted stages, and generated
process shutdown policies. Rewrite those constructs in supported steps and
review the resulting behavior before running them. macOS protected-directory
notices remain informational and do not grant operating-system permissions.
Literal `deleteDir()` and `checkout scm` at the start of a stage execute in
order against the per-build workspace; `checkout scm` requires an SCM-sourced
Jenkinsfile and checks out its already-pinned commit. `skipDefaultCheckout()`
and its literal boolean argument control the initial automatic checkout.
Checkout can reuse the configured repository or initialize a nonempty build
workspace. It restores tracked files to the pinned revision and preserves
unrelated untracked files; a foreign repository or external `.git` directory
is rejected instead of being modified.
Workspace operations in other positions or nested Groovy blocks require manual
conversion and are not silently treated as successful.

Stage `options { timeout(...) }` supports positive integral seconds, minutes
(the default), hours and days. Stage-level or step-level `input`, stage `post`,
`parallel`, `matrix`, nested stages, unsupported options and specific stage
agents fail validation rather than being ignored. With root `agent none`, each
supported executable stage must use `agent any`.

SCM build triggers resolve the selected branch and immutable commit before
deriving parameter defaults and approval requirements. The same commit supplies
the Jenkinsfile and workspace, including after recovery or retry. A push webhook
requires a complete SHA-1 or SHA-256 object ID; deletion events do not start a
build. Approval-required builds are held before entering the queue, and an
automatic retry of `publish-latest` requires a new manual confirmation.

Webhook requests authenticate before BuildWorld records a delivery and its
currently matching enabled projects in SQLite. Each target is resolved by one
of 16 background workers; transient failures retry with a bounded delay. Build
creation and the delivery target are committed together, so a worker restart
does not allocate a second build. Interrupted leases expire within two minutes.
Administrators can inspect recent target status and retry counts through
`GET /api/webhook-deliveries`. GitHub does not automatically redeliver requests
that fail before BuildWorld accepts them; arrange redelivery at the provider.

Delivery identity is required: GitHub `X-GitHub-Delivery`, Gitea
`X-Gitea-Delivery`, or GitLab `webhook-id` / `Idempotency-Key` (GitLab 17.4+
for the latter). Reusing one identity with a different body is rejected.
Deduplication covers the same provider delivery ID; a provider that issues a
new ID for a redelivery presents a new event. It does not infer identity from
commit hashes, because a legitimate later push can revisit the same commit.

New installations default to 16 concurrent builds and 16 builtin executor
slots. Both limits are configurable independently; the effective builtin
capacity is the lower of the two. Existing saved settings are preserved.

Complete helper reference is served by a running instance at
`/script-api.html`. Public documentation continues at
`docs-site/docs/pipelines.md` and `docs-site/docs/typescript-pipelines.md`.

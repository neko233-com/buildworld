# Changelog

All notable BuildWorld changes are recorded here. The project follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.17.1] - 2026-11-13

### Fixed

- Support worker operation request identities on HTTP intranet origins and retain operation errors through snapshot polling.
- Preserve nested quotes in Windows command programs so remote steps execute instead of echoing script text.

## [1.17.0] - 2026-11-13

### Added

- Add authenticated worker runtime diagnostics, tracked execution, durable operation receipts, drain/resume and live concurrency management.
- Add manual verified worker release updates with new-process readiness and automatic binary rollback, independent worker identity and metadata.

### Fixed

- Enforce worker-side capacity and preserve reused repository/branch workspaces with full-build OS ownership locks.
- Keep worker management forms stable through polling and show missing runtime evidence explicitly.
- Resolve the trusted Windows command interpreter independently of repository files.

## [1.16.38] - 2026-11-12

### Added

- Protect administrator login with optional TOTP and bounded failed-login attempts.
- Preserve idempotent build identities and provide authenticated read-only lookup for uncertain submissions.
- Record selected-server release receipts independently from complete-environment release history.
- Guard prestart builds against pipeline identity changes and unmasked task secrets.

### Fixed

- Prevent partial server releases from appearing as complete-environment restarts in build history.

## [1.16.37] - 2026-11-11

### Added

- Allow administrators to select audited, in-app-only BuildWorld notifications
  for an individual build and its retries without changing global channels.

## [1.16.36] - 2026-11-11

### Fixed

- Run Windows `pwsh` steps with PowerShell Core, preferring the native
  64-bit executable in PATH while retaining 32-bit-only installations.

## [1.16.35] - 2026-11-11

### Fixed

- On Windows, prefer the Bash beside an installed POSIX shell over the
  system WSL launcher when the launcher would otherwise shadow it.

## [1.16.34] - 2026-11-11

### Fixed

- Support bounded lookaround and backreference matching in Jenkins branch,
  tag, change-request, changeset and changelog regex conditions.

## [1.16.33] - 2026-11-11

### Added

- Run supported Jenkins stage-level `post` conditions in the local executor,
  including `aborted` for cancellation and Jenkinsfile stage timeouts.

### Fixed

- Keep stage `post` steps within their stage environment, skip them when its
  `when` condition is false, and preserve conditional artifact collection.
- Classify Jenkinsfile stage timeouts as cancelled, matching Jenkins ABORTED.

## [1.16.32] - 2026-11-11

### Fixed

- Preserve the order of supported shell, archive, and workspace cleanup steps
  mixed in a Jenkins pipeline-level post condition; reject unsupported syntax.
- Stop the current post condition after a step fails while still evaluating
  subsequent failure and cleanup conditions.

## [1.16.31] - 2026-11-11

### Fixed

- Keep each builtin build's temporary directory outside its workspace so
  Jenkins `deleteDir` and `cleanWs` cannot break subsequent Go and other steps.
- Clean that build's temporary directory when the build finishes.

## [1.16.30] - 2026-11-11

### Added

- Run supported Jenkins pipeline-level post archives and workspace cleanup in
  condition order, preserving artifact paths and recording SHA-256 checksums.

### Fixed

- Fail a successful build when a post step fails instead of reporting success.

## [1.16.29] - 2026-11-11

### Fixed

- Keep a slow SCM fetch from blocking all other builds in the central queue.
- Resolve pinned branch builds with a shallow clone and fetch the exact
  requested commit when necessary.

## [1.16.28] - 2026-11-11

### Fixed

- Avoid directly terminating a supervised macOS server and report the
  launchctl failure when its active LaunchAgent cannot be unloaded.
- Permit authenticated updates of a system-managed macOS service only through
  the verified, restricted administrator-installed supervisor bridge; reject
  unsafe service identities and report a healthy unchanged service accurately
  on rollback.
- Resolve SCM build parameters and approval rules from the selected immutable
  revision, including webhook, startup, scheduled, and retry paths. Preserve
  full webhook object IDs and hold approval-required builds before queueing.
- Require a fresh confirmation for `publish-latest` instead of replaying it
  through automatic retry.
- Allocate per-project build numbers atomically for concurrent triggers and
  retries, including simultaneous approval-required builds.
- Preserve declarative stage timeouts, evaluate Jenkins `equals` and numeric
  comparisons without losing integer precision, and reject unsupported stage
  approval, agent, post and nested execution rather than silently bypassing them.
- Carry configured-repository tag refs through signed GitHub, Gitea and GitLab push webhooks,
  verify and peel annotated tag objects, and expose their pinned commit and
  tag name consistently across stage conditions and restart recovery.

## [1.16.27] - 2026-11-11

### Fixed

- Preserve the selected branch name while checking out an immutable SCM
  revision, so existing Jenkinsfiles can verify their branch and commit.

## [1.16.26] - 2026-11-11

### Added

- Package and deploy the controller from its own Jenkinsfile, with an exact
  source commit and restart-safe system update reconciliation.
- Report the packaged source commit alongside the running release version.

### Changed

- Run native release verification and six-platform packaging on macOS and
  Linux as well as Windows, while keeping production port 8080 fixed.
- Pin SCM pipeline definitions and workspaces to the same Git commit, including
  after a controller restart.

### Fixed

- Preserve older rollback snapshots and stop activation when an update step
  fails; restore the previous service when activation cannot complete.
- Evaluate supported nested Jenkins conditions, branch and tag comparators,
  environment values and Groovy-style boolean conversions without silently
  running unsupported conditions.

## [1.16.25] - 2026-11-10

### Fixed

- Preserve supported Jenkins boolean `when` expressions and reject unsupported
  conditions instead of silently running gated stages.
- Find GNU tar in current Git for Windows layouts for release packaging.

## [1.16.24] - 2026-10-18

### Added

- Project purpose descriptions can be entered during project creation and are
  shown beneath project names on the dashboard.

## [1.16.23] - 2026-10-15

### Added

- English-first README, Chinese README, and bilingual HTML documentation with
  an explicitly dated historical local build-details screenshot.
- A Chinese installation guide covering PowerShell, all six platform bundles,
  multipart downloads, SHA-256 verification, user PATH, and service lifecycle.
- A checksummed `release-info.json` in every bundle and as a Release asset,
  recording the version, source commit, Go toolchain, build time, and platforms.
- Isolated release runtime verification of CLI/server versions, health,
  production Web UI, sign-in, and a real pipeline on the `builtin` executor.

### Changed

- Public HTML documentation publishes locally only for new release tags.
  Ordinary main pushes and replacement releases do not publish the site.
- Release notes come from this changelog. Packaging requires a clean source
  tree and rejects source changes during the build.
- README pipeline examples use the current restricted TypeScript DSL.

### Fixed

- Broken localized documentation links now fail verification.
- Retrying an interrupted Pages deployment verifies the existing static commit
  without rebuilding HTML or pushing a duplicate documentation commit.
- Unix release verification no longer requires `gzip.exe` on the packaging
  host PATH; it verifies decompressed SHA-256 and executable modes directly.
- Remote read/fetch checks retry transient EOF, TLS handshake interruptions,
  and gateway failures up to three times before reporting the original error.

## [1.0.0] - 2026-07-21

First production release.

### Added

- TypeScript pipeline DSL with Monaco validation, completion, comments, and a
  restricted execution model that rejects dynamic evaluation.
- GitHub-Actions-style YAML pipelines, Jenkinsfile migration, and migration
  validation without queuing a build.
- Durable local and distributed builds, Jenkins-like live log following,
  service log monitoring, and build-history retention.
- One-level project folders with accessible pastel color presets.
- Local-only verification, documentation publication, six-platform release
  packaging, checksum-verified installers, CLI lifecycle management, and
  per-user autostart.

### Changed

- The control-plane port is `8700`; Vite development and hot reload use `8701`.
- Navigation uses “Builds in Progress”, “Build History”, “VCS Repository
  Templates”, and “Build Templates”.
- Repository triggers use signed GitHub, GitLab, and Gitea provider webhooks.
  Placeholder deployment-status and per-project plaintext Git-hook features are
  not part of the 1.0.0 product surface.
- Repository SSH keys live only in VCS credentials; the unused per-user key
  registry and server-side key-generation helpers were removed.
- CI, documentation, packaging, and release uploads run locally; GitHub Actions
  is not used.

### Fixed

- Database migration v7 removes orphaned build-queue and project-statistics
  rows, while preserving historical notification events with missing build
  references set to `NULL`.

### Security

- Pipeline TypeScript is parsed as a restricted DSL and does not use `eval` or
  `new Function`.
- Release installers verify SHA-256 checksums before replacing an installation.

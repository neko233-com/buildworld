# Changelog

All notable BuildWorld changes are recorded here. The project follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

No unreleased changes.

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

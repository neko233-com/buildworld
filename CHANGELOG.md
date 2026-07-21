# Changelog

All notable BuildWorld changes are recorded here. The project follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

No unreleased changes.

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

# Engineering rules

## Verification scope

- Do not automatically install, discover, or run Unity or Tuanjie editors.
- Keep Unity/Tuanjie projects as fixtures only. Their builds are opt-in manual
  checks using the documented PowerShell or shell commands.
- For application changes, run the relevant Go and Web test suites, then build
  the Web bundle when the UI changes.

## Production interaction quality

- User actions must have clear loading, success, and failure feedback.
- Destructive actions require confirmation; keyboard navigation, focus
  restoration, and error states must remain accessible.
- Validate real rendered flows after non-trivial UI changes; a successful type
  check alone is not sufficient.

## Default networking

- BuildWorld's default control-plane HTTP port is `8080`; retained Jenkins
  listens on `8081`. The Vite development server uses `8701` and proxies to
  that control plane to avoid a local port collision. This intentional cutover
  exception is documented and covered by configuration tests.

## Release and installation contract

- GitHub Actions is not used for CI, notifications, documentation, packaging,
  or uploads. Run every verification, build, and publication step locally; do
  not add or enable cost-triggering workflow files.
- Build documentation locally and publish the verified static output from the
  repository's `gh-pages` branch with `scripts/publish-docs-local.ps1`. The
  script must remain dry-run by default and require an explicit, confirmed
  `-Publish` before any remote write. The publish path must rerun
  `scripts/verify-local.ps1` and verify the exact Pages build commit.
- Public documentation HTML is built and published only for a new `vX.Y.Z`
  release tag by the local release publisher. Ordinary `main` pushes, staging
  tags, and replacement releases must not publish documentation. The docs
  publisher requires `-ReleaseTag`, verifies its remote commit against the
  reviewed source, records the published tag and source commit, skips an
  already published tag, and rejects tag rewrites or version downgrades.
  Explicit local documentation builds remain available for verification.
- Build and upload release binaries locally. Documentation and release uploads
  are forbidden while applicable local verification is incomplete or failing.
- A release bundle must include the CLI, server, worker, and `web/dist` for
  Windows amd64/arm64, Linux amd64/arm64, and macOS amd64/arm64.
- Packaging requires a clean committed source tree. Every bundle and Release
  includes a checksummed `release-info.json` recording version, source commit,
  Go toolchain, build time, and supported platforms. Reject source changes
  during packaging and any metadata/tag source mismatch before upload.
- Before packaging on the Windows release host, run the isolated runtime gate
  in `scripts/verify-runtime-local.ps1`: CLI/server versions, health, Web UI,
  login, and a real `builtin` pipeline. Keep its state and PID isolated from
  existing services. This does not establish Linux/macOS runtime acceptance
  or deploy any production instance.
- One-click PowerShell and shell installers verify SHA-256 checksums, install
  the CLI into the user environment PATH, and enable silent per-user autostart
  unless explicitly disabled.
- CLI lifecycle commands (`start`, `pause`, `resume`, `restart`, `status`) and
  `reset-root-password --password-stdin` must remain functional and documented.

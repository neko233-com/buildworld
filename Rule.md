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

- BuildWorld's default control-plane HTTP port is `8700`. The Vite development
  server uses `8701` and proxies to that control plane to avoid a local port
  collision. Document and test any intentional exception explicitly.

## Release and installation contract

- GitHub Actions is not used for CI, notifications, documentation, packaging,
  or uploads. Run every verification, build, and publication step locally; do
  not add or enable cost-triggering workflow files.
- Build documentation locally and publish the verified static output from the
  repository's `gh-pages` branch with `scripts/publish-docs-local.ps1`. The
  script must remain dry-run by default and require an explicit, confirmed
  `-Publish` before any remote write. The publish path must rerun
  `scripts/verify-local.ps1` and verify the exact Pages build commit.
- Build and upload release binaries locally. Documentation and release uploads
  are forbidden while applicable local verification is incomplete or failing.
- A release bundle must include the CLI, server, worker, and `web/dist` for
  Windows amd64/arm64, Linux amd64/arm64, and macOS amd64/arm64.
- One-click PowerShell and shell installers verify SHA-256 checksums, install
  the CLI into the user environment PATH, and enable silent per-user autostart
  unless explicitly disabled.
- CLI lifecycle commands (`start`, `pause`, `resume`, `restart`, `status`) and
  `reset-root-password --password-stdin` must remain functional and documented.

---
sidebar_position: 3
---

# Local verification and publishing

BuildWorld does not use GitHub Actions. CI checks, documentation builds,
GitHub Pages publication, six-platform packaging, and Release uploads run on a
controlled local machine.

## Prerequisites

- Node.js 24 and npm;
- Git and PowerShell;
- GitHub CLI authenticated with `gh auth login`;
- admin or maintainer access to `neko233-com/buildworld`.

GitHub Pages is public even while the repository is private. Never put secrets,
private hostnames, tokens, or credentials in the documentation source.

## Build-only dry run

The default command installs locked dependencies, builds every configured
locale, checks the required entry points, and rejects symbolic links. It does
not fetch, commit, push, or change GitHub settings.

```powershell
.\scripts\publish-docs-local.ps1
```

## Publish

First run the canonical local gate, review and commit the source, push that
commit to `main`, and leave the worktree clean. Then run:

```powershell
.\scripts\verify-local.ps1
.\scripts\publish-docs-local.ps1 -Publish
```

Publication has a high-impact confirmation prompt. After confirmation, the
script reruns the same local gate, accepts only the expected GitHub fetch and
push URLs, verifies that local `HEAD` equals `origin/main`, creates an isolated
temporary worktree, replaces its contents with `docs-site/build`, adds
`.nojekyll`, and pushes one static-site commit to `gh-pages`. It then configures
GitHub Pages for legacy branch publication from `gh-pages:/` and waits for a
successful Pages build whose commit matches that publication.

GitHub can show a platform-managed Pages deployment record after the branch
push. This does not run a repository workflow or build the documentation on a
GitHub runner; `.nojekyll` makes the service publish the locally built static
files directly.

The current source branch and index are never switched or rewritten. A normal
push is used, so a concurrent `gh-pages` update is rejected instead of being
force-overwritten. Temporary worktree cleanup runs even when publication
fails.

Use PowerShell's standard preview mode to exercise the documentation build and
confirmation boundary without reading or changing Git/GitHub publication
state. Dependency installation can still access the configured npm registry:

```powershell
.\scripts\publish-docs-local.ps1 -Publish -WhatIf
```

Do not use `npm run deploy`, manually force-push `gh-pages`, or add an Actions
workflow as an alternate publishing path.

## Local Release publication

The Release publisher is also dry-run by default. It runs the complete local
gate, creates all six platform bundles, and verifies every SHA-256 entry without
writing GitHub state:

```powershell
.\scripts\publish-release-local.ps1 -Version 1.0.0
```

After the reviewed commit is on `origin/main` and required production checks
pass, publish the replacement `v1.0.0` and remove older Release objects with:

```powershell
.\scripts\publish-release-local.ps1 -Version 1.0.0 -Publish -ReplaceExisting -PruneOtherReleases
```

This requires a high-impact confirmation. The script creates a draft, uploads
assets in local batches, compares every remote name and size, and only then
publishes it as Latest. `-PruneOtherReleases` deletes old Release binaries but
keeps their source tags. GitHub Actions is never used.


---
sidebar_position: 4
---

# Local verification and release-tag publishing

BuildWorld runs verification, documentation builds, six-platform packaging,
and Release uploads locally. It does not use GitHub Actions.

**Only a new `vX.Y.Z` release tag triggers public HTML documentation building
and publication.** An ordinary `main` push, staging tag, or replacement Release
does not publish GitHub Pages. Explicit local preview builds remain available.

## Prerequisites

- Node.js 24 and npm;
- Git and PowerShell 7 (`pwsh`);
- GitHub CLI authenticated with `gh auth login`;
- maintainer access to `neko233-com/buildworld`;
- all required local verification and production checks completed.

Documentation is public. Keep secrets, credentials, tokens, and private
hostnames out of source text and screenshots.

## Preview without publication

```powershell
.\scripts\publish-docs-local.ps1
```

The dry run installs locked dependencies, builds English and Chinese HTML,
checks entry points and screenshot assets, and rejects symbolic links. It does
not read remote publication state, fetch, commit, push, or change Pages settings.
`main` pushes do not invoke this command automatically.

## Publish a new release and its documentation

Review the source, run `.\scripts\verify-local.ps1`, commit to `main`, and push
the reviewed commit. Keep the worktree clean. Replace `X.Y.Z` below with the
new release version:

```powershell
.\scripts\publish-release-local.ps1 -Version X.Y.Z -Publish
```

After confirmation, the local publisher builds all six platform bundles,
uploads to a staging draft, verifies asset sizes and SHA-256 digests, then
creates the final release tag and publishes the Release. Only when the final
tag is new does it call the documentation publisher with `-ReleaseTag`.
Replacing an existing Release with `-ReplaceExisting` skips documentation.

The documentation publisher verifies that the remote release tag resolves to
the reviewed `HEAD` on `main` and that `HEAD` equals `origin/main`. It rejects
older versions and rewritten tags, and skips an already published tag. It
reruns the full local gate before uploading any generated HTML.

An isolated temporary worktree receives the verified static output and
`.nojekyll`. The publisher records `releaseTag` and `sourceCommit` in
`.buildworld-release.json`, pushes normally to `gh-pages`, and verifies that
GitHub Pages reports the exact new Pages commit as built. Concurrent branch
updates fail rather than being overwritten.

GitHub may display a platform-managed Pages deployment record. The service
serves the locally generated files from `gh-pages:/`; no repository workflow
builds the documentation on a GitHub runner.

## Recover the documentation stage

If the Release succeeds but documentation fails, the error identifies the
documentation stage separately. Keep the new tag and its source commit intact.
From the clean, reviewed source commit, retry with its explicit tag:

```powershell
.\scripts\publish-docs-local.ps1 -ReleaseTag vX.Y.Z -Publish
```

This is recovery for the same new release, not a publishing path for `main`.
After a successful publication, retrying the same tag skips HTML building and
pushes. If the static commit was already submitted but Pages has not confirmed
it as built, the same command retries that deployment and verifies its exact
commit without rebuilding HTML or pushing another static commit.

To inspect eligibility without building or writing remote state, use
`-ReleaseTag vX.Y.Z -Publish -WhatIf`. This reads remote state. The default
dry run above builds locally without reading remote publication state.

Do not use `npm run deploy`, force-push `gh-pages`, or add an Actions workflow
as an alternate publishing path.


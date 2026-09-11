---
sidebar_position: 3
---

# System updates

BuildWorld supports explicit, authenticated bundle updates on macOS and Linux.
Updates are manual-only. The server does not poll GitHub or install a release
merely because a newer version exists.

## Security model

- only an administrator session or an administrator-owned API token with the
  exact `system:update` scope may inspect or trigger updates;
- the page's explicit check/apply action obtains the fixed official release
  metadata through the configured GitHub source, then verifies the checksums
  manifest, every multipart asset, and the reassembled bundle SHA-256;
- archives are size-limited, path-checked, and required to contain the complete
  CLI, server, worker, Web UI, Pipeline SDK, and trusted update helper;
- only one update may run at a time;
- installation is transactional and preserves one `.previous` bundle for
  automatic rollback;
- tokens, credentials, host paths, and update logs are never returned by the
  update API.

API tokens should be short-lived and used through the `Authorization` header.
Do not put a token in a URL, shell history, repository file, or build log.

## Check and apply from the BuildWorld page

An administrator can click the refresh button beside the BuildWorld logo. The
button calls the authenticated check endpoint and shows the matching release
for the current platform. If a newer stable release exists, the administrator
can confirm the update in the same popover. The service briefly restarts only
after that confirmation and rolls back automatically if readiness fails.

The equivalent API calls are:

```sh
curl -fsS \
  -H "Authorization: Bearer $BUILDWORLD_UPDATE_TOKEN" \
  http://buildworld.example:8080/api/system/update/check

curl -fsS -X POST \
  -H "Authorization: Bearer $BUILDWORLD_UPDATE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}' \
  http://buildworld.example:8080/api/system/update/apply
```

Both endpoints require an administrator session or an administrator-owned API
token with the exact `system:update` scope. The apply endpoint does not accept
a caller-provided download URL.

## Check update status

```sh
curl -fsS \
  -H "Authorization: Bearer $BUILDWORLD_UPDATE_TOKEN" \
  http://buildworld.example:8080/api/system/update/
```

The response includes the running version, target platform, maximum bundle
size, automatic-update policy, and the latest operation state.

## Apply a verified bundle

Generate the complete local release first. The canonical packaging command
reruns every required verification gate:

```powershell
.\scripts\release-local.ps1 -Version 1.0.1
```

Then invoke the production server:

```sh
BUNDLE=release/v1.0.1/buildworld-darwin-arm64.tar.gz
SHA256="$(shasum -a 256 "$BUNDLE" | awk '{print $1}')"

curl -fsS \
  -H "Authorization: Bearer $BUILDWORLD_UPDATE_TOKEN" \
  -F mode=manual \
  -F version=1.0.1 \
  -F sha256="$SHA256" \
  -F bundle=@"$BUNDLE" \
  http://buildworld.example:8080/api/system/update/
```

The server returns `202 Accepted` after staging and verification. The detached
helper pauses the service, rotates the current bundle, starts the new version,
checks `/api/health` and `/api/version`, and rolls back if readiness fails.
Poll the status endpoint until it reports `succeeded` or `rolled_back`.

The server uses `https://gh-proxy.com` as the default GitHub mirror for public
release metadata and assets. Set `BUILDWORLD_GITHUB_MIRROR=off` to use GitHub
directly, or set it to another trusted HTTPS mirror. The mirror is not trusted
for integrity: every asset is still checked against the release's SHA-256
manifest before installation.

## Automatic callers

Automatic update requests are not supported. Requests with `mode=automatic`
are rejected with `403 automatic_updates_disabled`; an administrator must
start every update explicitly.

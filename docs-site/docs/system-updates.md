---
sidebar_position: 3
---

# System updates

BuildWorld supports explicit, authenticated bundle updates on macOS and Linux.
Automatic update requests are disabled by default. The server does not poll
GitHub or install a release merely because a newer version exists.

## Security model

- only an administrator session or an administrator-owned API token with the
  exact `system:update` scope may inspect or trigger updates;
- the caller supplies a locally verified release bundle and its SHA-256;
- archives are size-limited, path-checked, and required to contain the complete
  CLI, server, worker, Web UI, Pipeline SDK, and trusted update helper;
- only one update may run at a time;
- installation is transactional and preserves one `.previous` bundle for
  automatic rollback;
- tokens, credentials, host paths, and update logs are never returned by the
  update API.

API tokens should be short-lived and used through the `Authorization` header.
Do not put a token in a URL, shell history, repository file, or build log.

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

## Automatic callers

An orchestrator may send the same request with `mode=automatic`, but the server
returns `409 automatic_updates_disabled` until an administrator explicitly
enables **Allow automatic update requests** under Runtime settings.

This setting is `false` in fresh configuration and does not create an internal
polling job. It is only an authorization gate for an external, authenticated
orchestrator. Manual updates remain available while it is disabled.

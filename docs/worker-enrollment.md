# Remote Worker Enrollment

`buildworld-server` is always the local default executor. A pipeline switches to
a remote worker only when its agent requirements select a pool or label.

1. Set a long random `workers.enrollment_token` in `config.yaml`.
2. Run a worker with its reachable address:

```powershell
buildworld-worker --server http://world.internal:8080 --token <enrollment-token> --listen :6051 --advertise 10.0.0.24:6051
```

3. Start another worker on the same host with another port, for example `:6052`, and its own `--state-dir`. The default state directory is isolated by controller address and worker listen address.

Workers auto-register through the enrollment endpoint, receive an individual
heartbeat token, and serve the structured `bytemsg233` major-1 protobuf RPC. The
server dispatches streamed logs and chunked artifact frames to an eligible
worker. One-shot workspaces are removed after artifact transfer. Git-backed
workspaces are reused per repository and branch, with an OS lock held for the
whole build so concurrent jobs never mutate one checkout. Existing matching
checkouts can be adopted without deleting engine Library caches. Each artifact is checksummed and limited to 256 MiB per
file. Buildworld falls back to the embedded local executor only when no remote
requirement is declared.

The v1 worker RPC authenticates requests but does not encrypt transport or use
mTLS. Bind and firewall each worker port to the BuildWorld server on a trusted
private network or VPN. Do not publish worker or control-plane ports directly
to the Internet; use a trusted HTTPS reverse proxy for control-plane traffic.

For a pipeline with `## Agents` requirements, the server reserves one worker
slot atomically before it dispatches. `active_builds` is displayed beside
`max_concurrent_builds` in the node list; when every matching worker is full,
the build remains pending and Buildworld retries assignment instead of running
it on the embedded executor.

## Remote Go plugins

When a pipeline uses a Go binary plugin installed from a GitHub URL,
`buildworld-server` sends the worker only the approved repository URL, plugin
name, version, and SHA-256 digest of the plugin manifest. The worker caches the
plugin under its user cache directory, resolves the matching release (or builds
the source) for its own OS and architecture, and verifies the manifest before
executing it. Executable bytes are never embedded in a build message. Script
plugins are not supported; workers execute only verified Go binaries.

## Remote operations (1.17.1+)

Use 1.17.1 or later on HTTP intranet origins. That version supports browser request identities without secure-context-only APIs and preserves nested quotes in native Windows shell programs.

Open the authenticated administrator page `/agents`, then **Manage**. The list refreshes every ten seconds; the open runtime dialog refreshes every five seconds without resetting editable fields. An old worker explicitly shows that no management snapshot has been reported. Upgrade that worker through its existing supervised installation first; an empty report is never treated as healthy.

- **Capture diagnostics** records version/source identity, PID, Windows session, active build/stage/step/workspace, memory, goroutine stacks (64 KiB maximum), owned process names and tool availability. Process arguments and environment are excluded. Build links retain live logs and cancellation through the existing build API.
- **Drain dispatch** immediately blocks controller reservations, then instructs the worker to reject new work. Existing builds continue. **Resume dispatch** clears the controller gate only after the worker acknowledges.
- **Apply capacity** accepts 1–64 slots. Both controller reservation and local worker admission enforce limits. Lowering the limit never cancels current tasks. Multiple workers in the same label/pool remain independently scheduled.
- **Update Worker** accepts an official full release archive, version and SHA-256. Updates are manual-only. The server validates archive paths, platform, release identity and the full-upload digest, stores it privately, and records an immutable operation. The worker downloads only its authenticated operation bundle and verifies it again. No remote URL, server-local path or installer command is accepted.

Each operation requires a stable `request_id`. Repeating identical input returns the same receipt; a changed input conflicts. Receipts survive controller/worker restart. The authenticated API is:

`GET /api/agents/{id}/runtime`

`POST /api/agents/{id}/operations` with `{"request_id":"unique-key","kind":"diagnose|drain|resume|concurrency","capacity":2}` (capacity is present only for concurrency).

`POST /api/agents/{id}/update` uses multipart fields `mode=manual`, `request_id`, `version`, `sha256`, `bundle`. Administrator API tokens additionally need the exact `system:update` scope. Worker heartbeat tokens cannot access administrator routes or another worker's bundle.

## Update ownership and recovery

The worker first drains and waits for all execution owners to finish. Its private helper waits for the original daemon's OS lock, preserves the original executable, installs only the verified worker, and starts it with the existing arguments and working directory. Only a new PID reporting the expected version through an acknowledged controller heartbeat proves readiness. The worker stays drained until the operator verifies its receipt and resumes dispatch. A candidate that exits or fails to acknowledge readiness is stopped by its owned process handle; the original executable is restored and restarted, with a `rolled_back` receipt. Workspace, runtime data, logs and controller metadata are preserved.

Keep the private worker state directory and its update materials when replacing an installation. It holds identity credentials, admission settings and pending update receipts; do not copy it to another worker. Full bundles carry a separate `buildworld-worker.release-info.json` so updating a worker never rewrites a co-located controller's identity. An interrupted helper or an unreadable state file fails closed; inspect the retained original binary and plan before manually recovering. Do not delete state to make an error disappear.

On Windows, a GUI-dependent engine worker must run in the intended logged-in user session. A Session 0 service cannot be made equivalent by increasing worker slots. Verify the reported session and a real engine build after changing startup ownership.

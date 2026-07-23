# Remote Worker Enrollment

`buildworld-server` is always the local default executor. A pipeline switches to
a remote worker only when its agent requirements select a pool or label.

1. Set a long random `workers.enrollment_token` in `config.yaml`.
2. Run a worker with its reachable address:

```powershell
buildworld-worker --server http://world.internal:8080 --token <enrollment-token> --listen :6051 --advertise 10.0.0.24:6051
```

3. Start another worker on the same host with another port, for example `:6052`.

Workers auto-register through the enrollment endpoint, receive an individual
heartbeat token, and serve the structured `bytemsg233` major-1 protobuf RPC. The
server dispatches streamed logs and chunked artifact frames to an eligible
worker, so generated files return to the server before the worker removes its
temporary workspace. Each artifact is checksummed and limited to 256 MiB per
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

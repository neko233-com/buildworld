# buildworld-server

Control-plane binary. It serves the web/API, runs the embedded local executor,
and schedules registered workers.

```powershell
go run ./buildworld-server --config config.yaml
```

Set `workers.enrollment_token` before registering remote workers. Login uses
the configured local account and issues a 30-day JWT session cookie.

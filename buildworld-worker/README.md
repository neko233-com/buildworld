# buildworld-worker

Remote Buildworld executor. Start more than one instance on the same computer
by assigning distinct listen and advertise addresses.

```powershell
go run ./buildworld-worker --server http://server:8700 --token <enrollment-token> --listen :6051 --advertise 10.0.0.24:6051
go run ./buildworld-worker --server http://server:8700 --token <enrollment-token> --listen :6052 --advertise 10.0.0.24:6052
```

Remote dispatch uses the versioned `bytemsg233/v3` protobuf contract and the
same enrollment token as a gRPC dispatch credential.

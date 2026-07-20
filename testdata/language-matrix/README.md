# BuildWorld language packaging matrix

Every directory is a deliberately dependency-light project used by the
opt-in integration suite in `integration/language_matrix_test.go`.

| Runtime | Verification | Packaging artifacts |
| --- | --- | --- |
| Go | test, vet, host build, Linux cross-build | Windows executable, Linux binary |
| Rust | fmt, clippy, test, release build | executable, `.crate` source package |
| TypeScript | strict type-check, compiled smoke test | ESM output, declarations, npm `.tgz` |
| C# | restore, build, executable smoke test | framework-dependent publish, `.nupkg` |
| Java | Gradle compile, check, executable smoke test | runnable `.jar` |

Run the full local capability matrix:

```powershell
$env:BUILDWORLD_RUN_LANGUAGE_MATRIX = '1'
go test ./integration -run TestLanguagePackagingMatrix -v -count=1 -timeout=20m
```

An unavailable SDK is reported as an explicit skipped subtest. Installed SDKs
must execute successfully and produce their expected artifacts.

Unity and Tuanjie are intentionally excluded from the automated matrix. Use
the manual PowerShell or shell commands in
[`docs/manual-unity-tuanjie.md`](../../docs/manual-unity-tuanjie.md) when a
game editor is configured on the target machine.

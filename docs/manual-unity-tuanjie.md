# Manual Unity and Tuanjie build checks

Unity and Tuanjie are not part of the automated verification matrix. On a
machine with the appropriate editor installed, run the fixture manually when a
game build needs confirmation.

## PowerShell (Windows)

```powershell
$editor = 'C:\Program Files\Unity\Hub\Editor\2022.3.0f1\Editor\Unity.exe' # or Tuanjie.exe
$project = (Resolve-Path 'testdata\language-matrix\unity').Path
$output = Join-Path $project 'Build\BuildWorldManual.exe'
& $editor -batchmode -nographics -quit `
  -projectPath $project `
  -executeMethod BuildScript.BuildWindowsPlayer `
  -buildOutput $output `
  -logFile (Join-Path $project 'build.log')
if ($LASTEXITCODE -ne 0) { throw "Editor build failed with exit code $LASTEXITCODE" }
Test-Path $output
```

Set `$editor` to the desired Unity or Tuanjie executable. The expected output
is `BuildWorldManual.exe` plus its `_Data` directory under the fixture's
`Build` folder.

## Shell (macOS/Linux)

```bash
EDITOR='/Applications/Unity/Hub/Editor/2022.3.0f1/Unity.app/Contents/MacOS/Unity' # or Tuanjie editor binary
PROJECT="$(pwd)/testdata/language-matrix/unity"
OUTPUT="$PROJECT/Build/BuildWorldManual"
"$EDITOR" -batchmode -nographics -quit \
  -projectPath "$PROJECT" \
  -executeMethod BuildScript.BuildWindowsPlayer \
  -buildOutput "$OUTPUT" \
  -logFile "$PROJECT/build.log"
test -f "$OUTPUT"
```

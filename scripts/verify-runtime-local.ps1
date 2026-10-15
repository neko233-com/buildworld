param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v\d+\.\d+\.\d+$')]
    [string]$Version
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'This local release smoke gate currently requires the Windows packaging host.' }
$repoRoot = Split-Path -Parent $PSScriptRoot
$smokeRoot = Join-Path $repoRoot ('.build/runtime-smoke-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $smokeRoot -Force | Out-Null
$server = Join-Path $smokeRoot 'buildworld-server.exe'
$cli = Join-Path $smokeRoot 'buildworld.exe'
$ldflags = "-s -w -X github.com/neko233-com/buildworld/internal/buildinfo.Version=$Version"
$child = $null
$previousCGO = [Environment]::GetEnvironmentVariable('CGO_ENABLED', 'Process')
try {
    $env:CGO_ENABLED = '0'
    Push-Location $repoRoot
    try {
        & go build -trimpath -buildvcs=false -ldflags $ldflags -o $server ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw 'Smoke server compilation failed.' }
        & go build -trimpath -buildvcs=false -ldflags $ldflags -o $cli ./cmd/cli
        if ($LASTEXITCODE -ne 0) { throw 'Smoke CLI compilation failed.' }
    } finally { Pop-Location }
    $cliVersion = (& $cli version).Trim()
    if ($LASTEXITCODE -ne 0 -or $cliVersion -ne "buildworld $Version") { throw 'CLI release version mismatch.' }

    # Reserve only the isolated loopback address. Never stop an existing service.
    $probe = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Parse('127.0.0.2'), 8080)
    try { $probe.Start() } finally { $probe.Stop() }
    $configPath = Join-Path $smokeRoot 'config.yaml'
    $yamlRoot = $smokeRoot.Replace('\', '/')
    $config = @"
server:
  host: 127.0.0.2
  port: 8080
database:
  path: "$yamlRoot/data/buildworld.db"
plugins:
  path: "$yamlRoot/plugins"
storage:
  build_temp: "$yamlRoot/build_temp"
  artifacts: "$yamlRoot/artifacts"
workers:
  local:
    max_concurrent_builds: 1
"@
    [IO.File]::WriteAllText($configPath, $config, [Text.UTF8Encoding]::new($false))
    # APPDATA isolates server.pid and all CLI state from the developer's service.
    $child = Start-Process -FilePath $server -ArgumentList @('-config', ('"' + $configPath + '"')) -WorkingDirectory $repoRoot `
        -WindowStyle Hidden -PassThru -Environment @{ APPDATA = (Join-Path $smokeRoot 'state') } `
        -RedirectStandardOutput (Join-Path $smokeRoot 'stdout.log') -RedirectStandardError (Join-Path $smokeRoot 'stderr.log')
    $base = 'http://127.0.0.2:8080'
    $deadline = [DateTimeOffset]::UtcNow.AddSeconds(30)
    $ready = $false
    do {
        if ($child.HasExited) { throw "Smoke server exited; inspect $smokeRoot/stderr.log" }
        try {
            $health = Invoke-RestMethod "$base/api/health" -NoProxy -TimeoutSec 2
            $ready = $health.status -eq 'ok'
        } catch { Start-Sleep -Milliseconds 250 }
    } while (-not $ready -and [DateTimeOffset]::UtcNow -lt $deadline)
    if (-not $ready) { throw 'Isolated server did not become healthy.' }
    $remoteVersion = Invoke-RestMethod "$base/api/version" -NoProxy -TimeoutSec 5
    if ($remoteVersion.version -ne $Version) { throw 'Server release version mismatch.' }
    $html = Invoke-WebRequest $base -NoProxy -TimeoutSec 5
    if ($html.Content -notmatch '(?i)<!doctype html>') { throw 'Production Web UI was not served.' }
    $login = Invoke-RestMethod "$base/api/auth/login" -NoProxy -Method Post -ContentType 'application/json' -Body '{"username":"root","password":"root"}'
    $headers = @{ Authorization = 'Bearer ' + $login.token }
    $pipeline = @"
import { definePipeline, stage, shell } from '@buildworld/pipeline';
export default definePipeline({name: 'release-smoke', stages: [stage('Verify', [shell('Echo', 'echo BUILDWORLD_RELEASE_SMOKE_OK')])]});
"@
    $body = @{ name = 'BuildWorld release smoke'; config = $pipeline; pipeline_format = 'typescript'; pipeline_source_mode = 'inline' } | ConvertTo-Json
    $project = Invoke-RestMethod "$base/api/projects/" -NoProxy -Method Post -Headers $headers -ContentType 'application/json' -Body $body
    $build = Invoke-RestMethod "$base/api/projects/$($project.id)/builds" -NoProxy -Method Post -Headers $headers -ContentType 'application/json' -Body '{}'
    $deadline = [DateTimeOffset]::UtcNow.AddSeconds(60)
    do {
        Start-Sleep -Milliseconds 300
        $build = Invoke-RestMethod "$base/api/builds/$($build.id)" -NoProxy -Headers $headers -TimeoutSec 5
    } while ($build.status -notin @('success', 'failed', 'cancelled') -and [DateTimeOffset]::UtcNow -lt $deadline)
    if ($build.status -ne 'success' -or $build.log -notmatch 'BUILDWORLD_RELEASE_SMOKE_OK') {
        throw "Builtin execution failed: status $($build.status). Inspect $smokeRoot."
    }
    [ordered]@{ version = $Version; endpoint = $base; cli = $cliVersion; web = 200; executor = 'builtin'; buildStatus = $build.status } |
        ConvertTo-Json | Set-Content -LiteralPath (Join-Path $smokeRoot 'result.json') -Encoding utf8
    Write-Host "Runtime smoke passed: CLI/server $Version, health, production Web UI, login and builtin pipeline. Evidence: $smokeRoot/result.json"
} finally {
    if ($null -ne $child -and -not $child.HasExited) { Stop-Process -Id $child.Id -ErrorAction SilentlyContinue; $child.WaitForExit() }
    if ($null -eq $previousCGO) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue } else { $env:CGO_ENABLED = $previousCGO }
}

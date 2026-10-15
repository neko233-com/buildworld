$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../release-archive.ps1')
$gitPath = (Get-Command git -ErrorAction Stop).Source
$tar = Join-Path (Split-Path -Parent (Split-Path -Parent $gitPath)) 'usr/bin/tar.exe'
$fixture = Join-Path $PSScriptRoot ('../../.build/archive-regression-' + [guid]::NewGuid().ToString('N'))
$fixture = [IO.Path]::GetFullPath($fixture)
$stage = Join-Path $fixture 'stage'
New-Item -ItemType Directory -Force -Path $stage | Out-Null
foreach ($name in @('buildworld', 'buildworld-server', 'buildworld-worker', 'apply-update.sh')) {
    [IO.File]::WriteAllText((Join-Path $stage $name), 'fixture')
}
$tarPath = Join-Path $fixture 'bundle.tar'
& $tar --force-local --format=ustar '--mode=u=rwx,go=rx' -C $stage.Replace('\', '/') -cf $tarPath.Replace('\', '/') .
if ($LASTEXITCODE -ne 0) { throw 'Unable to create tar regression fixture.' }
$archive = Join-Path $fixture 'bundle.tar.gz'
$source = [IO.File]::OpenRead($tarPath)
$target = [IO.File]::Create($archive)
$gzip = [IO.Compression.GZipStream]::new($target, [IO.Compression.CompressionLevel]::Optimal)
try { $source.CopyTo($gzip) } finally { $gzip.Dispose(); $source.Dispose(); $target.Dispose() }
$savedPath = $env:PATH
try {
    # Reproduce the release host without gzip.exe on PATH.
    $env:PATH = $stage
    Assert-GzipMatchesTar $archive $tarPath
    Assert-ReleaseTarModes $tar $tarPath
    [IO.File]::WriteAllBytes($archive, [byte[]]@(1, 2, 3))
    $rejected = $false
    try { Assert-GzipMatchesTar $archive $tarPath } catch { $rejected = $true }
    if (-not $rejected) { throw 'Corrupt gzip archive was accepted.' }
    & $tar --force-local --format=ustar '--mode=u=rw,go=r' -C $stage.Replace('\', '/') -cf $tarPath.Replace('\', '/') .
    if ($LASTEXITCODE -ne 0) { throw 'Unable to create non-executable fixture.' }
    $rejected = $false
    try { Assert-ReleaseTarModes $tar $tarPath } catch { $rejected = $true }
    if (-not $rejected) { throw 'Non-executable Unix release binaries were accepted.' }
} finally { $env:PATH = $savedPath }
Write-Host 'Unix release archive regressions passed without external gzip (3 cases).'

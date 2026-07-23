param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v?\d+\.\d+\.\d+$')]
    [string]$Version
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (Test-Path variable:PSNativeCommandUseErrorActionPreference) {
    $PSNativeCommandUseErrorActionPreference = $false
}

$Version = if ($Version.StartsWith('v')) { $Version } else { "v$Version" }
$repoRoot = [IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot))
$releaseRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot 'release'))
$output = [IO.Path]::GetFullPath((Join-Path $releaseRoot $Version))
$stagingOutput = [IO.Path]::GetFullPath((Join-Path $releaseRoot ('.staging-' + $Version + '-' + [guid]::NewGuid().ToString('N'))))
$webRoot = Join-Path $repoRoot 'web'
$partSizeBytes = 900KB
$gitExecutable = (Get-Command git -ErrorAction Stop).Source
$gitRoot = Split-Path -Parent (Split-Path -Parent $gitExecutable)
$gnuTar = Join-Path $gitRoot 'usr\bin\tar.exe'
$targets = @(
    @{ Os = 'windows'; Arch = 'amd64'; Archive = 'zip' },
    @{ Os = 'windows'; Arch = 'arm64'; Archive = 'zip' },
    @{ Os = 'linux'; Arch = 'amd64'; Archive = 'tar.gz' },
    @{ Os = 'linux'; Arch = 'arm64'; Archive = 'tar.gz' },
    @{ Os = 'darwin'; Arch = 'amd64'; Archive = 'tar.gz' },
    @{ Os = 'darwin'; Arch = 'arm64'; Archive = 'tar.gz' }
)

function Assert-ChildPath([string]$Path, [string]$Parent, [string]$Label) {
    $resolvedPath = [IO.Path]::GetFullPath($Path)
    $resolvedParent = [IO.Path]::GetFullPath($Parent).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
    if (-not $resolvedPath.StartsWith($resolvedParent + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Unsafe $Label path: $resolvedPath"
    }
    return $resolvedPath
}

function Remove-SafeDirectory([string]$Path, [string]$Parent, [string]$Label) {
    $safePath = Assert-ChildPath $Path $Parent $Label
    if (Test-Path -LiteralPath $safePath) {
        Remove-Item -LiteralPath $safePath -Recurse -Force
    }
}

function Remove-SafeFile([string]$Path, [string]$Parent, [string]$Label) {
    $safePath = Assert-ChildPath $Path $Parent $Label
    if (Test-Path -LiteralPath $safePath) {
        Remove-Item -LiteralPath $safePath -Force
    }
}

function New-UnixReleaseArchive([string]$Stage, [string]$Archive) {
    if (-not (Test-Path -LiteralPath $gnuTar -PathType Leaf)) {
        throw "GNU tar is required to preserve executable modes: $gnuTar"
    }

    $temporaryTar = Assert-ChildPath "$Archive.tmp.tar" $stagingOutput 'temporary tar archive'
    # Git for Windows GNU tar treats backslashes in -C paths as escapes. Use
    # slash-normalized absolute paths for every tar operand while retaining the
    # native paths for PowerShell/.NET file operations and safety checks.
    $tarStage = $Stage.Replace('\', '/')
    $tarArchive = $Archive.Replace('\', '/')
    $tarTemporary = $temporaryTar.Replace('\', '/')
    $binaries = @('./buildworld', './buildworld-server', './buildworld-worker', './apply-update.sh')
    try {
        & $gnuTar --force-local --format=ustar '--mode=u=rwX,go=rX' `
            '--exclude=./buildworld' '--exclude=./buildworld-server' '--exclude=./buildworld-worker' '--exclude=./apply-update.sh' `
            -C $tarStage -cf $tarTemporary .
        if ($LASTEXITCODE -ne 0) {
            throw "tar content archive failed with exit code $LASTEXITCODE"
        }

        & $gnuTar --force-local --format=ustar '--mode=u=rwx,go=rx' -C $tarStage -rf $tarTemporary @binaries
        if ($LASTEXITCODE -ne 0) {
            throw "tar executable append failed with exit code $LASTEXITCODE"
        }

        $sourceStream = $null
        $targetStream = $null
        $gzipStream = $null
        try {
            $sourceStream = [IO.File]::OpenRead($temporaryTar)
            $targetStream = [IO.File]::Create($Archive)
            $gzipStream = [IO.Compression.GZipStream]::new(
                $targetStream,
                [IO.Compression.CompressionLevel]::Optimal,
                $false
            )
            $sourceStream.CopyTo($gzipStream)
        } finally {
            if ($null -ne $gzipStream) { $gzipStream.Dispose() }
            if ($null -ne $sourceStream) { $sourceStream.Dispose() }
            if ($null -ne $targetStream) { $targetStream.Dispose() }
        }

        $listing = @(& $gnuTar --force-local -tzvf $tarArchive @binaries)
        if ($LASTEXITCODE -ne 0) {
            throw "tar mode verification failed with exit code $LASTEXITCODE"
        }
        foreach ($binary in $binaries) {
            $entry = $listing | Where-Object { $_ -match "\s$([regex]::Escape($binary))$" } | Select-Object -First 1
            if ($null -eq $entry -or $entry -notmatch '^-rwxr-xr-x\s') {
                throw "Release archive entry must be executable (0755): $binary; found '$entry'"
            }
        }
    } finally {
        Remove-SafeFile $temporaryTar $stagingOutput 'temporary tar cleanup'
    }
}

function Split-ReleaseAsset([string]$Archive) {
    $parts = [System.Collections.Generic.List[string]]::new()
    $stream = [IO.File]::OpenRead($Archive)
    try {
        $index = 0
        $buffer = New-Object byte[] $partSizeBytes
        while (($read = $stream.Read($buffer, 0, $buffer.Length)) -gt 0) {
            $part = "$Archive.part$($index.ToString('000'))"
            $target = [IO.File]::Open($part, [IO.FileMode]::Create, [IO.FileAccess]::Write)
            try { $target.Write($buffer, 0, $read) } finally { $target.Dispose() }
            $parts.Add($part)
            $index++
        }
    } finally {
        $stream.Dispose()
    }
    return $parts
}

[void](Assert-ChildPath $output $releaseRoot 'release output')
[void](Assert-ChildPath $stagingOutput $releaseRoot 'release staging')

$previousGoos = [Environment]::GetEnvironmentVariable('GOOS', 'Process')
$previousGoarch = [Environment]::GetEnvironmentVariable('GOARCH', 'Process')
$previousCgoEnabled = [Environment]::GetEnvironmentVariable('CGO_ENABLED', 'Process')
$committed = $false

try {
    Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    & (Join-Path $PSScriptRoot 'verify-local.ps1')
    if (-not $?) {
        throw 'Local verification failed.'
    }

    # Release binaries use the repository's pure-Go SQLite driver. Force CGO
    # off so inherited developer settings cannot require host/cross C toolchains
    # or silently produce platform-dependent release binaries.
    $env:CGO_ENABLED = '0'

    New-Item -ItemType Directory -Force -Path $stagingOutput | Out-Null
    $assets = [System.Collections.Generic.List[string]]::new()
    foreach ($target in $targets) {
        $os = $target.Os
        $arch = $target.Arch
        $extension = if ($os -eq 'windows') { '.exe' } else { '' }
        $stage = Assert-ChildPath (Join-Path $stagingOutput "stage-$os-$arch") $stagingOutput 'target staging'
        $archive = Assert-ChildPath (Join-Path $stagingOutput "buildworld-$os-$arch.$($target.Archive)") $stagingOutput 'target archive'
        Write-Host "Packaging $os/$arch ..."
        New-Item -ItemType Directory -Force -Path (Join-Path $stage 'web') | Out-Null
        New-Item -ItemType Directory -Force -Path (Join-Path $stage 'sdk\pipeline') | Out-Null
        Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'apply-update.sh') -Destination (Join-Path $stage 'apply-update.sh') -Force
        $env:GOOS = $os
        $env:GOARCH = $arch
        $ldflags = "-s -w -X github.com/neko233-com/buildworld/internal/buildinfo.Version=$Version"

        Push-Location $repoRoot
        try {
            & go build -trimpath -buildvcs=false -ldflags $ldflags -o (Join-Path $stage "buildworld$extension") ./cmd/cli
            if ($LASTEXITCODE -ne 0) { throw "CLI build failed for $os/$arch with exit code $LASTEXITCODE" }
            & go build -trimpath -buildvcs=false -ldflags $ldflags -o (Join-Path $stage "buildworld-server$extension") ./cmd/server
            if ($LASTEXITCODE -ne 0) { throw "server build failed for $os/$arch with exit code $LASTEXITCODE" }
            & go build -trimpath -buildvcs=false -ldflags $ldflags -o (Join-Path $stage "buildworld-worker$extension") ./cmd/worker
            if ($LASTEXITCODE -ne 0) { throw "worker build failed for $os/$arch with exit code $LASTEXITCODE" }
        } finally {
            Pop-Location
        }

        Copy-Item -Path (Join-Path $webRoot 'dist') -Destination (Join-Path $stage 'web') -Recurse -Force
        Copy-Item -Path (Join-Path $repoRoot 'sdk\pipeline\*') -Destination (Join-Path $stage 'sdk\pipeline') -Recurse -Force
        foreach ($required in @(
            (Join-Path $stage "buildworld$extension"),
            (Join-Path $stage "buildworld-server$extension"),
            (Join-Path $stage "buildworld-worker$extension"),
            (Join-Path $stage 'apply-update.sh'),
            (Join-Path $stage 'web\dist\index.html'),
            (Join-Path $stage 'web\dist\script-api.html'),
            (Join-Path $stage 'sdk\pipeline\index.d.ts'),
            (Join-Path $stage 'sdk\pipeline\index.js'),
            (Join-Path $stage 'sdk\pipeline\package.json')
        )) {
            if (-not (Test-Path -LiteralPath $required -PathType Leaf)) {
                throw "Release layout is incomplete for $os/${arch}: missing $required"
            }
        }

        if ($target.Archive -eq 'zip') {
            Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive -Force
        } else {
            New-UnixReleaseArchive $stage $archive
        }
        $assets.Add($archive)
        $parts = @(Split-ReleaseAsset $archive)
        if ($parts.Count -eq 0) { throw "No multipart assets were created for $archive" }
        foreach ($part in $parts) {
            if ((Get-Item -LiteralPath $part).Length -gt $partSizeBytes) {
                throw "Multipart asset exceeds $partSizeBytes bytes: $part"
            }
            $assets.Add($part)
        }
        Remove-SafeDirectory $stage $stagingOutput 'target staging cleanup'
    }

    Copy-Item (Join-Path $PSScriptRoot 'install.ps1') (Join-Path $stagingOutput 'install.ps1') -Force
    Copy-Item (Join-Path $PSScriptRoot 'install.sh') (Join-Path $stagingOutput 'install.sh') -Force
    $assets.Add((Join-Path $stagingOutput 'install.ps1'))
    $assets.Add((Join-Path $stagingOutput 'install.sh'))

    $checksums = foreach ($asset in $assets) {
        "{0}  {1}" -f (Get-FileHash -LiteralPath $asset -Algorithm SHA256).Hash.ToLowerInvariant(), (Split-Path $asset -Leaf)
    }
    $checksumPath = Join-Path $stagingOutput 'checksums.txt'
    [IO.File]::WriteAllLines($checksumPath, $checksums, [Text.UTF8Encoding]::new($false))

    $backupOutput = Assert-ChildPath (Join-Path $releaseRoot ('.backup-' + $Version + '-' + [guid]::NewGuid().ToString('N'))) $releaseRoot 'release backup'
    $hasBackup = $false
    try {
        if (Test-Path -LiteralPath $output) {
            Move-Item -LiteralPath $output -Destination $backupOutput
            $hasBackup = $true
        }
        try {
            Move-Item -LiteralPath $stagingOutput -Destination $output
        } catch {
            if ($hasBackup -and -not (Test-Path -LiteralPath $output)) {
                Move-Item -LiteralPath $backupOutput -Destination $output
                $hasBackup = $false
            }
            throw
        }
        $committed = $true
        if ($hasBackup) {
            try {
                Remove-SafeDirectory $backupOutput $releaseRoot 'release backup cleanup'
                $hasBackup = $false
            } catch {
                Write-Warning "New release is active, but the previous release backup could not be removed: $backupOutput"
            }
        }
    } finally {
        if (-not $committed -and $hasBackup -and -not (Test-Path -LiteralPath $output)) {
            Move-Item -LiteralPath $backupOutput -Destination $output
        }
    }

    Write-Host "Release assets are ready in $output"
    Get-ChildItem -LiteralPath $output -File | Select-Object Name, Length
} finally {
    if ($null -eq $previousGoos) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGoos }
    if ($null -eq $previousGoarch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGoarch }
    if ($null -eq $previousCgoEnabled) { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue } else { $env:CGO_ENABLED = $previousCgoEnabled }
    if (-not $committed -and (Test-Path -LiteralPath $stagingOutput)) {
        Remove-SafeDirectory $stagingOutput $releaseRoot 'failed release staging cleanup'
    }
}

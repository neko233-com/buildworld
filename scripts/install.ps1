param(
    [string]$Version = "latest",
    [switch]$NoAutostart
)

$ErrorActionPreference = "Stop"
$Repo = "neko233-com/buildworld233"
$InstallDir = Join-Path $env:LOCALAPPDATA "BuildWorld"
$PreviousDir = "$InstallDir.previous"

function Get-ReleaseVersion {
    if ($Version -ne "latest") { return $Version.TrimStart('v') }
    return ((Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest").tag_name).TrimStart('v')
}

function Get-Architecture {
    if ([Environment]::Is64BitOperatingSystem -and $env:PROCESSOR_ARCHITECTURE -eq "ARM64") { return "arm64" }
    return "amd64"
}

function Assert-Checksum([string]$Archive, [string]$Checksums) {
    $name = Split-Path $Archive -Leaf
    $expected = (($Checksums -split "`n") | Where-Object { $_ -match "\s$name$" } | Select-Object -First 1).Split()[0]
    if (-not $expected) { throw "No SHA-256 checksum published for $name" }
    $actual = (Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected.ToLowerInvariant()) { throw "Checksum mismatch for $name" }
}

function Get-BundleArchive([string]$Base, [string]$Asset, [string]$Archive, [string]$Checksums, [string]$Temporary) {
    try {
        Invoke-WebRequest -Uri "$Base/$Asset" -OutFile $Archive -UseBasicParsing
        return
    } catch {
        Remove-Item -LiteralPath $Archive -Force -ErrorAction SilentlyContinue
    }
    $escaped = [Regex]::Escape($Asset) + '\.part\d+$'
    $parts = @(($Checksums -split "`r?`n") | ForEach-Object { ($_ -split '\s{2,}', 2)[1] } | Where-Object { $_ -match $escaped } | Sort-Object)
    if ($parts.Count -eq 0) { throw "No release archive or multipart fallback published for $Asset" }
    $destination = [IO.File]::Open($Archive, [IO.FileMode]::Create, [IO.FileAccess]::Write)
    try {
        foreach ($part in $parts) {
            $partPath = Join-Path $Temporary $part
            Write-Host "Downloading $part ..."
            Invoke-WebRequest -Uri "$Base/$part" -OutFile $partPath -UseBasicParsing
            Assert-Checksum -Archive $partPath -Checksums $Checksums
            $source = [IO.File]::OpenRead($partPath)
            try { $source.CopyTo($destination) } finally { $source.Dispose() }
        }
    } finally { $destination.Dispose() }
}

function Add-UserPath([string]$Directory) {
    $current = [Environment]::GetEnvironmentVariable("Path", "User")
    $entries = @($current -split ';' | Where-Object { $_ })
    if ($entries -notcontains $Directory) {
        [Environment]::SetEnvironmentVariable("Path", (($entries + $Directory) -join ';'), "User")
    }
    if (($env:Path -split ';') -notcontains $Directory) { $env:Path = "$Directory;$env:Path" }
}

$release = Get-ReleaseVersion
$arch = Get-Architecture
$asset = "buildworld-windows-$arch.zip"
$base = "https://github.com/$Repo/releases/download/v$release"
$temporary = Join-Path ([IO.Path]::GetTempPath()) ("buildworld-" + [guid]::NewGuid())
$archive = Join-Path $temporary $asset
$staging = Join-Path $temporary "staging"

New-Item -ItemType Directory -Force -Path $temporary | Out-Null
try {
    Write-Host "Downloading BuildWorld v$release for Windows $arch ..."
    $checksums = (Invoke-WebRequest -Uri "$base/checksums.txt" -UseBasicParsing).Content
    Get-BundleArchive -Base $base -Asset $asset -Archive $archive -Checksums $checksums -Temporary $temporary
    Assert-Checksum -Archive $archive -Checksums $checksums
    Expand-Archive -LiteralPath $archive -DestinationPath $staging -Force

    $cli = Join-Path $InstallDir 'buildworld.exe'
    $wasRunning = $false
    if (Test-Path -LiteralPath $cli) {
        $wasRunning = (& $cli status 2>$null) -match 'is running'
        if ($wasRunning) {
            Write-Host 'Pausing running BuildWorld service ...'
            & $cli pause
        }
        Remove-Item -LiteralPath $PreviousDir -Recurse -Force -ErrorAction SilentlyContinue
        Move-Item -LiteralPath $InstallDir -Destination $PreviousDir
    }
    Move-Item -LiteralPath $staging -Destination $InstallDir
    Add-UserPath $InstallDir
    if (-not $NoAutostart) { & (Join-Path $InstallDir 'buildworld.exe') enable-autostart }
    & (Join-Path $InstallDir 'buildworld.exe') start
    if ($wasRunning) { Write-Host "Updated and restarted BuildWorld v$release. Previous bundle: $PreviousDir" }
    else { Write-Host "Installed BuildWorld v$release and started it. Previous bundle: $PreviousDir" }
    Write-Host "Commands: buildworld status | pause | resume | restart"
    Write-Host "Reset root safely: `$env:BUILDWORLD_ROOT_PASSWORD='...'; `$env:BUILDWORLD_ROOT_PASSWORD | buildworld reset-root-password --password-stdin"
} finally {
    Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}

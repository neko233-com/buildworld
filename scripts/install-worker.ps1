param([string]$Version = "latest")
$ErrorActionPreference = "Stop"
$BinaryName = "buildworld233-worker"
$Repo = "neko233-com/buildworld233"

function Get-LatestVersion {
    try {
        $r = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
        return ($r.tag_name -replace '^[vV]', '')
    } catch {
        return "0.1.0"
    }
}

function Install-BuildWorld233Worker {
    param([string]$Ver)
    $arch = "amd64"
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { $arch = "arm64" }
    $asset = "$BinaryName-windows-$arch.exe"
    $url = "https://github.com/$Repo/releases/download/v$Ver/$asset"
    $installDir = Join-Path $env:LOCALAPPDATA "buildworld233"
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    $dest = Join-Path $installDir "$BinaryName.exe"
    Write-Host "Downloading $url ..."
    Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing
    Write-Host "Installed to $dest"
    Write-Host "Register worker on server: buildworld233 worker register --server http://SERVER:6050 --name $(hostname)"
}

if ($Version -eq "latest") { $Version = Get-LatestVersion }
$Version = $Version -replace '^[vV]', ''
Write-Host "Installing buildworld233-worker v$Version ..."
Install-BuildWorld233Worker -Ver $Version

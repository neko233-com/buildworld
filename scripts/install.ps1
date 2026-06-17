param([string]$Version = "latest")
$ErrorActionPreference = "Stop"
$BinaryName = "buildworld233"
$Repo = "neko233-com/buildworld233"

function Get-LatestVersion {
    try {
        $r = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
        return ($r.tag_name -replace '^[vV]', '')
    } catch {
        return "0.1.0"
    }
}

function Install-BuildWorld233 {
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
    Write-Host "Run: buildworld233 start"
    Write-Host "Status: buildworld233 status"
    Write-Host "Enable boot autostart: buildworld233 enable-autostart"
    Write-Host "Change port: buildworld233 set-port 6050"
    Write-Host "Self update: buildworld233 update"
    Write-Host "Hot reload config: buildworld233 reload-config"
    Write-Host "Export backup: buildworld233 backup export --output ./backup.zip"
    Write-Host "Import backup: buildworld233 backup import --input ./backup.zip"
    Write-Host "Generate worker token: buildworld233 worker generate-token"
    Write-Host "List workers: buildworld233 worker list"
}

if ($Version -eq "latest") { $Version = Get-LatestVersion }
$Version = $Version -replace '^[vV]', ''
Write-Host "Installing buildworld233 v$Version ..."
Install-BuildWorld233 -Ver $Version

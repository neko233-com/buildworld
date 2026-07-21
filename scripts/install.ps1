param(
    [string]$Version = "latest",
    [switch]$NoAutostart
)

$ErrorActionPreference = "Stop"
$PSNativeCommandUseErrorActionPreference = $false
$Repo = "neko233-com/buildworld233"
$InstallDir = if ($env:BUILDWORLD_INSTALL_DIR) { $env:BUILDWORLD_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "BuildWorld" }
$PreviousDir = "$InstallDir.previous"
$script:ReleaseMetadata = $null

function Get-GitHubToken {
    if ($env:GH_TOKEN) { return $env:GH_TOKEN.Trim() }
    if ($env:GITHUB_TOKEN) { return $env:GITHUB_TOKEN.Trim() }

    $gh = Get-Command gh -ErrorAction SilentlyContinue
    if (-not $gh) { return "" }
    $candidate = @(& $gh.Source auth token 2>$null)
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0 -or $candidate.Count -eq 0) { return "" }
    return ([string]$candidate[0]).Trim()
}

$script:GitHubToken = Get-GitHubToken

function Get-GitHubHeaders([switch]$Binary) {
    $headers = @{
        "Accept"               = if ($Binary) { "application/octet-stream" } else { "application/vnd.github+json" }
        "User-Agent"           = "BuildWorld-Installer"
        "X-GitHub-Api-Version" = "2022-11-28"
    }
    if ($script:GitHubToken) { $headers["Authorization"] = "Bearer $($script:GitHubToken)" }
    return $headers
}

function Invoke-GitHubApi([string]$Uri) {
    return Invoke-RestMethod -Uri $Uri -Headers (Get-GitHubHeaders) -UseBasicParsing
}

function Get-ReleaseVersion {
    if ($Version -ne "latest") {
        $requested = $Version.TrimStart('v')
        if ($requested -notmatch '^\d+\.\d+\.\d+$') { throw "Invalid BuildWorld version: $Version" }
        return $requested
    }
    try {
        $script:ReleaseMetadata = Invoke-GitHubApi "https://api.github.com/repos/$Repo/releases/latest"
    } catch {
        $hint = if ($script:GitHubToken) { "Check that the token can read releases for $Repo." } else { "For a private repository, set GH_TOKEN/GITHUB_TOKEN or run 'gh auth login'." }
        throw "Cannot resolve the latest BuildWorld release. $hint $($_.Exception.Message)"
    }
    $tag = ([string]$script:ReleaseMetadata.tag_name).TrimStart('v')
    if ($tag -notmatch '^\d+\.\d+\.\d+$') { throw "GitHub returned an invalid BuildWorld release tag: $tag" }
    return $tag
}

function Get-ReleaseMetadata([string]$Release) {
    if ($script:ReleaseMetadata -and ([string]$script:ReleaseMetadata.tag_name).TrimStart('v') -eq $Release) {
        return $script:ReleaseMetadata
    }
    $script:ReleaseMetadata = Invoke-GitHubApi "https://api.github.com/repos/$Repo/releases/tags/v$Release"
    return $script:ReleaseMetadata
}

function Save-ReleaseAsset([string]$Release, [string]$Asset, [string]$Destination) {
    $apiFailure = $null
    if ($script:GitHubToken) {
        try {
            $metadata = Get-ReleaseMetadata $Release
            $releaseAsset = @($metadata.assets | Where-Object { $_.name -eq $Asset } | Select-Object -First 1)
            if ($releaseAsset.Count -eq 0) { throw "asset is not present in release metadata" }
            Invoke-WebRequest -Uri $releaseAsset[0].url -Headers (Get-GitHubHeaders -Binary) -OutFile $Destination -UseBasicParsing
            return
        } catch {
            $apiFailure = $_.Exception.Message
            Remove-Item -LiteralPath $Destination -Force -ErrorAction SilentlyContinue
        }
    }

    $direct = "https://github.com/$Repo/releases/download/v$Release/$Asset"
    try {
        Invoke-WebRequest -Uri $direct -OutFile $Destination -UseBasicParsing
        return
    } catch {
        Remove-Item -LiteralPath $Destination -Force -ErrorAction SilentlyContinue
        $hint = if ($script:GitHubToken) {
            "Authenticated API download failed: $apiFailure"
        } else {
            "For a private repository, set GH_TOKEN/GITHUB_TOKEN or run 'gh auth login'."
        }
        throw "Cannot download release asset $Asset. $hint $($_.Exception.Message)"
    }
}

function Get-Architecture {
    $architecture = ""
    try { $architecture = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() } catch {}
    if ($architecture -eq "Arm64" -or $env:PROCESSOR_ARCHITEW6432 -eq "ARM64" -or $env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
        return "arm64"
    }
    if ($architecture -eq "X64" -or $env:PROCESSOR_ARCHITEW6432 -eq "AMD64" -or $env:PROCESSOR_ARCHITECTURE -eq "AMD64") {
        return "amd64"
    }
    throw "Unsupported Windows architecture: $architecture $($env:PROCESSOR_ARCHITECTURE)"
}

function ConvertTo-ChecksumMap([string]$Contents) {
    $result = @{}
    foreach ($line in ($Contents -split "`r?`n")) {
        if (-not $line) { continue }
        if ($line -notmatch '^([0-9a-fA-F]{64})  ([^/\\]+)$') { throw "Invalid checksums.txt line: $line" }
        $name = $Matches[2]
        if ($result.ContainsKey($name)) { throw "Duplicate checksum entry for $name" }
        $result[$name] = $Matches[1].ToLowerInvariant()
    }
    return $result
}

function Assert-Checksum([string]$Path, [hashtable]$Checksums) {
    $name = Split-Path $Path -Leaf
    if (-not $Checksums.ContainsKey($name)) { throw "No SHA-256 checksum published for $name" }
    $actual = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $Checksums[$name]) { throw "Checksum mismatch for $name" }
}

function Get-BundleArchive([string]$Release, [string]$Asset, [string]$Archive, [hashtable]$Checksums, [string]$Temporary) {
    $pattern = '^' + [Regex]::Escape($Asset) + '\.part\d{3}$'
    $parts = @($Checksums.Keys | Where-Object { $_ -match $pattern } | Sort-Object)
    if ($parts.Count -eq 0) { throw "No multipart release bundle published for $Asset" }
    for ($index = 0; $index -lt $parts.Count; $index++) {
        $expectedName = "$Asset.part$($index.ToString('000'))"
        if ($parts[$index] -ne $expectedName) { throw "Multipart bundle is incomplete: expected $expectedName, found $($parts[$index])" }
    }

    $destination = [IO.File]::Open($Archive, [IO.FileMode]::Create, [IO.FileAccess]::Write)
    try {
        foreach ($part in $parts) {
            $partPath = Join-Path $Temporary $part
            Write-Host "Downloading $part ..."
            Save-ReleaseAsset -Release $Release -Asset $part -Destination $partPath
            Assert-Checksum -Path $partPath -Checksums $Checksums
            $source = [IO.File]::OpenRead($partPath)
            try { $source.CopyTo($destination) } finally { $source.Dispose() }
            Remove-Item -LiteralPath $partPath -Force
        }
    } finally {
        $destination.Dispose()
    }
}

function Invoke-CheckedNative([string]$Executable, [string[]]$Arguments) {
    & $Executable @Arguments
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) { throw "$Executable $($Arguments -join ' ') failed with exit code $exitCode" }
}

function Test-BuildWorldRunning([string]$Cli) {
    $output = @(& $Cli status 2>&1)
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) { throw "$Cli status failed with exit code $exitCode`: $($output -join [Environment]::NewLine)" }
    return [bool]($output -match 'is running')
}

function Test-AutostartRegistered {
    $null = & schtasks.exe /Query /TN "BuildWorld Server" 2>$null
    $exitCode = $LASTEXITCODE
    if ($exitCode -eq 0) { return $true }
    if ($exitCode -eq 1) { return $false }
    throw "Cannot inspect the BuildWorld scheduled task (schtasks exit code $exitCode)"
}

function Stop-AutostartWithoutCli {
    $output = @(& schtasks.exe /End /TN "BuildWorld Server" 2>&1)
    $exitCode = $LASTEXITCODE
    if ($exitCode -eq 0) { return $true }
    if ($exitCode -eq 1) { return $false }
    throw "Cannot stop the existing BuildWorld scheduled task (schtasks exit code $exitCode): $($output -join [Environment]::NewLine)"
}

function Add-UserPath([string]$Directory) {
    $current = [Environment]::GetEnvironmentVariable("Path", "User")
    $entries = @($current -split ';' | Where-Object { $_ })
    if ($entries -notcontains $Directory) {
        [Environment]::SetEnvironmentVariable("Path", (($entries + $Directory) -join ';'), "User")
    }
    if (($env:Path -split ';') -notcontains $Directory) { $env:Path = "$Directory;$env:Path" }
}

function Add-RollbackError([Collections.Generic.List[string]]$Errors, [string]$Action, [scriptblock]$Operation) {
    try { & $Operation } catch { $Errors.Add("$Action`: $($_.Exception.Message)") }
}

$release = Get-ReleaseVersion
$arch = Get-Architecture
$asset = "buildworld-windows-$arch.zip"
$temporary = Join-Path ([IO.Path]::GetTempPath()) ("buildworld-" + [guid]::NewGuid())
$archive = Join-Path $temporary $asset
$checksumsPath = Join-Path $temporary "checksums.txt"
$staging = Join-Path $temporary "staging"
$rotated = $false
$installedNew = $false
$wasRunning = $false
$hadAutostart = $false
$oldCli = Join-Path $InstallDir "buildworld.exe"

New-Item -ItemType Directory -Force -Path $temporary | Out-Null
try {
    Write-Host "Downloading BuildWorld v$release for Windows $arch ..."
    Save-ReleaseAsset -Release $release -Asset "checksums.txt" -Destination $checksumsPath
    $checksumsText = [IO.File]::ReadAllText($checksumsPath, [Text.Encoding]::UTF8)
    $checksums = ConvertTo-ChecksumMap $checksumsText
    Get-BundleArchive -Release $release -Asset $asset -Archive $archive -Checksums $checksums -Temporary $temporary
    Assert-Checksum -Path $archive -Checksums $checksums
    Expand-Archive -LiteralPath $archive -DestinationPath $staging -Force

    $hadAutostart = Test-AutostartRegistered
    if (Test-Path -LiteralPath $oldCli -PathType Leaf) {
        $wasRunning = Test-BuildWorldRunning $oldCli
        if ($wasRunning) {
            Write-Host "Pausing running BuildWorld service ..."
            Invoke-CheckedNative -Executable $oldCli -Arguments @("pause")
            if (Test-BuildWorldRunning $oldCli) { throw "BuildWorld is still running after pause" }
        }
    } elseif ($hadAutostart) {
        Write-Host "Stopping registered BuildWorld service ..."
        $wasRunning = Stop-AutostartWithoutCli
    }

    New-Item -ItemType Directory -Force -Path (Split-Path $InstallDir -Parent) | Out-Null
    if (Test-Path -LiteralPath $InstallDir) {
        Remove-Item -LiteralPath $PreviousDir -Recurse -Force -ErrorAction SilentlyContinue
        Move-Item -LiteralPath $InstallDir -Destination $PreviousDir
        $rotated = $true
    }
    Move-Item -LiteralPath $staging -Destination $InstallDir
    $installedNew = $true
    Add-UserPath $InstallDir

    $newCli = Join-Path $InstallDir "buildworld.exe"
    if ($NoAutostart) {
        if (Test-AutostartRegistered) {
            Invoke-CheckedNative -Executable $newCli -Arguments @("disable-autostart")
        }
    } else {
        Invoke-CheckedNative -Executable $newCli -Arguments @("enable-autostart")
    }
    Invoke-CheckedNative -Executable $newCli -Arguments @("start")

    if ($rotated) { Write-Host "Updated and started BuildWorld v$release. Previous bundle: $PreviousDir" }
    else { Write-Host "Installed and started BuildWorld v$release." }
    Write-Host "Commands: buildworld status | pause | resume | restart"
    Write-Host "Reset root safely: `$env:BUILDWORLD_ROOT_PASSWORD='...'; `$env:BUILDWORLD_ROOT_PASSWORD | buildworld reset-root-password --password-stdin"
} catch {
    $installationError = $_.Exception.Message
    $rollbackErrors = [Collections.Generic.List[string]]::new()
    if ($installedNew -and (Test-Path -LiteralPath (Join-Path $InstallDir "buildworld.exe") -PathType Leaf)) {
        $newCli = Join-Path $InstallDir "buildworld.exe"
        Add-RollbackError $rollbackErrors "stop failed installation" {
            Invoke-CheckedNative -Executable $newCli -Arguments @("pause")
        }
        if (-not $hadAutostart) {
            Add-RollbackError $rollbackErrors "remove failed installation autostart" {
                if (Test-AutostartRegistered) {
                    Invoke-CheckedNative -Executable $newCli -Arguments @("disable-autostart")
                }
            }
        }
    }
    if ($installedNew -and (Test-Path -LiteralPath $InstallDir)) {
        Add-RollbackError $rollbackErrors "remove failed bundle" {
            Remove-Item -LiteralPath $InstallDir -Recurse -Force
        }
    }
    if ($rotated -and (Test-Path -LiteralPath $PreviousDir)) {
        Add-RollbackError $rollbackErrors "restore previous bundle" {
            Move-Item -LiteralPath $PreviousDir -Destination $InstallDir
        }
        $restoredCli = Join-Path $InstallDir "buildworld.exe"
        if (Test-Path -LiteralPath $restoredCli -PathType Leaf) {
            if ($hadAutostart -and -not (Test-AutostartRegistered)) {
                Add-RollbackError $rollbackErrors "restore previous autostart" {
                    Invoke-CheckedNative -Executable $restoredCli -Arguments @("enable-autostart")
                    if (-not $wasRunning) { Invoke-CheckedNative -Executable $restoredCli -Arguments @("pause") }
                }
            }
            if ($wasRunning) {
                Add-RollbackError $rollbackErrors "restart previous BuildWorld" {
                    if ($hadAutostart) { Invoke-CheckedNative -Executable $restoredCli -Arguments @("enable-autostart") }
                    Invoke-CheckedNative -Executable $restoredCli -Arguments @("start")
                }
            }
        }
    } elseif ($wasRunning -and (Test-Path -LiteralPath $oldCli -PathType Leaf)) {
        Add-RollbackError $rollbackErrors "restart unchanged previous BuildWorld" {
            if ($hadAutostart) { Invoke-CheckedNative -Executable $oldCli -Arguments @("enable-autostart") }
            Invoke-CheckedNative -Executable $oldCli -Arguments @("start")
        }
    }
    $rollbackSummary = if ($rollbackErrors.Count -gt 0) { " Rollback issues: $($rollbackErrors -join '; ')" } elseif ($rotated) { " The previous bundle was restored." } else { "" }
    throw "BuildWorld installation failed: $installationError.$rollbackSummary"
} finally {
    Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}

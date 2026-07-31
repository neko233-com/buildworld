[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = 'High')]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v?\d+\.\d+\.\d+$')]
    [string]$Version,

    [switch]$Publish,
    [switch]$ReplaceExisting,
    [switch]$PruneOtherReleases,

    [ValidatePattern('^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$')]
    [string]$Repository = 'neko233-com/buildworld'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (Test-Path variable:PSNativeCommandUseErrorActionPreference) {
    $PSNativeCommandUseErrorActionPreference = $false
}
$Version = if ($Version.StartsWith('v')) { $Version } else { "v$Version" }
$remote = 'origin'
$sourceBranch = 'main'
$githubRepository = "github.com/$Repository"
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$output = Join-Path $repoRoot "release\$Version"

function Assert-Command([string]$Name) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command is unavailable: $Name"
    }
}

function Invoke-Checked([string]$Command, [string[]]$Arguments) {
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Command failed with exit code ${LASTEXITCODE}: $($Arguments -join ' ')"
    }
}

function Get-CheckedOutput([string]$Command, [string[]]$Arguments) {
    $output = & $Command @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "$Command failed with exit code ${LASTEXITCODE}: $($Arguments -join ' ')`n$($output -join [Environment]::NewLine)"
    }
    return ($output -join [Environment]::NewLine).Trim()
}

function Get-RemoteTagCommit([string]$Tag) {
    $output = & git -C $repoRoot ls-remote --tags $remote "refs/tags/$Tag" "refs/tags/$Tag^{}" 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to inspect $remote tag ${Tag}: $($output -join [Environment]::NewLine)"
    }
    $lines = @($output | ForEach-Object { [string]$_ } | Where-Object { $_ })
    if ($lines.Count -eq 0) { return $null }
    $peeled = $lines | Where-Object { $_ -match "refs/tags/$([regex]::Escape($Tag))\^\{\}$" } | Select-Object -First 1
    $selected = if ($peeled) { $peeled } else { $lines[0] }
    return (($selected -split '\s+')[0]).Trim()
}

function Assert-ReleaseAssets([string]$Tag, [object[]]$LocalAssets, [bool]$ExpectedDraft) {
    $release = (Get-CheckedOutput gh @(
        'release', 'view', $Tag,
        '--repo', $githubRepository,
        '--json', 'assets,isDraft'
    )) | ConvertFrom-Json
    if ([bool]$release.isDraft -ne $ExpectedDraft) {
        $expectedState = if ($ExpectedDraft) { 'draft' } else { 'published' }
        throw "Release $Tag is not in expected state: $expectedState"
    }
    $remoteAssets = @($release.assets)
    if ($remoteAssets.Count -ne $LocalAssets.Count) {
        throw "Remote release $Tag has $($remoteAssets.Count) assets; expected $($LocalAssets.Count)."
    }
    $remoteByName = @{}
    foreach ($asset in $remoteAssets) {
        $name = [string]$asset.name
        if ($remoteByName.ContainsKey($name)) { throw "Remote release contains duplicate asset: $name" }
        $remoteByName[$name] = $asset
    }
    foreach ($asset in $LocalAssets) {
        if (-not $remoteByName.ContainsKey($asset.Name)) {
            throw "Remote release is missing asset: $($asset.Name)"
        }
        $remoteAsset = $remoteByName[$asset.Name]
        if ([long]$remoteAsset.size -ne $asset.Length) {
            throw "Remote asset size mismatch: $($asset.Name)"
        }
        $localDigest = 'sha256:' + (Get-FileHash -LiteralPath $asset.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        if ([string]$remoteAsset.digest -ne $localDigest) {
            throw "Remote asset digest mismatch: $($asset.Name)"
        }
    }
    return $release
}

foreach ($command in @('git', 'gh', 'go', 'node', 'npm')) {
    Assert-Command $command
}

# This performs every local gate before creating all six platform bundles.
$requestedWhatIf = $WhatIfPreference
try {
    # WhatIf applies only to remote publication. Local gates and package
    # generation must still run normally and produce verifiable output.
    $WhatIfPreference = $false
    & (Join-Path $PSScriptRoot 'release-local.ps1') -Version $Version
    if (-not $?) {
        throw 'Local release build failed.'
    }
} finally {
    $WhatIfPreference = $requestedWhatIf
}

$requiredArchives = @(
    'buildworld-windows-amd64.zip',
    'buildworld-windows-arm64.zip',
    'buildworld-linux-amd64.tar.gz',
    'buildworld-linux-arm64.tar.gz',
    'buildworld-darwin-amd64.tar.gz',
    'buildworld-darwin-arm64.tar.gz'
)
foreach ($name in @($requiredArchives + 'install.ps1' + 'install.sh' + 'checksums.txt')) {
    $path = Join-Path $output $name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Release output is incomplete: missing $path"
    }
}

$checksumPath = Join-Path $output 'checksums.txt'
$expected = @{}
foreach ($line in Get-Content -LiteralPath $checksumPath) {
    if ($line -notmatch '^(?<hash>[0-9a-f]{64})  (?<name>.+)$') {
        throw "Malformed checksum line: $line"
    }
    if ($expected.ContainsKey($Matches.name)) {
        throw "Duplicate checksum entry: $($Matches.name)"
    }
    $expected[$Matches.name] = $Matches.hash
}

$payloadAssets = @(Get-ChildItem -LiteralPath $output -File | Where-Object { $_.Name -ne 'checksums.txt' } | Sort-Object Name)
if ($payloadAssets.Count -eq 0) {
    throw 'Release output contains no upload assets.'
}
foreach ($asset in $payloadAssets) {
    if (-not $expected.ContainsKey($asset.Name)) {
        throw "Checksum entry missing for $($asset.Name)"
    }
    $actual = (Get-FileHash -LiteralPath $asset.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected[$asset.Name]) {
        throw "Checksum mismatch for $($asset.Name)"
    }
}
if ($expected.Count -ne $payloadAssets.Count) {
    throw "checksums.txt contains $($expected.Count) entries, but $($payloadAssets.Count) payload assets exist."
}

$multipartAssets = @($payloadAssets | Where-Object {
    $_.Name -in @('install.ps1', 'install.sh') -or $_.Name -match '\.part\d{3}$'
})
foreach ($archive in $requiredArchives) {
    $partPattern = '^' + [regex]::Escape($archive) + '\.part\d{3}$'
    if (@($multipartAssets | Where-Object { $_.Name -match $partPattern }).Count -eq 0) {
        throw "Multipart upload assets are missing for $archive"
    }
}
# Canonical archives remain local and in checksums.txt. GitHub receives only
# <=900 KiB parts so the installer works through restrictive upload proxies.
$uploadAssets = @($multipartAssets + (Get-Item -LiteralPath $checksumPath))
Write-Host "Local release verified: $Version, $($uploadAssets.Count) upload assets."

if (-not $Publish) {
    Write-Host 'Dry run complete. No Git tag, release, asset, or remote setting was changed.'
    return
}

$operation = "publish $Version from local bundles"
if ($ReplaceExisting) { $operation += '; replace the existing release/tag if present' }
if ($PruneOtherReleases) { $operation += '; delete every other GitHub Release (source tags remain)' }
if (-not $PSCmdlet.ShouldProcess("$Repository GitHub Releases", $operation)) {
    Write-Host 'Publication preview complete. No remote state was changed.'
    return
}

Invoke-Checked gh @('auth', 'status', '--hostname', 'github.com')
$status = Get-CheckedOutput git @('-C', $repoRoot, 'status', '--porcelain', '--untracked-files=all')
if ($status) {
    throw 'Refusing to publish from a dirty worktree. Commit or remove every source change first.'
}
$branch = Get-CheckedOutput git @('-C', $repoRoot, 'branch', '--show-current')
if ($branch -ne $sourceBranch) {
    throw "Refusing to publish from '$branch'; expected '$sourceBranch'."
}
$allowedRemoteUrls = @(
    "https://github.com/$Repository",
    "https://github.com/$Repository.git",
    "git@github.com:$Repository",
    "git@github.com:$Repository.git",
    "ssh://git@github.com/$Repository",
    "ssh://git@github.com/$Repository.git"
)
$configuredRemoteUrls = @(
    @{ Label = 'fetch'; Arguments = @('-C', $repoRoot, 'remote', 'get-url', '--all', $remote) },
    @{ Label = 'push'; Arguments = @('-C', $repoRoot, 'remote', 'get-url', '--push', '--all', $remote) }
)
foreach ($configuredRemote in $configuredRemoteUrls) {
    $remoteUrlText = Get-CheckedOutput git $configuredRemote.Arguments
    $urls = @($remoteUrlText -split '\r?\n' | Where-Object { $_.Trim() })
    if ($urls.Count -eq 0) {
        throw "Remote '$remote' has no $($configuredRemote.Label) URL."
    }
    foreach ($url in $urls) {
        $normalizedUrl = $url.Trim().TrimEnd('/')
        if (-not ($allowedRemoteUrls -icontains $normalizedUrl)) {
            throw "Remote '$remote' has an unapproved $($configuredRemote.Label) URL: $url"
        }
    }
}

Invoke-Checked git @('-C', $repoRoot, 'fetch', '--no-tags', $remote, "+refs/heads/${sourceBranch}:refs/remotes/${remote}/${sourceBranch}")
$head = Get-CheckedOutput git @('-C', $repoRoot, 'rev-parse', 'HEAD')
$remoteHead = Get-CheckedOutput git @('-C', $repoRoot, 'rev-parse', "refs/remotes/$remote/$sourceBranch")
if ($head -ne $remoteHead) {
    throw "Local HEAD $head does not equal $remote/$sourceBranch $remoteHead. Push reviewed source first."
}

$releases = (Get-CheckedOutput gh @('release', 'list', '--repo', $githubRepository, '--limit', '1000', '--json', 'tagName,isDraft')) | ConvertFrom-Json
$releaseExists = @($releases | Where-Object { [string]$_.tagName -eq $Version }).Count -gt 0
$remoteTagCommit = Get-RemoteTagCommit $Version
if ($releaseExists -and -not $ReplaceExisting) {
    throw "Release $Version already exists. Re-run with -ReplaceExisting after reviewing the replacement."
}
if ($remoteTagCommit -and $remoteTagCommit -ne $head -and -not $ReplaceExisting) {
    throw "Remote tag $Version points to $remoteTagCommit, not current HEAD $head. Re-run with -ReplaceExisting after review."
}

$notes = @"
BuildWorld $Version

Verified, built, and uploaded locally. GitHub Actions was not used.
Includes CLI, server, worker, Web UI, TypeScript SDK, installers, checksums, and Windows/Linux/macOS amd64/arm64 bundles.
"@
$stagingTag = "buildworld-staging-$($Version.Substring(1))-$($head.Substring(0, 12))-$([guid]::NewGuid().ToString('N').Substring(0, 8))"
$stagingTagPushed = $false
$stagingReleaseCreated = $false
$cutoverStarted = $false
$publicationComplete = $false
try {
    Invoke-Checked git @('-C', $repoRoot, 'tag', '--annotate', $stagingTag, '--message', "BuildWorld $Version verified staging", $head)
    Invoke-Checked git @('-C', $repoRoot, 'push', $remote, "refs/tags/${stagingTag}:refs/tags/${stagingTag}")
    $stagingTagPushed = $true
    Invoke-Checked gh @(
        'release', 'create', $stagingTag,
        '--repo', $githubRepository,
        '--verify-tag',
        '--draft',
        '--latest=false',
        '--title', "BuildWorld $Version verified staging",
        '--notes', $notes
    )
    $stagingReleaseCreated = $true

    $batchSize = 20
    for ($offset = 0; $offset -lt $uploadAssets.Count; $offset += $batchSize) {
        $last = [Math]::Min($offset + $batchSize - 1, $uploadAssets.Count - 1)
        $batch = @($uploadAssets[$offset..$last] | ForEach-Object { $_.FullName })
        Invoke-Checked gh (@('release', 'upload', $stagingTag, '--repo', $githubRepository) + $batch)
    }
    [void](Assert-ReleaseAssets $stagingTag $uploadAssets $true)

    # The old public release remains available during the full upload. Only the
    # verified draft reaches this short final-tag cutover.
    $cutoverStarted = $true
    if ($releaseExists) {
        Invoke-Checked gh @('release', 'delete', $Version, '--repo', $githubRepository, '--yes', '--cleanup-tag')
        $remoteTagCommit = $null
    } elseif ($remoteTagCommit -and $remoteTagCommit -ne $head) {
        Invoke-Checked git @('-C', $repoRoot, 'push', $remote, ":refs/tags/$Version")
        $remoteTagCommit = $null
    }

    & git -C $repoRoot rev-parse --verify --quiet "refs/tags/$Version" *> $null
    if ($LASTEXITCODE -eq 0 -and -not $remoteTagCommit) {
        Invoke-Checked git @('-C', $repoRoot, 'tag', '--delete', $Version)
    }
    if (-not $remoteTagCommit) {
        Invoke-Checked git @('-C', $repoRoot, 'tag', '--annotate', $Version, '--message', "BuildWorld $Version", $head)
        Invoke-Checked git @('-C', $repoRoot, 'push', $remote, "refs/tags/${Version}:refs/tags/${Version}")
    }

    Invoke-Checked gh @(
        'release', 'edit', $stagingTag,
        '--repo', $githubRepository,
        '--tag', $Version,
        '--verify-tag',
        '--title', "BuildWorld $Version",
        '--notes', $notes
    )
    $stagingReleaseCreated = $false
    [void](Assert-ReleaseAssets $Version $uploadAssets $true)
    Invoke-Checked gh @('release', 'edit', $Version, '--repo', $githubRepository, '--draft=false', '--latest')
    [void](Assert-ReleaseAssets $Version $uploadAssets $false)
    $publicationComplete = $true
} catch {
    $failure = $_
    if (-not $cutoverStarted) {
        if ($stagingReleaseCreated) {
            & gh release delete $stagingTag --repo $githubRepository --yes --cleanup-tag *> $null
        } elseif ($stagingTagPushed) {
            & git -C $repoRoot push $remote ":refs/tags/$stagingTag" *> $null
        }
        & git -C $repoRoot tag --delete $stagingTag *> $null
        Write-Warning 'Verified staging publication failed; the existing public release was left untouched.'
    } else {
        Write-Warning "Final release cutover failed after verified staging. Inspect draft/tag '$Version' and '$stagingTag' before retrying."
    }
    throw $failure
} finally {
    if ($publicationComplete) {
        if ($stagingTagPushed) {
            & git -C $repoRoot push $remote ":refs/tags/$stagingTag" *> $null
            if ($LASTEXITCODE -ne 0) { Write-Warning "Unable to remove remote staging tag: $stagingTag" }
        }
        & git -C $repoRoot tag --delete $stagingTag *> $null
        if ($LASTEXITCODE -ne 0) { Write-Warning "Unable to remove local staging tag: $stagingTag" }
    }
}

if ($PruneOtherReleases) {
    $releases = (Get-CheckedOutput gh @('release', 'list', '--repo', $githubRepository, '--limit', '1000', '--json', 'tagName')) | ConvertFrom-Json
    foreach ($release in @($releases)) {
        $tag = [string]$release.tagName
        if ($tag -and $tag -ne $Version) {
            Invoke-Checked gh @('release', 'delete', $tag, '--repo', $githubRepository, '--yes')
        }
    }
}

Write-Host "Release published and remotely verified: $Version"

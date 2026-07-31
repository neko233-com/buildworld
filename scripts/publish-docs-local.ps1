[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = 'High')]
param(
    [switch]$Publish,

    [ValidateRange(30, 1800)]
    [int]$PublishTimeoutSeconds = 600
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$remote = 'origin'
$repository = 'neko233-com/buildworld'
$sourceBranch = 'main'
$pagesBranch = 'gh-pages'

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

function Get-RemoteBranchSha([string]$RepoRoot, [string]$Remote, [string]$Branch) {
    $output = & git -C $RepoRoot ls-remote --exit-code --heads $Remote "refs/heads/$Branch" 2>&1
    $exitCode = $LASTEXITCODE
    if ($exitCode -eq 2) {
        return $null
    }
    if ($exitCode -ne 0) {
        throw "Unable to inspect $Remote/$Branch (git exit $exitCode): $($output -join [Environment]::NewLine)"
    }
    $refPattern = '^([0-9a-fA-F]{40,64})\s+refs/heads/' + [regex]::Escape($Branch) + '$'
    $match = @($output | ForEach-Object { [regex]::Match([string]$_, $refPattern) } | Where-Object Success)
    if ($match.Count -ne 1) {
        throw "Unexpected ls-remote response for $Remote/${Branch}: $($output -join [Environment]::NewLine)"
    }
    return $match[0].Groups[1].Value.ToLowerInvariant()
}

function Get-PagesState([string]$Repo) {
    $json = Get-CheckedOutput gh @(
        'api', '--hostname', 'github.com',
        '-H', 'Accept: application/vnd.github+json',
        '-H', 'X-GitHub-Api-Version: 2022-11-28',
        "repos/$Repo/pages"
    )
    return $json | ConvertFrom-Json
}

function Get-LatestPagesBuild([string]$Repo) {
    $arguments = @(
        'api', '--hostname', 'github.com',
        '-H', 'Accept: application/vnd.github+json',
        '-H', 'X-GitHub-Api-Version: 2022-11-28',
        "repos/$Repo/pages/builds/latest"
    )
    $output = & gh @arguments 2>&1
    $exitCode = $LASTEXITCODE
    if ($exitCode -eq 0) {
        return ($output -join [Environment]::NewLine) | ConvertFrom-Json
    }

    $errorText = $output -join [Environment]::NewLine
    if ($errorText -match '(?i)(HTTP\s+404|Not Found)') {
        return $null
    }
    throw "Unable to inspect the latest GitHub Pages build (gh exit ${exitCode}): $errorText"
}

function Request-PagesBuild([string]$Repo) {
    $arguments = @(
        'api', '--hostname', 'github.com', '--method', 'POST',
        '-H', 'Accept: application/vnd.github+json',
        '-H', 'X-GitHub-Api-Version: 2022-11-28',
        "repos/$Repo/pages/builds"
    )
    $output = & gh @arguments 2>&1
    $exitCode = $LASTEXITCODE
    if ($exitCode -eq 0) {
        return
    }

    $errorText = $output -join [Environment]::NewLine
    if ($errorText -match '(?i)(HTTP\s+409|Conflict|build is already queued)') {
        Write-Host 'A GitHub Pages build is already queued; waiting for the published commit.'
        return
    }
    throw "Unable to request the GitHub Pages build (gh exit ${exitCode}): $errorText"
}

function Assert-DocumentationOutput([string]$BuildRoot) {
    if (-not (Test-Path -LiteralPath $BuildRoot -PathType Container)) {
        throw "Documentation build is incomplete: missing $BuildRoot"
    }
    $buildRootItem = Get-Item -LiteralPath $BuildRoot -Force
    if (($buildRootItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "Documentation output root must not be a symbolic link: $BuildRoot"
    }

    $requiredOutput = @(
        (Join-Path $BuildRoot 'index.html'),
        (Join-Path $BuildRoot '404.html'),
        (Join-Path $BuildRoot 'sitemap.xml'),
        (Join-Path $BuildRoot 'zh-Hans\index.html')
    )
    foreach ($path in $requiredOutput) {
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "Documentation build is incomplete: missing $path"
        }
    }

    $rootHtml = Get-Content -LiteralPath (Join-Path $BuildRoot 'index.html') -Raw
    if (-not $rootHtml.Contains('/buildworld/')) {
        throw 'Documentation output does not use the required /buildworld/ base URL.'
    }

    $outputItems = @(Get-ChildItem -LiteralPath $BuildRoot -Recurse -Force)
    $links = @($outputItems | Where-Object {
        ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0
    })
    if ($links.Count -gt 0) {
        throw "GitHub Pages branch output must not contain symbolic links: $($links[0].FullName)"
    }

    $gitMetadata = @($outputItems | Where-Object { $_.Name -ieq '.git' })
    if ($gitMetadata.Count -gt 0) {
        throw "GitHub Pages branch output must not contain Git metadata: $($gitMetadata[0].FullName)"
    }

    $outputFiles = @($outputItems | Where-Object { -not $_.PSIsContainer })
    if ($outputFiles.Count -eq 0) {
        throw 'Documentation build produced no files.'
    }
    Write-Host "Documentation build verified locally: $($outputFiles.Count) files."
}

foreach ($command in @('git', 'node', 'npm')) {
    Assert-Command $command
}

$nodeVersion = Get-CheckedOutput node @('--version')
if ($nodeVersion -notmatch '^v24\.') {
    throw "Documentation publishing requires Node.js 24; found $nodeVersion."
}

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$docsRoot = Join-Path $repoRoot 'docs-site'
$buildRoot = Join-Path $docsRoot 'build'

[void](Get-CheckedOutput git @('-C', $repoRoot, 'check-ref-format', '--branch', $sourceBranch))
[void](Get-CheckedOutput git @('-C', $repoRoot, 'check-ref-format', '--branch', $pagesBranch))

if ($Publish) {
    & (Join-Path $PSScriptRoot 'verify-local.ps1')
    if (-not $?) {
        throw 'Canonical local verification failed.'
    }
} else {
    Push-Location $docsRoot
    try {
        Invoke-Checked npm @('ci')
        Invoke-Checked npm @('run', 'build')
    } finally {
        Pop-Location
    }
}

Assert-DocumentationOutput $buildRoot

if (-not $Publish) {
    Write-Host 'Dry run complete. No Git refs or GitHub remote settings were read or changed.'
    return
}

$publishTarget = "$repository Pages ($pagesBranch branch; public site)"
if (-not $PSCmdlet.ShouldProcess($publishTarget, "publish documentation built from $sourceBranch")) {
    Write-Host 'Publication preview complete. No Git refs or GitHub remote settings were read or changed.'
    return
}

Assert-Command gh

$status = Get-CheckedOutput git @('-C', $repoRoot, 'status', '--porcelain', '--untracked-files=all')
if ($status) {
    throw 'Refusing to publish from a dirty worktree. Commit or remove every source change first.'
}

Assert-Command pwsh
Invoke-Checked pwsh @('-NoProfile', '-File', (Join-Path $PSScriptRoot 'verify-local.ps1'))
Assert-DocumentationOutput $buildRoot
$status = Get-CheckedOutput git @('-C', $repoRoot, 'status', '--porcelain', '--untracked-files=all')
if ($status) {
    throw 'Local verification changed the worktree. Review and commit or remove those changes before publishing.'
}

$currentBranch = Get-CheckedOutput git @('-C', $repoRoot, 'branch', '--show-current')
if ($currentBranch -ne $sourceBranch) {
    throw "Refusing to publish from '$currentBranch'; expected '$sourceBranch'."
}

Invoke-Checked gh @('auth', 'status', '--hostname', 'github.com')
$allowedRemoteUrls = @(
    "https://github.com/$repository",
    "https://github.com/$repository.git",
    "git@github.com:$repository",
    "git@github.com:$repository.git",
    "ssh://git@github.com/$repository",
    "ssh://git@github.com/$repository.git"
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

Invoke-Checked git @(
    '-C', $repoRoot, 'fetch', '--no-tags', $remote,
    "+refs/heads/${sourceBranch}:refs/remotes/${remote}/${sourceBranch}"
)
$sourceSha = Get-CheckedOutput git @('-C', $repoRoot, 'rev-parse', 'HEAD')
$remoteSourceSha = Get-CheckedOutput git @('-C', $repoRoot, 'rev-parse', "refs/remotes/$remote/$sourceBranch")
if ($sourceSha -ne $remoteSourceSha) {
    throw "Local HEAD $sourceSha does not equal $remote/$sourceBranch $remoteSourceSha. Push the reviewed source first."
}

$remotePagesSha = Get-RemoteBranchSha $repoRoot $remote $pagesBranch
if ($remotePagesSha) {
    Invoke-Checked git @(
        '-C', $repoRoot, 'fetch', '--no-tags', $remote,
        "+refs/heads/${pagesBranch}:refs/remotes/${remote}/${pagesBranch}"
    )
    $pagesBase = $remotePagesSha
} else {
    $pagesBase = $sourceSha
}

$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$worktreePath = [IO.Path]::GetFullPath((Join-Path $tempRoot ("buildworld-gh-pages-" + [guid]::NewGuid().ToString('N'))))
if (-not $worktreePath.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Unsafe temporary worktree path: $worktreePath"
}

$worktreeAdded = $false
$payloadPath = $null
try {
    Invoke-Checked git @('-C', $repoRoot, 'worktree', 'add', '--detach', $worktreePath, $pagesBase)
    $worktreeAdded = $true

    Invoke-Checked git @('-C', $worktreePath, 'rm', '-r', '--ignore-unmatch', '--', '.')
    Get-ChildItem -LiteralPath $buildRoot -Force | Copy-Item -Destination $worktreePath -Recurse -Force
    [IO.File]::WriteAllBytes((Join-Path $worktreePath '.nojekyll'), [byte[]]@())
    [IO.File]::WriteAllText(
        (Join-Path $worktreePath '.buildworld-source-commit'),
        "$sourceSha`n",
        [Text.UTF8Encoding]::new($false)
    )

    if (Test-Path -LiteralPath (Join-Path $worktreePath '.github')) {
        throw 'Refusing to publish repository automation files in the Pages branch.'
    }

    Invoke-Checked git @('-C', $worktreePath, 'add', '--all')
    & git -C $worktreePath diff --cached --quiet
    $diffExit = $LASTEXITCODE
    if ($diffExit -eq 1) {
        Invoke-Checked git @(
            '-C', $worktreePath,
            '-c', 'user.name=BuildWorld Docs',
            '-c', 'user.email=buildworld-pages@users.noreply.github.com',
            'commit', '--no-gpg-sign', '-m', "docs: publish $sourceSha"
        )
    } elseif ($diffExit -ne 0) {
        throw "Unable to compare generated Pages output (git exit $diffExit)."
    } else {
        Write-Host 'Generated documentation already matches the gh-pages branch.'
    }
    $publishedSha = Get-CheckedOutput git @('-C', $worktreePath, 'rev-parse', 'HEAD')

    $latestRemotePagesSha = Get-RemoteBranchSha $repoRoot $remote $pagesBranch
    if ($latestRemotePagesSha -ne $remotePagesSha) {
        throw "$remote/$pagesBranch changed during publication; refusing to overwrite it."
    }

    # Normal push only: concurrent branch changes must reject this publication.
    Invoke-Checked git @('-C', $worktreePath, 'push', '--porcelain', $remote, "HEAD:refs/heads/$pagesBranch")

    $payload = @{
        build_type = 'legacy'
        source = @{
            branch = $pagesBranch
            path = '/'
        }
    } | ConvertTo-Json -Depth 3
    $payloadPath = Join-Path $tempRoot ("buildworld-pages-" + [guid]::NewGuid().ToString('N') + '.json')
    [IO.File]::WriteAllText($payloadPath, $payload, [Text.UTF8Encoding]::new($false))

    & gh api --hostname github.com -H 'Accept: application/vnd.github+json' -H 'X-GitHub-Api-Version: 2022-11-28' "repos/$repository/pages" --silent 2>$null
    $pagesExists = $LASTEXITCODE -eq 0
    $method = if ($pagesExists) { 'PUT' } else { 'POST' }
    Invoke-Checked gh @(
        'api', '--hostname', 'github.com', '--method', $method,
        '-H', 'Accept: application/vnd.github+json',
        '-H', 'X-GitHub-Api-Version: 2022-11-28',
        "repos/$repository/pages", '--input', $payloadPath
    )

    $pages = Get-PagesState $repository
    if ($pages.build_type -ne 'legacy' -or $pages.source.branch -ne $pagesBranch -or $pages.source.path -ne '/') {
        throw 'GitHub Pages source does not match the requested legacy gh-pages:/ configuration.'
    }

    # A newly enabled legacy Pages site does not always enqueue its first build
    # when the source branch is pushed before the Pages source is configured.
    Request-PagesBuild $repository

    $deadline = [DateTimeOffset]::UtcNow.AddSeconds($PublishTimeoutSeconds)
    $lastStatus = "waiting for build commit $publishedSha"
    $latestCommit = 'none'
    do {
        $latestBuild = Get-LatestPagesBuild $repository
        if ($null -ne $latestBuild) {
            $commitProperty = $latestBuild.PSObject.Properties['commit']
            $statusProperty = $latestBuild.PSObject.Properties['status']
            $latestCommit = if ($null -ne $commitProperty) { [string]$commitProperty.Value } else { 'unknown' }
            $latestBuildStatus = if ($null -ne $statusProperty) { [string]$statusProperty.Value } else { 'unknown' }

            if ($latestCommit -ieq $publishedSha) {
                $lastStatus = $latestBuildStatus
                if ($lastStatus -eq 'built') {
                    Write-Host "Documentation published from $publishedSha`: $($pages.html_url)"
                    return
                }
                if ($lastStatus -eq 'errored') {
                    $errorMessage = 'no error message returned'
                    $errorProperty = $latestBuild.PSObject.Properties['error']
                    if ($null -ne $errorProperty -and $null -ne $errorProperty.Value) {
                        $messageProperty = $errorProperty.Value.PSObject.Properties['message']
                        if ($null -ne $messageProperty -and $messageProperty.Value) {
                            $errorMessage = [string]$messageProperty.Value
                        }
                    }
                    throw "GitHub Pages failed to deploy commit ${publishedSha}: $errorMessage"
                }
            } else {
                $lastStatus = "waiting for $publishedSha; latest build is $latestCommit ($latestBuildStatus)"
            }
        }
        Start-Sleep -Seconds 5
    } while ([DateTimeOffset]::UtcNow -lt $deadline)

    throw "Timed out waiting for GitHub Pages commit $publishedSha (latest commit: $latestCommit; status: $lastStatus). The branch push succeeded; inspect Pages settings before retrying."
} finally {
    if ($payloadPath -and (Test-Path -LiteralPath $payloadPath)) {
        try {
            Remove-Item -LiteralPath $payloadPath -Force
        } catch {
            Write-Warning "Temporary Pages payload cleanup failed: $payloadPath ($($_.Exception.Message))"
        }
    }
    if ($worktreeAdded) {
        try {
            & git -C $repoRoot worktree remove --force $worktreePath
            if ($LASTEXITCODE -ne 0) {
                Write-Warning "Temporary worktree cleanup failed: $worktreePath"
            }
        } catch {
            Write-Warning "Temporary worktree cleanup failed: $worktreePath ($($_.Exception.Message))"
        }
    }
}

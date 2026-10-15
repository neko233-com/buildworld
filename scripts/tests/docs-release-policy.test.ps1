$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot '../docs-release-policy.ps1')

$source = 'a' * 40
$previous = [pscustomobject]@{ releaseTag = 'v1.16.22'; sourceCommit = $source }
foreach ($case in @(
    @{ Commit = ''; Exists = $false; Repo = 'neko233-com/buildworld'; Expected = $true },
    @{ Commit = $source; Exists = $false; Repo = 'neko233-com/buildworld'; Expected = $false },
    @{ Commit = ''; Exists = $true; Repo = 'neko233-com/buildworld'; Expected = $false },
    @{ Commit = $source; Exists = $true; Repo = 'neko233-com/buildworld'; Expected = $false },
    @{ Commit = ''; Exists = $false; Repo = 'other/buildworld'; Expected = $false }
)) {
    $actual = Test-NewDocsReleaseTag -ExistingTagCommit $case.Commit -ReleaseExists $case.Exists -Repository $case.Repo
    if ($actual -ne $case.Expected) { throw 'Incorrect new-tag documentation trigger.' }
}
foreach ($case in @(
    @{ Tag = 'v1.16.23'; Previous = $null; Expected = 'Publish' },
    @{ Tag = 'v1.16.23'; Previous = $previous; Expected = 'Publish' },
    @{ Tag = 'v1.16.22'; Previous = $previous; Expected = 'AlreadyPublished' },
    @{ Tag = 'v1.17.0'; Previous = $previous; Expected = 'Publish' }
)) {
    $actual = Get-DocsReleaseDecision -ReleaseTag $case.Tag -TagCommit $source -SourceCommit $source -PreviousRelease $case.Previous
    if ($actual -ne $case.Expected) { throw "Unexpected decision for $($case.Tag): $actual" }
}

$rejections = @(
    @{ Tag = ''; Commit = $source; Previous = $null; Error = '*requires a release tag*' },
    @{ Tag = 'main'; Commit = $source; Previous = $null; Error = '*requires a release tag*' },
    @{ Tag = 'buildworld-staging-1.16.23'; Commit = $source; Previous = $null; Error = '*requires a release tag*' },
    @{ Tag = 'v01.16.23'; Commit = $source; Previous = $null; Error = '*requires a release tag*' },
    @{ Tag = 'v1.16.23'; Commit = ''; Previous = $null; Error = '*must resolve*' },
    @{ Tag = 'v1.16.23'; Commit = ('b' * 40); Previous = $null; Error = '*must resolve*' },
    @{ Tag = 'v1.16.21'; Commit = $source; Previous = $previous; Error = '*newer release tag*' },
    @{ Tag = 'v1.16.22'; Commit = $source; Previous = [pscustomobject]@{ releaseTag = 'v1.16.22'; sourceCommit = ('b' * 40) }; Error = '*immutable*' },
    @{ Tag = 'v1.16.23'; Commit = $source; Previous = [pscustomobject]@{ releaseTag = 'main'; sourceCommit = $source }; Error = '*record is invalid*' }
)
foreach ($case in $rejections) {
    $rejected = $false
    try {
        Get-DocsReleaseDecision -ReleaseTag $case.Tag -TagCommit $case.Commit -SourceCommit $source -PreviousRelease $case.Previous | Out-Null
    } catch {
        if ($_.Exception.Message -notlike $case.Error) { throw }
        $rejected = $true
    }
    if (-not $rejected) { throw "Invalid documentation publication was accepted: $($case.Tag)" }
}

# An untagged publish must fail before npm, canonical verification or remote access.
try {
    & (Join-Path $PSScriptRoot '../publish-docs-local.ps1') -Publish -Confirm:$false
    throw 'Untagged publish unexpectedly ran.'
} catch {
    if ($_.Exception.Message -notlike '*requires -ReleaseTag*') { throw }
}
Write-Host 'Documentation release policy regressions passed (19 cases).'

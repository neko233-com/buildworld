function Test-NewDocsReleaseTag {
    param(
        [AllowNull()][string]$ExistingTagCommit,
        [bool]$ReleaseExists,
        [string]$Repository
    )
    return -not $ExistingTagCommit -and -not $ReleaseExists -and $Repository -ieq 'neko233-com/buildworld'
}

function Get-DocsReleaseDecision {
    param(
        [string]$ReleaseTag,
        [string]$TagCommit,
        [string]$SourceCommit,
        [AllowNull()][object]$PreviousRelease
    )

    if ($ReleaseTag -notmatch '^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$') {
        throw 'Documentation publication requires a release tag in vX.Y.Z format; main and staging tags cannot publish.'
    }
    if (-not $TagCommit -or $TagCommit -ne $SourceCommit) {
        throw "Remote release tag $ReleaseTag must resolve to the reviewed source commit $SourceCommit."
    }
    if ($null -eq $PreviousRelease) { return 'Publish' }

    $previousTag = [string]$PreviousRelease.releaseTag
    $previousCommit = [string]$PreviousRelease.sourceCommit
    if ($previousTag -notmatch '^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$' -or
        $previousCommit -notmatch '^[0-9a-f]{40,64}$') {
        throw 'The previous documentation release record is invalid; refusing to overwrite it.'
    }
    if ($ReleaseTag -eq $previousTag) {
        if ($SourceCommit -ne $previousCommit) {
            throw "Documentation for $ReleaseTag already records a different source commit; release tags are immutable."
        }
        return 'AlreadyPublished'
    }
    if ([version]$ReleaseTag.Substring(1) -le [version]$previousTag.Substring(1)) {
        throw "Documentation requires a newer release tag than $previousTag; refusing $ReleaseTag."
    }
    return 'Publish'
}

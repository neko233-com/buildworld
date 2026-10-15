function Wait-DocsPagesBuild {
    param(
        [string]$Commit,
        [int]$TimeoutSeconds = 600,
        [Parameter(Mandatory = $true)][scriptblock]$GetLatest,
        [scriptblock]$Wait = { Start-Sleep -Seconds 5 }
    )
    $deadline = [DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds)
    $latestCommit = 'none'
    $lastStatus = 'waiting'
    do {
        $build = & $GetLatest
        if ($null -ne $build) {
            $latestCommit = [string]$build.commit
            $lastStatus = [string]$build.status
            if ($latestCommit -eq $Commit) {
                if ($lastStatus -eq 'built') { return }
                if ($lastStatus -eq 'errored') {
                    $message = 'no error message returned'
                    if ($null -ne $build.PSObject.Properties['error'] -and $null -ne $build.error -and
                        $null -ne $build.error.PSObject.Properties['message'] -and $build.error.message) {
                        $message = [string]$build.error.message
                    }
                    throw "GitHub Pages failed to deploy commit ${Commit}: $message"
                }
            }
        }
        if ([DateTimeOffset]::UtcNow -lt $deadline) { & $Wait }
    } while ([DateTimeOffset]::UtcNow -lt $deadline)
    throw "Timed out waiting for GitHub Pages commit $Commit (latest commit: $latestCommit; status: $lastStatus). Retry the same release tag to resume this deployment."
}

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. (Join-Path $PSScriptRoot '../docs-pages.ps1')
$sha = 'a' * 40
Wait-DocsPagesBuild -Commit $sha -TimeoutSeconds 0 -GetLatest { [pscustomobject]@{commit = $sha; status = 'built'} } -Wait {}
$script:pageAttempts = 0
Wait-DocsPagesBuild -Commit $sha -TimeoutSeconds 1 -Wait {} -GetLatest {
    $script:pageAttempts++
    if ($script:pageAttempts -eq 1) { return [pscustomobject]@{commit = ('b' * 40); status = 'errored'} }
    if ($script:pageAttempts -eq 2) { return [pscustomobject]@{commit = $sha; status = 'building'} }
    return [pscustomobject]@{commit = $sha; status = 'built'}
}
if ($script:pageAttempts -ne 3) { throw 'Pages verification accepted the wrong commit or an intermediate status.' }
foreach ($case in @(
    @{Build = [pscustomobject]@{commit = $sha; status = 'errored'; error = [pscustomobject]@{message = 'test failure'}}; Error = '*test failure*'},
    @{Build = [pscustomobject]@{commit = ('b' * 40); status = 'built'}; Error = '*Timed out*'}
)) {
    $rejected = $false
    try { Wait-DocsPagesBuild -Commit $sha -TimeoutSeconds 0 -GetLatest { $case.Build } -Wait {} } catch {
        if ($_.Exception.Message -notlike $case.Error) { throw }
        $rejected = $true
    }
    if (-not $rejected) { throw 'An unverified Pages deployment was accepted.' }
}
Write-Host 'Pages exact-commit regressions passed (4 cases).'

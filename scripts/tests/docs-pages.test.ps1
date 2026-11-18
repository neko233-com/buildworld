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
$earlierFailed = [pscustomobject]@{commit = $sha; status = 'errored'; created_at = '2026-11-18T04:35:04Z'}
$laterRunning = [pscustomobject]@{commit = $sha; status = 'building'; created_at = '2026-11-18T04:35:08Z'}
$laterBuilt = [pscustomobject]@{commit = $sha; status = 'built'; created_at = $laterRunning.created_at}
$script:pageAttempts = 0
Wait-DocsPagesBuild -Commit $sha -TimeoutSeconds 1 -Wait {} -GetLatest {
    $script:pageAttempts++
    $later = if ($script:pageAttempts -eq 1) { $laterRunning } else { $laterBuilt }
    Get-NewestDocsPagesBuild -Builds @($earlierFailed, $later)
}
if ($script:pageAttempts -ne 2) { throw 'Pages verification did not wait for the newer run.' }
if ($null -ne (Get-NewestDocsPagesBuild -Builds @())) { throw 'Empty Pages build history returned a build.' }

Write-Host 'Pages exact-commit regressions passed (6 cases).'

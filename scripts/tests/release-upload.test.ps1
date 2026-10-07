$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../release-upload.ps1')

$a = [pscustomobject]@{ Name = 'a.part000'; FullName = 'a'; Length = 42; SHA256 = 'sha256:' + ('a' * 64) }
$b = [pscustomobject]@{ Name = 'a.part001'; FullName = 'b'; Length = 43; SHA256 = 'sha256:' + ('b' * 64) }
$remoteA = [pscustomobject]@{ name = $a.Name; size = $a.Length; digest = $a.SHA256 }
$remoteB = [pscustomobject]@{ name = $b.Name; size = $b.Length; digest = $b.SHA256 }

# Server accepted the first upload but the client received a duplicate-name error.
$script:calls = 0
Invoke-VerifiedReleaseUpload -Assets @($a, $b) -Upload { param($paths) $script:calls++; return 1 } -ListRemote { @($remoteA, $remoteB) } -Wait {}
if ($script:calls -ne 1) { throw 'Matching remote assets were uploaded again.' }

# Partial success retries ONLY the missing asset, with no clobber of verified data.
$script:calls = 0
Invoke-VerifiedReleaseUpload -Assets @($a, $b) -Upload {
    param($paths)
    $script:calls++
    if ($script:calls -eq 1) { return 1 }
    if ($paths.Count -ne 1 -or $paths[0] -ne 'b') { throw 'Retry included an existing verified asset.' }
    return 0
} -ListRemote { @($remoteA) } -Wait {}
if ($script:calls -ne 2) { throw 'Missing asset was not retried.' }

foreach ($bad in @(
    [pscustomobject]@{ name = $a.Name; size = 999; digest = $a.SHA256 },
    [pscustomobject]@{ name = $a.Name; size = $a.Length; digest = $b.SHA256 }
)) {
    $rejected = $false
    try {
        Invoke-VerifiedReleaseUpload -Assets @($a) -Upload { param($paths) return 1 } -ListRemote { @($bad) } -Wait {}
    } catch {
        if ($_.Exception.Message -notlike '*mismatch*') { throw }
        $rejected = $true
    }
    if (-not $rejected) { throw 'Mismatched existing asset was accepted.' }
}

$script:calls = 0
$rejected = $false
try {
    Invoke-VerifiedReleaseUpload -Assets @($a) -Upload { param($paths) $script:calls++; return 1 } -ListRemote { @() } -Wait {}
} catch {
    if ($_.Exception.Message -notlike '*after 3 attempts*') { throw }
    $rejected = $true
}
if (-not $rejected -or $script:calls -ne 3) { throw 'Upload retry was not bounded.' }
Write-Host 'Release upload retry regressions passed (5 cases).'

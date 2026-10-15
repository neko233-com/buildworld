$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../network-read.ps1')
foreach ($failure in @('EOF', 'schannel: failed to receive handshake, SSL/TLS connection failed', 'HTTP 503')) {
    $script:readAttempts = 0
    $result = Invoke-NetworkReadRetry -Wait {} -Read {
        $script:readAttempts++
        if ($script:readAttempts -eq 1) { return [pscustomobject]@{ ExitCode = 1; Output = @($failure) } }
        return [pscustomobject]@{ ExitCode = 0; Output = @('ok') }
    }
    if ($result.ExitCode -ne 0 -or $script:readAttempts -ne 2) { throw 'Transient read was not retried.' }
}
foreach ($failure in @('HTTP 403 forbidden', 'fatal: invalid reference')) {
    $script:readAttempts = 0
    $result = Invoke-NetworkReadRetry -Wait {} -Read { $script:readAttempts++; [pscustomobject]@{ ExitCode = 1; Output = @($failure) } }
    if ($result.ExitCode -ne 1 -or $script:readAttempts -ne 1) { throw 'Permanent read failure was retried.' }
}
$script:readAttempts = 0
$result = Invoke-NetworkReadRetry -Wait {} -Read { $script:readAttempts++; [pscustomobject]@{ ExitCode = 1; Output = @('EOF') } }
if ($result.ExitCode -ne 1 -or $script:readAttempts -ne 3) { throw 'Read retries were not bounded.' }
Write-Host 'Transient network read regressions passed (6 cases).'

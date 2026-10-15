# Use only for remote reads/fetches; never wrap tag pushes or Release mutations.
function Invoke-NetworkReadRetry {
    param(
        [Parameter(Mandatory = $true)][scriptblock]$Read,
        [scriptblock]$Wait = { Start-Sleep -Seconds 2 }
    )
    for ($attempt = 1; $attempt -le 3; $attempt++) {
        $result = & $Read
        $message = $result.Output -join [Environment]::NewLine
        $transient = $message -match '(?i)(\bEOF\b|failed to receive handshake|SSL/TLS connection failed|connection (reset|timed out)|timeout|timed out|HTTP\s+50[234]|service unavailable|bad gateway)'
        if ($result.ExitCode -eq 0 -or -not $transient -or $attempt -eq 3) { return $result }
        Write-Host "Transient network read failed; retrying ($($attempt + 1)/3)."
        & $Wait
    }
}

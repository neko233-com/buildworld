function Assert-GzipMatchesTar {
    param([string]$Archive, [string]$TarPath)
    $stream = $null
    $gzip = $null
    $hash = [Security.Cryptography.SHA256]::Create()
    try {
        $stream = [IO.File]::OpenRead($Archive)
        $gzip = [IO.Compression.GZipStream]::new($stream, [IO.Compression.CompressionMode]::Decompress)
        $actual = [Convert]::ToHexString($hash.ComputeHash($gzip)).ToLowerInvariant()
        $expected = (Get-FileHash -LiteralPath $TarPath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actual -ne $expected) { throw 'Compressed release archive differs from its verified tar content.' }
    } finally {
        if ($null -ne $gzip) { $gzip.Dispose() }
        if ($null -ne $stream) { $stream.Dispose() }
        $hash.Dispose()
    }
}

function Assert-ReleaseTarModes {
    param([string]$TarExecutable, [string]$TarPath)
    $binaries = @('./buildworld', './buildworld-server', './buildworld-worker', './apply-update.sh')
    # Read the original tar directly; this never launches an external gzip.
    $listing = @(& $TarExecutable --force-local -tvf $TarPath.Replace('\', '/') @binaries)
    if ($LASTEXITCODE -ne 0) { throw "tar mode verification failed with exit code $LASTEXITCODE" }
    foreach ($binary in $binaries) {
        $entry = $listing | Where-Object { $_ -match "\s$([regex]::Escape($binary))$" } | Select-Object -First 1
        if ($null -eq $entry -or $entry -notmatch '^-rwxr-xr-x\s') {
            throw "Release archive entry must be executable (0755): $binary; found '$entry'"
        }
    }
}

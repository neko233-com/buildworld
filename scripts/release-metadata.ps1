function Get-ReleaseNotes {
    param([string]$Changelog, [string]$Version)
    $number = $Version.TrimStart('v')
    $pattern = '(?ms)^## \[' + [regex]::Escape($number) + '\][^\r\n]*\r?\n(?<body>.*?)(?=^## |\z)'
    $entry = [regex]::Match($Changelog, $pattern)
    if (-not $entry.Success -or -not $entry.Groups['body'].Value.Trim()) {
        throw "Changelog entry is required for release $Version."
    }
    return "BuildWorld $Version`n`n$($entry.Groups['body'].Value.Trim())`n`nVerified, built, and uploaded locally. GitHub Actions was not used.`nIncludes CLI, server, worker, Web UI, pipeline SDK, installers, checksums, and Windows/Linux/macOS amd64/arm64 bundles."
}

function New-ReleaseMetadata {
    param([string]$Version, [string]$SourceCommit, [string]$GoVersion)
    if ($Version -notmatch '^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$' -or $SourceCommit -notmatch '^[0-9a-f]{40,64}$') {
        throw 'Release metadata requires a canonical release version and source commit.'
    }
    return [ordered]@{
        schemaVersion = 1
        repository = 'https://github.com/neko233-com/buildworld'
        version = $Version
        sourceCommit = $SourceCommit
        goVersion = $GoVersion
        builtAt = [DateTimeOffset]::UtcNow.ToString('o')
        platforms = @('windows/amd64', 'windows/arm64', 'linux/amd64', 'linux/arm64', 'darwin/amd64', 'darwin/arm64')
    }
}

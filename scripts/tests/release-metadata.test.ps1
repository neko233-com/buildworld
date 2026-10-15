$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../release-metadata.ps1')
$changelog = "# Changes`n## [Unreleased]`nFuture`n## [1.16.23] - 2026-10-15`n### Added`n- Release identity.`n## [1.16.22] - 2026-10-14`nOlder changes.`n"
$notes = Get-ReleaseNotes -Changelog $changelog -Version v1.16.23
if (-not $notes.Contains('Release identity.') -or $notes.Contains('Older changes.') -or $notes.Contains('Future')) { throw 'Release notes selected the wrong changelog section.' }
$failed = $false
try { Get-ReleaseNotes -Changelog $changelog -Version v1.16.24 | Out-Null } catch { $failed = $true }
if (-not $failed) { throw 'Release without changelog was accepted.' }
$metadata = (New-ReleaseMetadata -Version v1.16.23 -SourceCommit ('a' * 40) -GoVersion 'go1.26.0' | ConvertTo-Json) | ConvertFrom-Json
if ($metadata.sourceCommit -ne ('a' * 40) -or $metadata.version -ne 'v1.16.23' -or $metadata.platforms.Count -ne 6) { throw 'Release metadata lost its identity or a supported platform.' }
[void][DateTimeOffset]::Parse($metadata.builtAt)
$failed = $false
try { New-ReleaseMetadata -Version main -SourceCommit '' | Out-Null } catch { $failed = $true }
if (-not $failed) { throw 'Invalid release identity was accepted.' }
Write-Host 'Release metadata regressions passed (4 cases).'

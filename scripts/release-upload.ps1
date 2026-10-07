function Invoke-VerifiedReleaseUpload {
    param(
        [Parameter(Mandatory = $true)][object[]]$Assets,
        [Parameter(Mandatory = $true)][scriptblock]$Upload,
        [Parameter(Mandatory = $true)][scriptblock]$ListRemote,
        [scriptblock]$Wait = { Start-Sleep -Seconds 2 }
    )

    $pending = @($Assets)
    for ($attempt = 1; $attempt -le 3; $attempt++) {
        $paths = @($pending | ForEach-Object { $_.FullName })
        $exitCode = & $Upload $paths
        if ($exitCode -eq 0) { return }

        # A request can succeed remotely while its response is lost. Retrying
        # that upload returns 422 because the asset name already exists. Treat
        # only exact size AND SHA-256 matches as completed; never clobber assets.
        $remoteByName = @{}
        foreach ($asset in @(& $ListRemote)) {
            if ($remoteByName.ContainsKey($asset.name)) { throw "Duplicate remote asset: $($asset.name)" }
            $remoteByName[$asset.name] = $asset
        }
        $missing = [System.Collections.Generic.List[object]]::new()
        foreach ($asset in $pending) {
            if (-not $remoteByName.ContainsKey($asset.Name)) {
                $missing.Add($asset)
                continue
            }
            $remoteAsset = $remoteByName[$asset.Name]
            if ([long]$remoteAsset.size -ne [long]$asset.Length -or [string]$remoteAsset.digest -ne [string]$asset.SHA256) {
                throw "Remote asset size or SHA-256 mismatch after upload failure: $($asset.Name)"
            }
        }
        $pending = @($missing.ToArray())
        if ($pending.Count -eq 0) { return }
        if ($attempt -lt 3) { & $Wait }
    }
    throw "Release upload failed after 3 attempts; $($pending.Count) verified assets are still missing."
}

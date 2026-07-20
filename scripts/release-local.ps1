param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v?\d+\.\d+\.\d+$')]
    [string]$Version
)

$ErrorActionPreference = 'Stop'
$Version = if ($Version.StartsWith('v')) { $Version } else { "v$Version" }
$repoRoot = Split-Path -Parent $PSScriptRoot
$output = Join-Path $repoRoot ("release\$Version")
$webRoot = Join-Path $repoRoot 'web'
$partSizeBytes = 900KB
$targets = @(
    @{ Os = 'windows'; Arch = 'amd64'; Archive = 'zip' },
    @{ Os = 'windows'; Arch = 'arm64'; Archive = 'zip' },
    @{ Os = 'linux'; Arch = 'amd64'; Archive = 'tar.gz' },
    @{ Os = 'linux'; Arch = 'arm64'; Archive = 'tar.gz' },
    @{ Os = 'darwin'; Arch = 'amd64'; Archive = 'tar.gz' },
    @{ Os = 'darwin'; Arch = 'arm64'; Archive = 'tar.gz' }
)

if (Test-Path $output) { Remove-Item -LiteralPath $output -Recurse -Force }
New-Item -ItemType Directory -Force -Path $output | Out-Null

function Split-ReleaseAsset([string]$Archive) {
    $parts = [System.Collections.Generic.List[string]]::new()
    $stream = [IO.File]::OpenRead($Archive)
    try {
        $index = 0
        $buffer = New-Object byte[] $partSizeBytes
        while (($read = $stream.Read($buffer, 0, $buffer.Length)) -gt 0) {
            $part = "$Archive.part$($index.ToString('000'))"
            $target = [IO.File]::Open($part, [IO.FileMode]::Create, [IO.FileAccess]::Write)
            try { $target.Write($buffer, 0, $read) } finally { $target.Dispose() }
            $parts.Add($part)
            $index++
        }
    } finally { $stream.Dispose() }
    return $parts
}

Push-Location $webRoot
try { npm ci; npm run build } finally { Pop-Location }

$assets = [System.Collections.Generic.List[string]]::new()
foreach ($target in $targets) {
    $os = $target.Os
    $arch = $target.Arch
    $extension = if ($os -eq 'windows') { '.exe' } else { '' }
    $stage = Join-Path $output "stage-$os-$arch"
    $archive = Join-Path $output "buildworld-$os-$arch.$($target.Archive)"
    Write-Host "Packaging $os/$arch ..."
    New-Item -ItemType Directory -Force -Path (Join-Path $stage 'web') | Out-Null
    $env:GOOS = $os
    $env:GOARCH = $arch
    $ldflags = "-s -w -X github.com/neko233-com/buildworld/internal/cli.version=$Version"
    Push-Location $repoRoot
    try {
        go build -trimpath -buildvcs=false -ldflags $ldflags -o (Join-Path $stage "buildworld$extension") ./cmd/cli
        go build -trimpath -buildvcs=false -ldflags $ldflags -o (Join-Path $stage "buildworld-server$extension") ./cmd/server
        go build -trimpath -buildvcs=false -ldflags $ldflags -o (Join-Path $stage "buildworld-worker$extension") ./cmd/worker
    } finally { Pop-Location }
    Copy-Item -Path (Join-Path $webRoot 'dist') -Destination (Join-Path $stage 'web') -Recurse -Force
    if ($target.Archive -eq 'zip') {
        Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive -Force
    } else {
        & tar -C $stage -czf $archive .
        if ($LASTEXITCODE -ne 0) { throw "tar failed for $os/$arch" }
    }
    $assets.Add($archive)
	# The installer treats the original archive as the canonical checksum while
	# GitHub receives <=900 KiB parts. This survives restrictive proxy upload
	# limits without weakening the one-click installer or verification contract.
	foreach ($part in (Split-ReleaseAsset $archive)) { $assets.Add($part) }
	Remove-Item -LiteralPath $stage -Recurse -Force
}
Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
Copy-Item (Join-Path $PSScriptRoot 'install.ps1') (Join-Path $output 'install.ps1') -Force
Copy-Item (Join-Path $PSScriptRoot 'install.sh') (Join-Path $output 'install.sh') -Force
$assets.Add((Join-Path $output 'install.ps1'))
$assets.Add((Join-Path $output 'install.sh'))

$checksums = foreach ($asset in $assets) {
    "{0}  {1}" -f (Get-FileHash -LiteralPath $asset -Algorithm SHA256).Hash.ToLowerInvariant(), (Split-Path $asset -Leaf)
}
$checksumPath = Join-Path $output 'checksums.txt'
[IO.File]::WriteAllLines($checksumPath, $checksums)
Write-Host "Release assets are ready in $output"
Get-ChildItem $output -File | Select-Object Name, Length

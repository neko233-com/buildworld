param()

$ErrorActionPreference = 'Stop'
if (Test-Path variable:PSNativeCommandUseErrorActionPreference) {
    $PSNativeCommandUseErrorActionPreference = $false
}
$repoRoot = Split-Path -Parent $PSScriptRoot

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Label,
        [Parameter(Mandatory = $true)]
        [scriptblock]$Command
    )

    Write-Host "`n==> $Label"
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$Label failed with exit code $LASTEXITCODE"
    }
}

Push-Location $repoRoot
try {
    $workflowRoot = Join-Path $repoRoot '.github\workflows'
    $workflowFiles = @()
    if (Test-Path -LiteralPath $workflowRoot -PathType Container) {
        $workflowFiles = @(Get-ChildItem -LiteralPath $workflowRoot -Recurse -File | Where-Object {
            $_.Extension -in @('.yml', '.yaml')
        })
    }
    if ($workflowFiles.Count -gt 0) {
        $relativeWorkflows = @($workflowFiles | ForEach-Object {
            [IO.Path]::GetRelativePath($repoRoot, $_.FullName)
        })
        throw "GitHub Actions are forbidden by Rule.md; remove: $($relativeWorkflows -join ', ')"
    }

    # Keep the gate focused on this change. The repository still contains
    # historical Go files which predate gofmt; release work must not silently
    # rewrite unrelated user code.
    $goFiles = @(
        & git diff --name-only --diff-filter=ACMR HEAD -- '*.go'
        if ($LASTEXITCODE -ne 0) { throw "git diff failed with exit code $LASTEXITCODE" }
        & git ls-files --others --exclude-standard -- '*.go'
        if ($LASTEXITCODE -ne 0) { throw "git ls-files failed with exit code $LASTEXITCODE" }
        & git diff-tree --no-commit-id --name-only --diff-filter=ACMR -r HEAD -- '*.go'
        if ($LASTEXITCODE -ne 0) { throw "git diff-tree failed with exit code $LASTEXITCODE" }
    ) | Sort-Object -Unique
    if ($goFiles.Count -gt 0) {
        $unformatted = @(& gofmt -l -- $goFiles)
        if ($LASTEXITCODE -ne 0) { throw "gofmt inspection failed with exit code $LASTEXITCODE" }
        if ($unformatted.Count -gt 0) {
            throw "gofmt is required for: $($unformatted -join ', ')"
        }
    }

    Invoke-Checked 'Go tests' { go test ./... -count=1 }
    Invoke-Checked 'Go vet' { go vet ./... }

    $nodeVersion = (& node --version).Trim()
    if ($LASTEXITCODE -ne 0) { throw "node --version failed with exit code $LASTEXITCODE" }
    if ($nodeVersion -notmatch '^v24\.') {
        throw "Local verification requires Node.js 24; found $nodeVersion"
    }

    Push-Location (Join-Path $repoRoot 'web')
    try {
        Invoke-Checked 'Web dependency install' { npm ci }
        Invoke-Checked 'Web tests' { npm test -- --run }
        Invoke-Checked 'Web lint' { npm run lint }
        Invoke-Checked 'Web production build' { npm run build }
    } finally {
        Pop-Location
    }

    Invoke-Checked 'Pipeline SDK runtime tests' { node --test sdk/pipeline/index.test.mjs }
    $tsc = Join-Path $repoRoot 'web\node_modules\.bin\tsc.cmd'
    if (-not (Test-Path -LiteralPath $tsc -PathType Leaf)) {
        throw "TypeScript compiler not found after npm ci: $tsc"
    }
    Invoke-Checked 'Pipeline SDK strict type tests' {
        & $tsc --noEmit --strict --target ES2022 --module NodeNext --moduleResolution NodeNext --skipLibCheck sdk/pipeline/index.types.test.ts
    }

    Push-Location (Join-Path $repoRoot 'docs-site')
    try {
        Invoke-Checked 'Documentation dependency install' { npm ci }
        Invoke-Checked 'Documentation production build' { npm run build }
    } finally {
        Pop-Location
    }

    $parseFailures = [System.Collections.Generic.List[string]]::new()
    foreach ($script in Get-ChildItem -LiteralPath $PSScriptRoot -Filter '*.ps1' -File) {
        $tokens = $null
        $errors = $null
        [void][System.Management.Automation.Language.Parser]::ParseFile($script.FullName, [ref]$tokens, [ref]$errors)
        foreach ($error in $errors) {
            $parseFailures.Add("$($script.Name): $($error.Message)")
        }
    }
    if ($parseFailures.Count -gt 0) {
        throw "PowerShell syntax validation failed: $($parseFailures -join '; ')"
    }

    $bashCandidates = [System.Collections.Generic.List[string]]::new()
    foreach ($command in @(Get-Command bash -All -ErrorAction SilentlyContinue)) {
        if ($command.Source) { $bashCandidates.Add($command.Source) }
    }
    foreach ($root in @($env:ProgramFiles, ${env:ProgramFiles(x86)})) {
        if ($root) {
            $bashCandidates.Add((Join-Path $root 'Git\bin\bash.exe'))
            $bashCandidates.Add((Join-Path $root 'Git\usr\bin\bash.exe'))
        }
    }
    $bashPath = $null
    foreach ($candidate in @($bashCandidates | Select-Object -Unique)) {
        if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) { continue }
        & $candidate --version *> $null
        if ($LASTEXITCODE -eq 0) {
            $bashPath = $candidate
            break
        }
    }
    if (-not $bashPath) {
        throw 'A functional bash installation is required to validate scripts/install.sh'
    }
    Invoke-Checked 'Shell installer syntax' { & $bashPath -n scripts/install.sh }
    Invoke-Checked 'Git whitespace validation' { git diff HEAD --check }
} finally {
    Pop-Location
}

Write-Host "`nAll local verification gates passed."

[CmdletBinding()]
param(
    [string]$OutputRoot = "build/server-cli",
    [string[]]$Architectures = @("amd64", "arm64")
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\")).Path
$docsPath = Join-Path $repoRoot "docs/server-cli.md"
$skillPath = Join-Path $repoRoot "skills/opscopilot-server/SKILL.md"

if (-not (Test-Path -LiteralPath $docsPath -PathType Leaf)) {
    throw "Required bundle document is missing: $docsPath"
}
if (-not (Test-Path -LiteralPath $skillPath -PathType Leaf)) {
    throw "Required bundle skill is missing: $skillPath"
}

$validArchitectures = @("amd64", "arm64")
foreach ($arch in $Architectures) {
    if ($validArchitectures -notcontains $arch) {
        throw "Unsupported Linux architecture '$arch'; expected amd64 or arm64"
    }
}

$outputPath = Join-Path $repoRoot $OutputRoot
New-Item -ItemType Directory -Force -Path $outputPath | Out-Null
$previousGoos = $env:GOOS
$previousGoarch = $env:GOARCH
$previousCgo = $env:CGO_ENABLED
$previousLocation = Get-Location
try {
    Set-Location -LiteralPath $repoRoot
    $env:GOOS = "linux"
    $env:CGO_ENABLED = "0"
    foreach ($arch in $Architectures) {
        $stage = [IO.Path]::GetFullPath((Join-Path $outputPath "stage-$arch-$([guid]::NewGuid().ToString('N'))"))
        $allowedRoot = [IO.Path]::GetFullPath($outputPath).TrimEnd([IO.Path]::DirectorySeparatorChar) + [IO.Path]::DirectorySeparatorChar
        if (-not $stage.StartsWith($allowedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Stage escaped output directory' }
        New-Item -ItemType Directory -Force -Path $stage | Out-Null
        $env:GOARCH = $arch
        $binary = Join-Path $stage "opscopilot-cli"
        & go build -trimpath -o $binary ./cmd/opscopilot-cli
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed for linux/$arch"
        }

        Copy-Item -LiteralPath $docsPath -Destination (Join-Path $stage "server-cli.md")
        $skillDest = Join-Path $stage "skills/opscopilot-server/SKILL.md"
        New-Item -ItemType Directory -Force -Path (Split-Path $skillDest) | Out-Null
        Copy-Item -LiteralPath $skillPath -Destination $skillDest

        # The archive contains only the independent CLI and its operator docs.
        $archive = Join-Path $outputPath "opscopilot-server-cli-linux-$arch.zip"
        if (Test-Path -LiteralPath $archive) {
            Remove-Item -LiteralPath $archive -Force
        }
        Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $archive -CompressionLevel Optimal
        Remove-Item -LiteralPath $stage -Recurse -Force
        Write-Host "Created $archive"
    }
    Get-ChildItem -LiteralPath $outputPath -Filter 'opscopilot-server-cli-linux-*.zip' -File |
        Sort-Object Name |
        ForEach-Object { "{0}  {1}" -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $_.Name } |
        Set-Content -LiteralPath (Join-Path $outputPath 'SHA256SUMS.txt') -Encoding ascii
}
finally {
    Set-Location -LiteralPath $previousLocation.Path
    $env:GOOS = $previousGoos
    $env:GOARCH = $previousGoarch
    $env:CGO_ENABLED = $previousCgo
}

param(
    [string]$Version = '',
    [string]$JdkHome = $env:JAVA_HOME,
    [string]$OutputDirectory = '',
    [string]$RuntimeDirectory = '',
    [string]$Maven = 'mvn'
)
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'Build the desktop package on Windows x64.' }
if (!$Version) { $Version = (git -C $repo describe --tags --always --dirty).Trim() }
if ($Version -notmatch '^[a-zA-Z0-9._-]+$') { throw 'Invalid build version' }
if (!$OutputDirectory) { $OutputDirectory = Join-Path $repo ('build/desktop/' + $Version + '-' + [guid]::NewGuid().ToString('N').Substring(0,8)) }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $OutputDirectory) { throw "Output directory already exists: $OutputDirectory" }
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
if (!$RuntimeDirectory) {
    if (!$JdkHome) {
        $compiler = Get-Command javac -ErrorAction Stop
        $JdkHome = Split-Path (Split-Path $compiler.Source)
    }
    $release = Get-Content -LiteralPath (Join-Path $JdkHome 'release') -Raw
    if ($release -notmatch '(?m)^JAVA_VERSION="21(?:\.|\+|")' -or $release -notmatch '(?m)^OS_ARCH="(?:amd64|x86_64)"') { throw 'A Windows x64 Java 21 JDK is required.' }
    $RuntimeDirectory = Join-Path $OutputDirectory 'component-source'
    & (Join-Path $repo 'build/jdbc/build.ps1') -JdkHome $JdkHome -OutputDirectory $RuntimeDirectory -Maven $Maven
}
$RuntimeDirectory = [IO.Path]::GetFullPath($RuntimeDirectory)
foreach ($required in @('jdbc-helper.jar','runtime/bin/java.exe','runtime/lib/modules','NOTICE.txt','LICENSE-gson.txt','LICENSE-jaxb-activation.txt')) {
    if (!(Test-Path -LiteralPath (Join-Path $RuntimeDirectory $required) -PathType Leaf)) { throw "Missing runtime file: $required" }
}
$package = Join-Path $OutputDirectory 'package'
New-Item -ItemType Directory -Path $package | Out-Null
Add-Type -AssemblyName System.IO.Compression.FileSystem
$payload = Join-Path $package 'jdbc-runtime.bundle'
[IO.Compression.ZipFile]::CreateFromDirectory($RuntimeDirectory, $payload, [IO.Compression.CompressionLevel]::Optimal, $false)
$digest = (Get-FileHash -LiteralPath $payload -Algorithm SHA256).Hash.ToLowerInvariant()
$previousDev = $env:OPSCOPILOT_DEV_MODE
Push-Location $repo
try {
    $env:OPSCOPILOT_DEV_MODE = $null
    # Never replace a build/bin executable that a running preview may use.
    $intermediateName = 'opscopilot-desktop-' + [guid]::NewGuid().ToString('N') + '.exe'
    & wails build -platform windows/amd64 -o $intermediateName -m -nocolour -ldflags "-X main.Version=$Version -X opscopilot/pkg/desktopruntime.ExpectedSHA256=$digest"
    if ($LASTEXITCODE -ne 0) { throw 'Desktop build failed' }
    # Explicit allowlist: no development credentials, config, or user drivers.
    $intermediatePath = Join-Path $repo "build/bin/$intermediateName"
    Move-Item -LiteralPath $intermediatePath -Destination (Join-Path $package 'opscopilot.exe')
} finally {
    Pop-Location
    $env:OPSCOPILOT_DEV_MODE = $previousDev
}
$zip = Join-Path $OutputDirectory 'opscopilot-windows.zip'
[IO.Compression.ZipFile]::CreateFromDirectory($package, $zip)
$sum = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
Set-Content -LiteralPath (Join-Path $OutputDirectory 'SHA256SUMS.txt') -Value "$sum  opscopilot-windows.zip" -Encoding ascii
Write-Host "Desktop package: $zip"
Write-Host "JDBC payload SHA256: $digest"

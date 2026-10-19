[CmdletBinding()]
param(
    [string]$JdkHome = $env:JAVA_HOME,
    [string]$Maven = 'mvn',
    [string]$OutputDirectory = '',
    [string]$Version = ''
)
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'This entry point builds the Windows x64 desktop package.' }
foreach ($tool in @('go', 'node', 'npm', 'wails', $Maven)) {
    if (!(Get-Command $tool -ErrorAction SilentlyContinue)) { throw "Required build tool not found: $tool" }
}
if (!$JdkHome) {
    $compiler = Get-Command javac -ErrorAction SilentlyContinue
    if (!$compiler) { throw 'Java 21 JDK is required. Set JAVA_HOME or pass -JdkHome.' }
    $JdkHome = Split-Path (Split-Path $compiler.Source)
}
$JdkHome = [IO.Path]::GetFullPath($JdkHome)
foreach ($tool in @('java.exe', 'javac.exe', 'jlink.exe')) {
    if (!(Test-Path -LiteralPath (Join-Path $JdkHome "bin/$tool") -PathType Leaf)) { throw "Missing JDK build tool: $tool" }
}
$jdkRelease = Get-Content -LiteralPath (Join-Path $JdkHome 'release') -Raw
if ($jdkRelease -notmatch '(?m)^JAVA_VERSION="21(?:\.|\+|")') { throw 'Use a Java 21 JDK to build the private runtime.' }
if ($jdkRelease -notmatch '(?m)^OS_ARCH="(?:amd64|x86_64)"') { throw 'Use a Windows x64 JDK for this package.' }
$buildId = (Get-Date -Format 'yyyyMMdd-HHmmss-fff') + '-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
if (!$OutputDirectory) { $OutputDirectory = Join-Path $repo "build/jdbc-output/release-$buildId" }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
# Every build owns a fresh directory. Never clean or replace an existing package.
if (Test-Path -LiteralPath $OutputDirectory) { throw "Output directory already exists: $OutputDirectory" }
Push-Location $repo
$previousDevMode = $env:OPSCOPILOT_DEV_MODE
$previousJavaHome = $env:JAVA_HOME
try {
    if (!$Version) {
        $Version = $env:GITHUB_REF_NAME
        if (!$Version -and (Get-Command git -ErrorAction SilentlyContinue)) {
            $Version = & git describe --tags --always --dirty 2>$null
            if ($LASTEXITCODE -ne 0) { $Version = '' }
        }
        if (!$Version) { $Version = 'dev' }
    }
    if ($Version -notmatch '^[a-zA-Z0-9._+\-]+$') { throw 'Version contains unsupported characters.' }
    $env:OPSCOPILOT_DEV_MODE = $null
    $env:JAVA_HOME = $JdkHome
    $packageDirectory = Join-Path $OutputDirectory 'opscopilot-windows-jdbc'
    New-Item -ItemType Directory -Path $packageDirectory -Force | Out-Null
    Write-Host "[INFO] Building JDBC runtime: $JdkHome"
    & (Join-Path $PSScriptRoot 'build.ps1') -JdkHome $JdkHome -Maven $Maven -OutputDirectory (Join-Path $packageDirectory 'jdbc-runtime')
    # Use a unique intermediate filename; build/bin may contain a running app
    # and runtime data. Never overwrite that app or package the whole directory.
    $intermediateName = "opscopilot-jdbc-$buildId.exe"
    $intermediatePath = Join-Path $repo "build/bin/$intermediateName"
    Write-Host "[INFO] Building production application: $Version"
    & wails build -platform windows/amd64 -o $intermediateName -m -nocolour -ldflags "-X main.Version=$Version"
    if ($LASTEXITCODE -ne 0) { throw 'Wails production build failed.' }
    if (!(Test-Path -LiteralPath $intermediatePath -PathType Leaf)) { throw "Build output missing: $intermediatePath" }
    Move-Item -LiteralPath $intermediatePath -Destination (Join-Path $packageDirectory 'opscopilot.exe')
    # Only repository defaults. Local config.json, file_access.json, connections,
    # secrets, imported drivers, old binaries and backups are not release inputs.
    foreach ($name in @('prompts.json', 'quick_commands.json', 'highlight_rules.json', 'command_whitelist.json')) {
        $source = Join-Path $repo $name
        if (Test-Path -LiteralPath $source -PathType Leaf) { Copy-Item -LiteralPath $source -Destination $packageDirectory }
    }
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'package-readme.txt') -Destination (Join-Path $packageDirectory 'JDBC_README.txt')
    foreach ($required in @('opscopilot.exe', 'jdbc-runtime/jdbc-helper.jar', 'jdbc-runtime/runtime/bin/java.exe', 'jdbc-runtime/NOTICE.txt', 'jdbc-runtime/LICENSE-gson.txt')) {
        if (!(Test-Path -LiteralPath (Join-Path $packageDirectory $required) -PathType Leaf)) { throw "Incomplete package: $required" }
    }
    $zip = Join-Path $OutputDirectory 'opscopilot-windows-jdbc.zip'
    Write-Host '[INFO] Creating complete ZIP package...'
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [IO.Compression.ZipFile]::CreateFromDirectory($packageDirectory, $zip, [IO.Compression.CompressionLevel]::Optimal, $false)
    $hash = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText("$zip.sha256", "$hash  opscopilot-windows-jdbc.zip`n", [Text.Encoding]::ASCII)
    Write-Host "[SUCCESS] Application directory: $packageDirectory"
    Write-Host "[SUCCESS] Complete package: $zip"
    Write-Host "[INFO] ZIP size: $([math]::Round((Get-Item -LiteralPath $zip).Length / 1MB, 2)) MiB"
    Write-Host '[INFO] Extract the entire ZIP, then run opscopilot.exe. No system Java installation is required.'
} finally {
    $env:OPSCOPILOT_DEV_MODE = $previousDevMode
    $env:JAVA_HOME = $previousJavaHome
    Pop-Location
}

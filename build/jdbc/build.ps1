param(
    [string]$JdkHome = $env:JAVA_HOME,
    [string]$OutputDirectory = '',
    [string]$Maven = 'mvn'
)
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
if (!$JdkHome) { $JdkHome = Split-Path (Split-Path (Get-Command javac).Source) }
$JdkHome = [IO.Path]::GetFullPath($JdkHome)
if (!$OutputDirectory) { $OutputDirectory = Join-Path $repo 'build/jdbc-output/jdbc-runtime' }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
# Never delete caller-supplied directories. Existing outputs must be moved explicitly.
if (Test-Path -LiteralPath $OutputDirectory) { throw "Output directory already exists; choose a new directory: $OutputDirectory" }
$javaBin = Join-Path $JdkHome 'bin'
$env:JAVA_HOME = $JdkHome
Push-Location (Join-Path $repo 'jdbc-helper')
try {
    & $Maven --batch-mode --no-transfer-progress -DskipTests package
    if ($LASTEXITCODE -ne 0) { throw 'JDBC helper build failed.' }
} finally { Pop-Location }
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $repo 'jdbc-helper/target/jdbc-helper.jar') -Destination $OutputDirectory
# Imported vendor drivers are not visible to jdeps at release time. Keep standard
# JDBC/XML/naming/security/management modules, including TLS and charset providers.
& (Join-Path $javaBin 'jlink') --add-modules java.sql,java.naming,java.management,java.security.jgss,java.net.http,jdk.crypto.ec,jdk.charsets,jdk.unsupported --strip-debug --no-header-files --no-man-pages --compress=zip-6 --output (Join-Path $OutputDirectory 'runtime')
if ($LASTEXITCODE -ne 0) { throw 'Java runtime build failed.' }
$versionInfo = New-Object System.Diagnostics.ProcessStartInfo
$versionInfo.FileName = Join-Path $javaBin 'java'
$versionInfo.Arguments = '-version'
$versionInfo.UseShellExecute = $false
$versionInfo.CreateNoWindow = $true
$versionInfo.RedirectStandardOutput = $true
$versionInfo.RedirectStandardError = $true
$versionProcess = [Diagnostics.Process]::Start($versionInfo)
try {
    $versionText = $versionProcess.StandardOutput.ReadToEnd() + $versionProcess.StandardError.ReadToEnd()
    $versionProcess.WaitForExit()
    if ($versionProcess.ExitCode -ne 0) { throw 'Failed to read Java runtime version.' }
    Set-Content -LiteralPath (Join-Path $OutputDirectory 'runtime-version.txt') -Value $versionText -Encoding utf8
} finally { $versionProcess.Dispose() }
Copy-Item -LiteralPath (Join-Path $repo 'jdbc-helper/NOTICE.txt') -Destination $OutputDirectory
Copy-Item -LiteralPath (Join-Path $repo 'jdbc-helper/LICENSE-gson.txt') -Destination $OutputDirectory
Add-Type -AssemblyName System.IO.Compression.FileSystem
$helperArchive=[IO.Compression.ZipFile]::OpenRead((Join-Path $OutputDirectory 'jdbc-helper.jar'))
try {
    $licenseEntry=$helperArchive.GetEntry('META-INF/LICENSE.md')
    if (!$licenseEntry) { throw 'Compatibility dependency license is missing from helper JAR.' }
    [IO.Compression.ZipFileExtensions]::ExtractToFile($licenseEntry,(Join-Path $OutputDirectory 'LICENSE-jaxb-activation.txt'))
} finally { $helperArchive.Dispose() }
Write-Host "JDBC runtime component: $OutputDirectory"

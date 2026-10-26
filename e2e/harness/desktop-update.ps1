param(
    [Parameter(Mandatory=$true)][string]$Package,
    [Parameter(Mandatory=$true)][string]$LegacyExecutable,
    [string]$OutputDirectory = ''
)
$ErrorActionPreference = 'Stop'
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../..'))
if (!$OutputDirectory) { $OutputDirectory = Join-Path $repo ('e2e/.runtime/desktop-update-' + [guid]::NewGuid().ToString('N')) }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Choose a new test output directory' }
New-Item -ItemType Directory -Path $OutputDirectory | Out-Null
Add-Type -AssemblyName System.IO.Compression.FileSystem
$source = Join-Path $OutputDirectory 'source'
[IO.Compression.ZipFile]::ExtractToDirectory([IO.Path]::GetFullPath($Package),$source)
$payload = Join-Path $source 'jdbc-runtime.bundle'
$digest = (Get-FileHash -LiteralPath $payload -Algorithm SHA256).Hash.ToLowerInvariant()
$newExeHash = (Get-FileHash -LiteralPath (Join-Path $source 'opscopilot.exe')).Hash
$results = @()

function Stop-TestApp([string]$directory) {
    $expected = [IO.Path]::GetFullPath((Join-Path $directory 'opscopilot.exe'))
    # Exact path comparison prevents touching the user's preview or desktop.
    Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Path -and [string]::Equals($_.Path,$expected,[StringComparison]::OrdinalIgnoreCase) } | Stop-Process -Force
}
function Wait-Ready([string]$directory) {
    $marker = Join-Path $directory "jdbc-runtime/managed/$digest/.bundle-ready"
    $deadline = (Get-Date).AddSeconds(90)
    while ((Get-Date) -lt $deadline) {
        if ((Test-Path -LiteralPath $marker) -and ((Get-Content -LiteralPath $marker -Raw) -eq $digest)) { Start-Sleep -Milliseconds 600; return }
        Start-Sleep -Milliseconds 400
    }
    throw "Runtime was not committed: $directory"
}
function Start-TestApp([string]$directory) {
    Start-Process -FilePath (Join-Path $directory 'opscopilot.exe') -WorkingDirectory $directory -WindowStyle Hidden | Out-Null
}

foreach ($scenario in @('fresh','legacy-without-jdbc','legacy-with-jdbc','damaged-and-retry')) {
    $dir = Join-Path $OutputDirectory $scenario
    New-Item -ItemType Directory -Path $dir | Out-Null
    $sentinels = @{
        'sessions.json' = '[{"id":"test-folder","name":"keep connections","type":"folder"}]'
        'quick_commands.json' = '[]'
        'knowledge/keep.md' = 'Keep this user knowledge'
        'jdbc-drivers/keep.jar' = 'Keep this vendor driver'
        'vault/keep.bin' = 'Keep this credential-store fixture'
    }
    if ($scenario -eq 'legacy-with-jdbc') { $sentinels['jdbc-runtime/jdbc-helper.jar'] = 'Legacy helper must remain unchanged' }
    $before = @{}
    foreach ($name in $sentinels.Keys) {
        $p = Join-Path $dir $name
        New-Item -ItemType Directory -Path (Split-Path $p) -Force | Out-Null
        [IO.File]::WriteAllText($p,$sentinels[$name],[Text.UTF8Encoding]::new($false))
        $before[$name] = (Get-FileHash -LiteralPath $p).Hash
    }
    try {
        if ($scenario.StartsWith('legacy-')) {
            $exe = Join-Path $dir 'opscopilot.exe'
            Copy-Item -LiteralPath $LegacyExecutable -Destination $exe
            $manifest = @{
                extractedDir = $source; appDir = $dir; exePath = $exe; parentPid = 0
                version = 'desktop-update-trial'; logPath = (Join-Path $dir 'update.log')
            }
            $manifestPath = Join-Path $OutputDirectory "$scenario-manifest.json"
            [IO.File]::WriteAllText($manifestPath,($manifest | ConvertTo-Json),[Text.UTF8Encoding]::new($false))
            $process = Start-Process -FilePath $exe -ArgumentList @('--self-update',('"'+$manifestPath+'"')) -WorkingDirectory $dir -WindowStyle Hidden -PassThru
            if (!$process.WaitForExit(60000)) { throw 'Legacy updater did not exit' }
            if ($process.ExitCode -ne 0) { throw "Legacy updater failed: $($process.ExitCode)" }
        } else {
            Copy-Item -Path (Join-Path $source '*') -Destination $dir
            if ($scenario -eq 'damaged-and-retry') {
                # Simulate a legacy updater delivering a truncated component.
                [IO.File]::WriteAllText((Join-Path $dir 'jdbc-runtime.bundle'),'damaged payload')
                [IO.File]::WriteAllText((Join-Path $dir 'opscopilot.exe.old'),'rollback executable fixture')
                [IO.File]::WriteAllText((Join-Path $dir 'update_result.json'),'{"success":true,"version":"trial"}')
                Start-TestApp $dir
                Start-Sleep -Seconds 4
                if (Test-Path -LiteralPath (Join-Path $dir "jdbc-runtime/managed/$digest/.bundle-ready")) { throw 'Damaged payload was activated' }
                if (!(Test-Path -LiteralPath (Join-Path $dir 'opscopilot.exe.old'))) { throw 'Backup was removed before runtime completion' }
                Stop-TestApp $dir
                Copy-Item -LiteralPath $payload -Destination $dir -Force
            }
            Start-TestApp $dir
        }
        Wait-Ready $dir
        if ((Get-FileHash -LiteralPath (Join-Path $dir 'opscopilot.exe')).Hash -ne $newExeHash) { throw 'Wrong executable installed' }
        foreach ($name in $before.Keys) {
            if ((Get-FileHash -LiteralPath (Join-Path $dir $name)).Hash -ne $before[$name]) { throw "User data changed: $name" }
        }
        if ($scenario.StartsWith('legacy-') -and !(Test-Path -LiteralPath (Join-Path $dir 'update.log'))) { throw 'Legacy updater did not run' }
        $results += @{scenario=$scenario;passed=$true;runtimeDigest=$digest;userFilesPreserved=$before.Count}
    } finally { Stop-TestApp $dir }
}
$results | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $OutputDirectory 'results.json') -Encoding utf8
Write-Host "Desktop update validation passed: $OutputDirectory"

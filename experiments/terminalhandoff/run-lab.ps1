param(
    [string]$Distro = 'docker-desktop',
    [switch]$PrepareBash,
    [ValidateRange(1, 10)][int]$Count = 1
)

# Opt-in local fixture only. Never opens a saved/business SSH connection.
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$runtime = Join-Path $repo 'e2e/.runtime/terminal-handoff'
$names = @('GOOS', 'GOARCH', 'CGO_ENABLED', 'WSL_UTF8',
    'OPSCOPILOT_HANDOFF_DISTRO', 'OPSCOPILOT_HANDOFF_FIXTURE',
    'OPSCOPILOT_HANDOFF_SHELLS', 'OPSCOPILOT_HANDOFF_LIB',
    'OPSCOPILOT_HANDOFF_ADDR', 'OPSCOPILOT_HANDOFF_KEEP',
    'OPSCOPILOT_HANDOFF_OWNER_PID', 'OPSCOPILOT_HANDOFF_DESKTOP')
$saved = @{}
foreach ($name in $names) { $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }

Push-Location $repo
try {
    # Desktop preview settings must not leak into the bounded PTY test run.
    foreach ($name in @('OPSCOPILOT_HANDOFF_ADDR', 'OPSCOPILOT_HANDOFF_KEEP',
        'OPSCOPILOT_HANDOFF_OWNER_PID', 'OPSCOPILOT_HANDOFF_DESKTOP')) {
        [Environment]::SetEnvironmentVariable($name, $null, 'Process')
    }
    New-Item -ItemType Directory -Path $runtime -Force | Out-Null
    $env:WSL_UTF8 = '1'
    if ($PrepareBash) {
        & python -X utf8 (Join-Path $PSScriptRoot 'prepare-lab.py')
        if ($LASTEXITCODE -ne 0) { throw 'Preparing the isolated Bash files failed.' }
    }
    $linuxRoot = (& wsl.exe -d $Distro -e wslpath -a $repo | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or -not $linuxRoot.StartsWith('/')) { throw 'Cannot resolve workspace in WSL.' }
    $uid = (& wsl.exe -d $Distro -e id -u | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $uid -ne '0') { throw 'The su fixture requires an isolated WSL root account.' }
    & wsl.exe -d $Distro -e id nobody
    if ($LASTEXITCODE -ne 0) { throw 'The fixture requires an existing nobody account; no account will be created.' }
    $env:GOOS = 'linux'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    & go build -o (Join-Path $runtime 'pty-lab') ./experiments/terminalhandoff/fixture
    if ($LASTEXITCODE -ne 0) { throw 'Building the Linux PTY fixture failed.' }
    # Tests run on the Windows host, not under the cross-build environment.
    foreach ($name in @('GOOS', 'GOARCH', 'CGO_ENABLED')) {
        [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process')
    }
    $env:OPSCOPILOT_HANDOFF_DISTRO = $Distro
    $env:OPSCOPILOT_HANDOFF_FIXTURE = "$linuxRoot/e2e/.runtime/terminal-handoff/pty-lab"
    $env:OPSCOPILOT_HANDOFF_SHELLS = '/bin/sh'
    $env:OPSCOPILOT_HANDOFF_LIB = ''
    if (Test-Path (Join-Path $runtime 'alpine/bin/bash')) {
        $env:OPSCOPILOT_HANDOFF_LIB = "$linuxRoot/e2e/.runtime/terminal-handoff/alpine/usr/lib"
        $env:OPSCOPILOT_HANDOFF_SHELLS += ";$linuxRoot/e2e/.runtime/terminal-handoff/alpine/bin/bash"
    }
    $log = Join-Path $runtime 'validation-repeat.log'
    & go test ./pkg/terminalcontrol ./experiments/terminalhandoff -v "-count=$Count" -timeout=90s *> $log
    $testExit = $LASTEXITCODE
    Get-Content -LiteralPath $log
    if ($testExit -ne 0) { throw "Terminal handoff tests failed. See $log" }
    Write-Output "Validation log: $log"
} finally {
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
    Pop-Location
}

[CmdletBinding()]
param(
    [string]$JdkHome = $env:JAVA_HOME,
    [string]$Maven = 'mvn',
    [string]$OutputDirectory = '',
    [string]$Version = ''
)
# Compatibility entry point: all desktop builds use the same complete package.
& (Join-Path $PSScriptRoot '../../scripts/build-desktop.ps1') @PSBoundParameters

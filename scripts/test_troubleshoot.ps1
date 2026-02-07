param([string]$Problem, [string]$OutputDir)

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

if ([string]::IsNullOrWhiteSpace($Problem)) {
    Write-Host "ERROR: Missing -Problem parameter" -ForegroundColor Red
    exit 1
}

if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    Write-Host "ERROR: Missing -OutputDir parameter" -ForegroundColor Red
    exit 1
}

if (!(Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

$ConclusionFile = Join-Path $OutputDir "conclusion.md"

$Content = "# External Tool Conclusion`n`n## Problem`n`n$Problem`n`n## Received Parameters`n`n- Problem: $Problem`n- OutputDir: $OutputDir`n`n## Test Results`n`nThis is a test output from the external script.`n`n### Detected Issues`n`n- Test issue 1`n- Test issue 2`n`n## Suggested Commands`n`n```bash`ncommand1`ncommand2`n```"

[System.IO.File]::WriteAllText($ConclusionFile, $Content, [System.Text.Encoding]::UTF8)

Write-Host "Test complete" -ForegroundColor Green

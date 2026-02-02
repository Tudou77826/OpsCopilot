$Path = "D:\dev\workspace-go\OpsCopilot\scripts\test_troubleshoot.ps1"
$Content = Get-Content $Path -Raw
$Utf8BomEncoding = New-Object System.Text.UTF8Encoding $true
[System.IO.File]::WriteAllText($Path, $Content, $Utf8BomEncoding)
Write-Host "BOM added successfully"

$Content = @'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
Write-Output "测试外部脚本 - 立即返回"
Write-Output ""
Write-Output "# 外部工具定位结论"
Write-Output ""
Write-Output "## 测试结果"
Write-Output ""
Write-Output "这是一个快速测试脚本，用于验证外部脚本集成功能。"
Write-Output ""
Write-Output "### 检测到的问题"
Write-Output "- 测试问题 1"
Write-Output "- 测试问题 2"
exit 0
'@

$Utf8BomEncoding = New-Object System.Text.UTF8Encoding $true
$Path = "D:\dev\workspace-go\OpsCopilot\scripts\test_troubleshoot.ps1"
[System.IO.File]::WriteAllText($Path, $Content, $Utf8BomEncoding)
Write-Host "File created with UTF-8 BOM encoding"

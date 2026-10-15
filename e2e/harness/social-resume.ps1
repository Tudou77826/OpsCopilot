param([string]$RunDirectory)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$runtimeRoot = Join-Path $repoRoot 'e2e/.runtime'
if (-not $RunDirectory) {
    $reviewRun = Get-ChildItem -LiteralPath $runtimeRoot -Directory -Filter 'social-*' |
        Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName 'evidence/result.json') } |
        Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if (-not $reviewRun) { throw '没有已完成验收的数据目录，请先运行 social:test。' }
    $RunDirectory = $reviewRun.FullName
}
$RunDirectory = (Resolve-Path -LiteralPath $RunDirectory).Path
if (-not $RunDirectory.StartsWith($runtimeRoot + '\', [StringComparison]::OrdinalIgnoreCase)) {
    throw '只能恢复本仓库 e2e/.runtime 下的验收实例。'
}
$appDir = Join-Path $RunDirectory 'app'
$legacyAssets = Join-Path $RunDirectory 'legacy-assets'
if (Test-Path -LiteralPath $legacyAssets) {
    # Keep images referenced by existing review tabs available across local frontend rebuilds.
    $reviewAssets = Join-Path $repoRoot 'frontend/dist/assets'
    New-Item -ItemType Directory -Path $reviewAssets -Force | Out-Null
    Get-ChildItem -LiteralPath $legacyAssets -File -Filter '*.png' | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $reviewAssets $_.Name) -Force
    }
}
$launchStamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$started = @()
function Start-ReviewProcess($Name, $Executable, $Port, $Environment) {
    if (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue) {
        Write-Host "$Name 已有监听端口 $Port，保留现有进程。"
        return
    }
    if (-not (Test-Path -LiteralPath $Executable)) { throw "缺少验收程序：$Executable" }
    $previous = @{}
    try {
        foreach ($key in $Environment.Keys) {
            $previous[$key] = [Environment]::GetEnvironmentVariable($key, 'Process')
            [Environment]::SetEnvironmentVariable($key, $Environment[$key], 'Process')
        }
        $process = Start-Process -FilePath $Executable -WorkingDirectory $RunDirectory -WindowStyle Hidden -PassThru `
            -RedirectStandardOutput (Join-Path $RunDirectory "$Name-resume-$launchStamp.stdout.log") `
            -RedirectStandardError (Join-Path $RunDirectory "$Name-resume-$launchStamp.stderr.log")
        $script:started += @{name=$Name; processId=$process.Id; executable=$Executable}
    } finally {
        foreach ($key in $previous.Keys) { [Environment]::SetEnvironmentVariable($key, $previous[$key], 'Process') }
    }
}
$reviewAdminBytes = New-Object byte[] 32
$reviewRandom = [Security.Cryptography.RandomNumberGenerator]::Create()
try { $reviewRandom.GetBytes($reviewAdminBytes) } finally { $reviewRandom.Dispose() }
Start-ReviewProcess 'social-service' (Join-Path $RunDirectory 'service-center.exe') 34116 @{
    OPS_SERVICE_DATA=(Join-Path $RunDirectory 'service'); OPS_SERVICE_LISTEN='127.0.0.1:34116'
    OPS_SERVICE_PUBLIC_URL='http://127.0.0.1:34116'; OPS_SERVICE_LOCAL_HTTP='true'
    OPS_SERVICE_ADMIN_TOKEN=[Convert]::ToBase64String($reviewAdminBytes)
}
Start-ReviewProcess 'ssh-test' (Join-Path $RunDirectory 'fakessh.exe') 34117 @{
    FAKESSH_ADDR='127.0.0.1:34117'; FAKESSH_SFTP_ROOT=(Join-Path $appDir 'docs')
}
Start-ReviewProcess 'desktop' (Join-Path $appDir 'opscopilot-social.exe') 34115 @{
    devserver='127.0.0.1:34115'; assetdir=(Join-Path $repoRoot 'frontend/dist')
    LLM_API_KEY=''; LLM_BASE_URL='http://127.0.0.1:1/v1'; LLM_MODEL='local-test-no-ai'
}
$started | ConvertTo-Json -Depth 3 | Set-Content -LiteralPath (Join-Path $RunDirectory "resumed-processes-$launchStamp.json") -Encoding UTF8
foreach ($url in @('http://127.0.0.1:34116/healthz', 'http://127.0.0.1:34115/')) {
    $ready = $false
    $deadline = (Get-Date).AddSeconds(15)
    while ((Get-Date) -lt $deadline) {
        try { $response = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 1; $ready = $response.StatusCode -eq 200 } catch { }
        if ($ready) { break }
        Start-Sleep -Milliseconds 300
    }
    if (-not $ready) { throw "服务未就绪：$url；查看 $RunDirectory 下的 resume 日志。" }
}
Write-Host "已恢复 http://127.0.0.1:34115/，保留数据 $RunDirectory。"
Write-Host '各进程独立运行；不启动辅助桌面实例。'

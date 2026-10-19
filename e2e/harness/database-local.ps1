param([int]$Port = 33306, [string]$MySQLDirectory)
$ErrorActionPreference = 'Stop'
$repo = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$run = Join-Path $repo 'e2e/.runtime/database-workspace'
New-Item -ItemType Directory -Force -Path $run | Out-Null
if (-not $MySQLDirectory) {
    $MySQLDirectory = Join-Path $run 'mysql/mysql-8.4.8-winx64'
    if (-not (Test-Path -LiteralPath (Join-Path $MySQLDirectory 'bin/mysqld.exe'))) {
        $archive = Join-Path $run 'mysql-8.4.8.zip'
        if (-not (Test-Path -LiteralPath $archive)) { Invoke-WebRequest -Uri 'https://cdn.mysql.com/archives/mysql-8.4/mysql-8.4.8-winx64.zip' -OutFile $archive }
        Expand-Archive -LiteralPath $archive -DestinationPath (Join-Path $run 'mysql') -Force
    }
}
$MySQLDirectory = (Resolve-Path -LiteralPath $MySQLDirectory).Path
$server = Join-Path $MySQLDirectory 'bin/mysqld.exe'
$client = Join-Path $MySQLDirectory 'bin/mysql.exe'
$data = Join-Path $run 'mysql-data'
$endpointFile = Join-Path $run 'mysql-endpoint.json'
if (Test-Path -LiteralPath $endpointFile) { $endpoint = Get-Content -LiteralPath $endpointFile -Raw | ConvertFrom-Json }
else {
    $endpoint = [pscustomobject]@{ host='127.0.0.1'; port=$Port; database='opscopilot_review'; user='ops_review'; password=[Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(24)); adminPassword=[Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(24)); provisioned=$false }
    $endpoint | ConvertTo-Json | Set-Content -LiteralPath $endpointFile -Encoding utf8
}
if ($endpoint.port -ne $Port) { throw '已有测试实例使用不同端口，请沿用 endpoint 中的端口。' }
if (-not (Test-Path -LiteralPath $data)) {
    & $server --no-defaults --initialize-insecure "--basedir=$MySQLDirectory" "--datadir=$data" --console 2> (Join-Path $run 'mysql-init.log')
    if ($LASTEXITCODE -ne 0) { throw 'MySQL 初始化失败，请检查 mysql-init.log。' }
}
$listener = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($listener) {
    $running = Get-CimInstance Win32_Process -Filter "ProcessId=$($listener[0].OwningProcess)"
    if ($running.ExecutablePath -ne $server -or -not $running.CommandLine.Contains($data)) { throw "端口 $Port 由其他进程使用，保留原进程。" }
} else {
    $args = @('--no-defaults', "--basedir=`"$MySQLDirectory`"", "--datadir=`"$data`"", '--bind-address=127.0.0.1', "--port=$Port", '--mysqlx=0', '--skip-log-bin', '--innodb-buffer-pool-size=64M', '--max-connections=24', '--secure-file-priv=NULL', "--log-error=`"$(Join-Path $run 'mysql-server.log')`"")
    $process = Start-Process -FilePath $server -ArgumentList $args -WindowStyle Hidden -PassThru
    $process.Id | Set-Content -LiteralPath (Join-Path $run 'mysql.pid')
}
$clientConfig = Join-Path $run 'mysql-client.cnf'
@"
[client]
host=127.0.0.1
port=$Port
user=root
password=$($endpoint.adminPassword)
default-character-set=utf8mb4
"@ | Set-Content -LiteralPath $clientConfig -Encoding ascii
$ready = $false
for ($i=0; $i -lt 60; $i++) {
    $socket = New-Object Net.Sockets.TcpClient
    try { $ready = $socket.ConnectAsync('127.0.0.1', $Port).Wait(300) -and $socket.Connected } catch { } finally { $socket.Dispose() }
    if ($ready) { break }; Start-Sleep -Milliseconds 500
}
if (-not $ready) { throw 'MySQL 启动超时，请检查 mysql-server.log。' }
if (-not $endpoint.provisioned) {
    $seed = @"
CREATE DATABASE IF NOT EXISTS opscopilot_review CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'ops_review'@'127.0.0.1' IDENTIFIED BY '$($endpoint.password)';
GRANT ALL PRIVILEGES ON opscopilot_review.* TO 'ops_review'@'127.0.0.1';
USE opscopilot_review;
CREATE TABLE IF NOT EXISTS service_nodes (id BIGINT PRIMARY KEY AUTO_INCREMENT, service_name VARCHAR(100) NOT NULL COMMENT '业务服务', environment VARCHAR(20) NOT NULL, host VARCHAR(100) NOT NULL, status VARCHAR(20) NOT NULL, cpu_percent DECIMAL(5,2), updated_at DATETIME NOT NULL, note TEXT NULL);
CREATE TABLE IF NOT EXISTS incident_records (id BIGINT PRIMARY KEY AUTO_INCREMENT, title VARCHAR(180) NOT NULL, severity VARCHAR(20), status VARCHAR(20), service_name VARCHAR(100), opened_at DATETIME, resolved_at DATETIME NULL, detail TEXT NULL);
CREATE TABLE IF NOT EXISTS query_scratch (id BIGINT PRIMARY KEY AUTO_INCREMENT, label VARCHAR(100), created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
INSERT INTO service_nodes (service_name,environment,host,status,cpu_percent,updated_at,note) VALUES ('order-api','staging','10.20.1.11','healthy',18.50,NOW(),'订单接口'),('payment-worker','staging','10.20.1.12','healthy',24.80,NOW(),NULL),('knowledge-indexer','staging','10.20.1.13','warning',76.20,NOW(),'索引任务正在执行'),('gateway','staging','10.20.1.10','healthy',12.30,NOW(),'');
INSERT INTO incident_records (title,severity,status,service_name,opened_at,resolved_at,detail) VALUES ('连接池接近上限','P2','resolved','order-api',NOW()-INTERVAL 2 DAY,NOW()-INTERVAL 1 DAY,'已调整连接回收策略'),('索引更新耗时增加','P3','investigating','knowledge-indexer',NOW()-INTERVAL 2 HOUR,NULL,'等待下一轮任务完成');
CREATE VIEW service_health AS SELECT service_name,host,status,cpu_percent FROM service_nodes;
ALTER USER 'root'@'localhost' IDENTIFIED BY '$($endpoint.adminPassword)';
"@
    $seed | & $client --no-defaults --protocol=TCP --host=127.0.0.1 "--port=$Port" --user=root --default-character-set=utf8mb4 --batch 2> (Join-Path $run 'mysql-seed.stderr.log')
    if ($LASTEXITCODE -ne 0) { throw '初始化测试库失败，请检查 mysql-seed.stderr.log。' }
    $endpoint.provisioned = $true
    $endpoint | ConvertTo-Json | Set-Content -LiteralPath $endpointFile -Encoding utf8
}
$paginationFixture = @"
USE opscopilot_review;
CREATE TABLE IF NOT EXISTS job_runs (id INT PRIMARY KEY, job_name VARCHAR(100), status VARCHAR(20), duration_ms INT, started_at DATETIME);
INSERT IGNORE INTO job_runs (id,job_name,status,duration_ms,started_at) WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<260) SELECT n,CONCAT('daily-check-',n),IF(MOD(n,17)=0,'failed','completed'),120+MOD(n*37,1500),NOW()-INTERVAL n MINUTE FROM seq;
SELECT VERSION();
"@
$paginationFixture | & $client "--defaults-extra-file=$clientConfig" --batch --skip-column-names
if ($LASTEXITCODE -ne 0) { throw 'MySQL 连接验证失败。' }
Write-Host "独立 MySQL 已就绪：127.0.0.1:$Port / opscopilot_review（仅测试数据）。"

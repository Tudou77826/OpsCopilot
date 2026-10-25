@echo off
setlocal
cd /d "%~dp0"
if not exist "opscopilot.exe" (
    echo Put this file beside opscopilot.exe before running it.
    pause
    exit /b 1
)
tasklist /FI "IMAGENAME eq opscopilot.exe" /NH 2>nul | find /I "opscopilot.exe" >nul
if not errorlevel 1 (
    echo Exit the running OpsCopilot application before this check.
    pause
    exit /b 1
)
echo JDBC direct-memory comparison - local computer only
echo 1. Baseline:   96 MiB direct memory per JDBC connection
echo 2. Comparison: 1024 MiB direct memory per JDBC connection
echo Requested heap maximum stays 96 MiB. Actual JVM settings are in the logs.
echo This setting applies only to this launch. No automatic connection retry.
choice /C 12 /N /M "Choose 1 or 2: "
if errorlevel 3 exit /b 1
if errorlevel 2 (
    set "OPSCOPILOT_JDBC_DIRECT_MEMORY_MB=1024"
) else if errorlevel 1 (
    set "OPSCOPILOT_JDBC_DIRECT_MEMORY_MB=96"
) else (
    exit /b 1
)
start "" /wait "%~dp0opscopilot.exe"
endlocal

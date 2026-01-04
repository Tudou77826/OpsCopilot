@echo off
setlocal

:: ==============================
:: OpsCopilot 构建脚本 (Release)
:: ==============================

echo [INFO] Building OpsCopilot for Production...

:: 检查 wails 是否安装
where wails >nul 2>nul
if %errorlevel% neq 0 (
    echo [ERROR] Wails CLI not found. Please install it first.
    pause
    exit /b 1
)

:: 执行构建
:: 注意：这里不设置 OPSCOPILOT_DEV_MODE，确保日志只输出到文件
wails build

if %errorlevel% neq 0 (
    echo [ERROR] Build failed.
    pause
    exit /b 1
)

echo [SUCCESS] Build complete. Executable is in build/bin/
endlocal

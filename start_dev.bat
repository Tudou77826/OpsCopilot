@echo off
setlocal EnableExtensions

:: ==============================
:: OpsCopilot 启动脚本
:: ==============================

if not defined LLM_BASE_URL set "LLM_BASE_URL=https://huazi.de5.net/v1"
if not defined LLM_MODEL set "LLM_MODEL=gemini-2.5-flash"
if not defined OPSCOPILOT_DEV_MODE set "OPSCOPILOT_DEV_MODE=true"

if exist ".env.local" (
    for /f "usebackq tokens=1,* delims==" %%A in (".env.local") do (
        if /I "%%A"=="LLM_API_KEY" set "LLM_API_KEY=%%B"
        if /I "%%A"=="LLM_BASE_URL" set "LLM_BASE_URL=%%B"
        if /I "%%A"=="LLM_MODEL" set "LLM_MODEL=%%B"
    )
)

if not defined LLM_API_KEY (
    echo [WARN] LLM_API_KEY not set. Set it in environment or .env.local.
)

echo [INFO] Environment variables set.
echo [INFO] Starting OpsCopilot (Dev Mode)...

:: 检查 wails 是否安装
where wails >nul 2>nul
if %errorlevel% neq 0 (
    echo [ERROR] Wails CLI not found. Please install it first or run the compiled binary.
    pause
    exit /b 1
)

:: 启动 Wails 开发模式
wails dev

endlocal

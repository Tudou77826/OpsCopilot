@echo off
setlocal

:: ==============================
:: OpsCopilot 启动脚本
:: ==============================

:: 设置 LLM API Key (请在此处填入您的 Key)
set "LLM_API_KEY=sk-BW5vqaJWdVTWabwQK9ORXekDA2a7RdC8bFULbTN2XS6KRQDC"

:: 设置 LLM Base URL (默认为 DeepSeek，可按需修改)
set "LLM_BASE_URL=https://huazi.de5.net/v1"

:: 设置 LLM 模型 (可选，默认 deepseek-chat)
set "LLM_MODEL=gemini-2.5-flash"

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

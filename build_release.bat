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

:: 复制配置文件到输出目录
echo [INFO] Copying configuration files to build/bin/...
if exist "config.json" (
    copy /Y "config.json" "build\bin\" >nul
    echo [INFO]   - config.json
)
if exist "prompts.json" (
    copy /Y "prompts.json" "build\bin\" >nul
    echo [INFO]   - prompts.json
)
if exist "quick_commands.json" (
    copy /Y "quick_commands.json" "build\bin\" >nul
    echo [INFO]   - quick_commands.json
)
if exist "highlight_rules.json" (
    copy /Y "highlight_rules.json" "build\bin\" >nul
    echo [INFO]   - highlight_rules.json
)

:: 复制桥接脚本模板到输出目录
echo [INFO] Copying bridge script template to build/bin/...
if exist "scripts\troubleshoot_bridge_template.bat" (
    copy /Y "scripts\troubleshoot_bridge_template.bat" "build\bin\" >nul
    echo [INFO]   - troubleshoot_bridge_template.bat
)

echo [SUCCESS] Build complete. Executable is in build/bin/
echo [INFO] Configuration files and bridge script template have been copied to build/bin/
endlocal

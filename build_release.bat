@echo off
setlocal

:: ==============================
:: OpsCopilot 构建脚本 (Release)
:: ==============================

:: CI 环境下跳过 pause
if "%CI%"=="true" set NOPAUSE=true

echo [INFO] Building OpsCopilot for Production...

:: 检查 wails 是否安装
where wails >nul 2>nul
if %errorlevel% neq 0 (
    echo [ERROR] Wails CLI not found. Please install it first.
    if not "%NOPAUSE%"=="true" pause
    exit /b 1
)

:: 执行构建
:: 注意：这里不设置 OPSCOPILOT_DEV_MODE，确保日志只输出到文件
wails build

if %errorlevel% neq 0 (
    echo [ERROR] Build failed.
    if not "%NOPAUSE%"=="true" pause
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
if exist "command_whitelist.json" (
    copy /Y "command_whitelist.json" "build\bin\" >nul
    echo [INFO]   - command_whitelist.json
)
if exist "mcp-config.example.json" (
    copy /Y "mcp-config.example.json" "build\bin\" >nul
    echo [INFO]   - mcp-config.example.json
)

:: 构建 MCP Server
echo [INFO] Building MCP Server...
go build -o "build\bin\mcp-server.exe" ./cmd/mcp-server/
if %errorlevel% neq 0 (
    echo [ERROR] MCP Server build failed.
    if not "%NOPAUSE%"=="true" pause
    exit /b 1
)
echo [INFO]   - mcp-server.exe

:: 构建 FTP 文件管理器
echo [INFO] Building FTP File Manager...
echo [INFO] Building FTP frontend assets...
pushd frontend-ftp
call npm install
if %errorlevel% neq 0 (
    echo [ERROR] FTP frontend dependencies install failed.
    if not "%NOPAUSE%"=="true" pause
    popd
    exit /b 1
)
call npm run build
if %errorlevel% neq 0 (
    echo [ERROR] FTP frontend build failed.
    if not "%NOPAUSE%"=="true" pause
    popd
    exit /b 1
)
popd
if exist "cmd\ftpmanager\static" rmdir /S /Q "cmd\ftpmanager\static"
mkdir "cmd\ftpmanager\static"
xcopy /E /I /Y "frontend-ftp\dist\*" "cmd\ftpmanager\static\" >nul
if %errorlevel% neq 0 (
    echo [ERROR] Copy FTP frontend assets failed.
    if not "%NOPAUSE%"=="true" pause
    exit /b 1
)
go build -tags production -ldflags "-s -w" -o "build\bin\OpsFTP.exe" ./cmd/ftpmanager/
if %errorlevel% neq 0 (
    echo [ERROR] FTP File Manager build failed.
    if not "%NOPAUSE%"=="true" pause
    exit /b 1
)
echo [INFO]   - OpsFTP.exe

echo [SUCCESS] Build complete. Executable is in build/bin/
echo [INFO] Configuration files have been copied to build/bin/
echo [INFO] MCP Server has been built for Claude Code integration.
echo [INFO] FTP File Manager has been built for file management.
endlocal

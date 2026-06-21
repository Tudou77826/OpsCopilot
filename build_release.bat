@echo off
setlocal EnableExtensions

rem ==============================
rem OpsCopilot Release Build Script
rem ==============================

rem Skip pause in CI.
if "%CI%"=="true" set "NOPAUSE=true"

echo [INFO] Building OpsCopilot for Production...

where wails >nul 2>nul
if %errorlevel% neq 0 (
    echo [ERROR] Wails CLI not found. Please install it first.
    if not "%NOPAUSE%"=="true" pause
    exit /b 1
)

rem Extract version: prefer CI tag, fallback to git describe.
if defined GITHUB_REF_NAME (
    set "TAG_VERSION=%GITHUB_REF_NAME%"
) else (
    for /f "delims=" %%v in ('git describe --tags --always --dirty 2^>nul') do set "TAG_VERSION=%%v"
)
if "%TAG_VERSION%"=="" set "TAG_VERSION=dev"
echo [INFO] Building version: %TAG_VERSION%

rem Build the Wails app. Do not set OPSCOPILOT_DEV_MODE for release builds.
wails build -ldflags "-X main.Version=%TAG_VERSION%"
if %errorlevel% neq 0 (
    echo [ERROR] Build failed.
    if not "%NOPAUSE%"=="true" pause
    exit /b 1
)

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
if exist "file_access.json" (
    copy /Y "file_access.json" "build\bin\" >nul
    echo [INFO]   - file_access.json
)

echo [SUCCESS] Build complete. Executable is in build/bin/
echo [INFO] Configuration files have been copied to build/bin/
echo [INFO] CLI mode available: opscopilot.exe exec/diagnose/file
endlocal

@echo off
setlocal EnableExtensions

rem Build a complete Windows package with the private Java/JDBC runtime.
rem Optional arguments are forwarded to build\jdbc\release.ps1.
where pwsh >nul 2>nul
if %errorlevel% equ 0 (
    pwsh -NoProfile -ExecutionPolicy Bypass -File "%~dp0build\jdbc\release.ps1" %*
) else (
    powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0build\jdbc\release.ps1" %*
)
set "BUILD_RESULT=%errorlevel%"
if not "%BUILD_RESULT%"=="0" echo [ERROR] JDBC release build failed. See the error above.
if not "%CI%"=="true" if not "%NOPAUSE%"=="true" pause
exit /b %BUILD_RESULT%

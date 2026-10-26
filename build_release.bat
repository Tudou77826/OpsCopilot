@echo off
setlocal EnableExtensions
rem Standard desktop delivery includes the private JDBC helper and Java runtime.
where pwsh >nul 2>nul
if %errorlevel% equ 0 (
    pwsh -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\build-desktop.ps1" %*
) else (
    powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\build-desktop.ps1" %*
)
set "BUILD_RESULT=%errorlevel%"
if not "%CI%"=="true" if not "%NOPAUSE%"=="true" pause
exit /b %BUILD_RESULT%

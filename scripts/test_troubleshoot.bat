@echo off
setlocal enabledelayedexpansion

REM Initialize variables
set "PROBLEM="
set "OUTPUT_DIR="

REM Parse command line arguments
:parse_args
if "%~1"=="" goto :end_parse
if "%~1"=="-Problem" (
    set "PROBLEM=%~2"
    shift
    shift
    goto :parse_args
)
if "%~1"=="-OutputDir" (
    set "OUTPUT_DIR=%~2"
    shift
    shift
    goto :parse_args
)
shift
goto :parse_args

:end_parse

REM Validate required parameters
if "%PROBLEM%"=="" (
    echo Error: Missing -Problem parameter
    exit /b 1
)

if "%OUTPUT_DIR%"=="" (
    echo Error: Missing -OutputDir parameter
    exit /b 1
)

REM Create output directory if not exists
if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"

REM Define output file
set "CONCLUSION_FILE=%OUTPUT_DIR%\conclusion.md"

REM Write conclusion to file
(
echo # External Tool Conclusion ^(Test Script^)
echo.
echo ## Problem Description
echo.
echo %PROBLEM%
echo.
echo ## Received Parameters
echo.
echo - **Problem**: %PROBLEM%
echo - **Output Directory**: %OUTPUT_DIR%
echo.
echo ## Test Results
echo.
echo This is the output from the external troubleshooting script.
echo The script successfully received the parameters and created this conclusion file.
echo.
echo ### Detected Issues
echo.
echo - **Issue 1**: High CPU utilization detected on the server
echo - **Issue 2**: Multiple processes consuming excessive CPU resources
echo - **Issue 3**: Possible memory leak contributing to CPU spikes
echo.
echo ## Investigation Steps
echo.
echo 1. Identify top CPU consuming processes using `top` or `htop`
echo 2. Check process details and resource usage
echo 3. Review system logs for related errors
echo 4. Analyze application logs for performance issues
echo.
echo ## Suggested Commands
echo.
echo ```bash
echo # View real-time process statistics
echo top -bc
echo.
echo # Check CPU usage for specific process
echo ps -p PID -o %%cpu,%%mem,cmd
echo.
echo # View system load average
echo uptime
echo ```
echo.
echo ## Root Cause Analysis
echo.
echo Based on the problem description and system analysis, the high CPU utilization
echo is likely caused by one or more application processes not properly releasing
echo resources or entering infinite loops.
) > "%CONCLUSION_FILE%"

endlocal
exit /b 0

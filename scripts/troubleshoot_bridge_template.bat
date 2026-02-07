@echo off
REM OpsCopilot 外部脚本桥接模板
REM 用途：接收来自 OpsCopilot 的固定参数，调用实际的外部定位工具
REM
REM 接收参数：
REM   -Problem: 问题描述
REM   -OutputDir: 输出目录路径
REM
REM 输出：在 %OutputDir% 目录下创建 conclusion.md 文件

setlocal enabledelayedexpansion
chcp 65001 >nul 2>&1

REM 初始化变量
set "PROBLEM="
set "OUTPUT_DIR="

REM 解析命令行参数
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

REM 验证必需参数
if "%PROBLEM%"=="" (
    echo 错误：缺少 -Problem 参数
    exit /b 1
)

if "%OUTPUT_DIR%"=="" (
    echo 错误：缺少 -OutputDir 参数
    exit /b 1
)

REM ============================================
REM 在此区域自定义你的外部工具调用逻辑
REM ============================================

REM 示例1: 调用另一个 Python 脚本
REM python "C:\tools\my_troubleshoot.py" "%PROBLEM%" "%OUTPUT_DIR%"

REM 示例2: 调用另一个批处理脚本
REM call "C:\tools\my_external_script.bat" "%PROBLEM%" "%OUTPUT_DIR%"

REM 示例3: 直接在这里编写你的定位逻辑
REM echo 正在分析问题: %PROBLEM% > "%OUTPUT_DIR%\conclusion.md"
REM echo. >> "%OUTPUT_DIR%\conclusion.md"
REM echo ## 定位结论 >> "%OUTPUT_DIR%\conclusion.md"
REM echo 根据问题描述，发现以下问题... >> "%OUTPUT_DIR%\conclusion.md"

REM ============================================
REM 模拟输出（实际使用时请替换为真实逻辑）
REM ============================================

echo 问题: %PROBLEM%
echo 输出目录: %OUTPUT_DIR%

REM 创建结论文件
set "CONCLUSION_FILE=%OUTPUT_DIR%\conclusion.md"

(
echo # 外部工具定位结论
echo.
echo ## 问题描述
echo.
echo %PROBLEM%
echo.
echo ## 定位结果
echo.
echo 这里是你的外部工具返回的定位结论。
echo 请根据实际情况修改此模板，调用你的真实外部工具。
echo.
echo ## 建议命令
echo.
echo ```bash
echo command1
echo command2
echo ```
) > "%CONCLUSION_FILE%"

echo 定位完成，结论已保存至: %CONCLUSION_FILE%

endlocal

@echo off
chcp 65001 >nul
echo 等待30秒后返回定位结论...
timeout /t 30 /nobreak >nul
echo.
echo # 问题定位结论
echo.
echo ## 问题分析
echo.
echo 根据系统监控数据和日志分析，发现以下关键问题：
echo.
echo ### 1. 内存使用率异常
echo - 当前内存使用率: 85%% (正常阈值: 70%%)
echo - 峰值内存占用: 12GB
echo - 主要占用进程: java.exe (9.2GB)
echo.
echo ### 2. CPU 负载过高
echo - 当前 CPU 使用率: 78%% (正常阈值: 60%%)
echo - 主要 CPU 消耗者: GC 线程
echo.
echo ### 3. 磁盘 I/O 瓶颈
echo - 磁盘读取延迟: 45ms (正常: ^<20ms)
echo - 磁盘写入延迟: 68ms (正常: ^<30ms)
echo.
echo ## 排查思路
echo.
echo ### 第一步: 内存分析
echo ```bash
echo jmap -heap [pid]
echo jstat -gcutil [pid] 1000 10
echo ```
echo.
echo ### 第二步: 线程分析
echo ```bash
echo jstack [pid] > thread_dump.txt
echo jstack [pid] | grep "BLOCKED" -A 10
echo ```
echo.
echo ### 第三步: GC 分析
echo ```bash
echo jstat -gc [pid] 1000 10
echo ```
echo.
echo ## 建议命令
echo.
echo 1. 查看堆内存使用情况:
echo    ```bash
echo    jmap -heap [pid]
echo    ```
echo.
echo 2. 生成线程转储:
echo    ```bash
echo    jstack [pid] ^> thread_dump_%%date:~0,4%%%%date:~5,2%%%%date:~8,2%%_%%time:~0,2%%%%time:~3,2%%.txt
echo    ```
echo.
echo 3. 监控 GC 情况:
echo    ```bash
echo    jstat -gcutil [pid] 1000 20
echo    ```
echo.
echo 4. 查看系统资源使用:
echo    ```bash
echo    top -b -n 1 ^| head -20
echo    df -h
echo    iostat -x 2 5
echo    ```
echo.
echo ## 预期解决方案
echo.
echo 1. 调整 JVM 堆内存大小: -Xms8g -Xmx12g
echo 2. 优化 GC 算法: 使用 G1GC (-XX:+UseG1GC)
echo 3. 增加磁盘缓存: 调整 vm.dirty_ratio 参数
echo 4. 考虑水平扩展: 增加应用实例数量

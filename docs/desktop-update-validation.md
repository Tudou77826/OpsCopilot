# 桌面完整包升级验证

验证日期：2026-10-26。状态：本地实现及 Windows 实测完成，尚未发布 GitHub Release 或部署内网版本。

## 交付物

- 试用版本：`v1.13.0-desktop-update-trial`。
- 完整包：`output/desktop-update-verified-20261026/opscopilot-windows.zip`。
- ZIP SHA-256：`a12d6d07a82984a8d713ae7ab50ab8be4c6688d66f1ad1e78b6504497e9dd702`。
- 根目录内容：`opscopilot.exe`、`jdbc-runtime.bundle`，不含开发配置或用户驱动。
- 组件 SHA-256：`297f3f95b83896d7c12a442f80361a1e4763a67d66b8bd75439dcc5126ae8f66`。
- 自带运行时：Java 21.0.7；首次安装实际启动 Java 和 helper 验证。

## 结果

| 验证项 | 结果与范围 |
| --- | --- |
| 全新解压运行 | 通过，自动安装组件，无系统 Java 配置步骤 |
| 正式旧版 v1.11.4 直接升级 | 通过，运行真实旧 EXE 的 `--self-update`；覆盖没有 JDBC 和已有旧 helper 的目录 |
| 2026-10-23 试用程序升级 | 通过，同样验证旧更新器只复制根目录文件的路径 |
| 新版更新器更新 | 通过，使用最终包中的真实 EXE 执行更新 |
| 损坏载荷与重启恢复 | 通过，损坏组件不提交，更新备份保留；恢复正确载荷并重启后完成准备 |
| 用户文件保留 | 通过，连接树、快捷命令、厂商 JAR、知识文件、凭据样本文件内容保持不变；已有旧 helper 保留 |
| 更新文件复制失败 | 自动测试通过，恢复原 EXE，删除本次引入的残缺文件 |
| 自动恢复失败 | 自动测试通过，结果明确标记未恢复，不误报回滚成功；保留原 EXE 备份 |
| 组件版本不匹配 | 自动测试通过，拒绝回退使用其他版本 helper |
| 路径和内容校验 | 自动测试通过，拒绝越界、重复、保留路径与损坏载荷；已安装文件损坏可重新准备 |
| 升级成功上报 | 自动测试通过，组件尚未就绪时不确认启动成功 |
| JDBC 实际能力 | 使用升级安装出的 Java/helper，连接隔离 MySQL 8.4.8；驱动导入、节点配置、元数据和只读查询、SQL 文件输入及写入拒绝测试通过 |
| 代码回归 | `go test ./...` 通过；最终更新相关改动再次通过 updater、desktopruntime、servicecenter、installguard 和主包测试及相关 `go vet` |
| 桌面构建 | Wails 完整生产构建通过，包含 TypeScript 检查、前端构建和 Go 编译 |
| 更新界面 | 9 项组件测试通过，覆盖挂载前已有错误、重试调用、多个状态视图同步和现有更新界面 |

正式旧版 ZIP 从 v1.11.4 Release 取得并核对 SHA-256：`102a25cf0715921664d725bc55eca36272de2ec0bd0bfa4d33678b3bec3ebbef`。

## 证据位置

最终包的实测记录：

- `e2e/.runtime/desktop-update-verified-v1.11.4-20261026/results.json`：v1.11.4 升级及四类启动场景。
- `e2e/.runtime/desktop-update-verified-current-20261026/results.json`：新更新器及四类启动场景。
- `e2e/.runtime/desktop-update-final-20261026/results.json`：近期试用程序升级。
- `e2e/.runtime/desktop-update-jdbc-live.log`：升级组件的 MySQL 联测。
- `e2e/.runtime/desktop-update-regression.log`：全量 Go 回归。
- `e2e/.runtime/desktop-update-verified-build.log`：最终包构建。

测试运行在独立目录，没有替换正在使用的 34175 预览。测试应用已关闭；临时 MySQL 实例已按进程路径和测试数据目录核对后停止。

## 验证边界

实测覆盖本机 Windows，以及上述两种真实旧程序；不代表逐个验证了全部历史版本或所有 Windows 系统。凭据测试使用样本文件，没有修改真实账户或重建系统 Keyring。数据库联测使用隔离 MySQL，未连接内网 Gauss 环境。

损坏组件的恢复实测使用重新启动；重试按钮的调用和状态同步已通过组件测试，未进行真实桌面点击实测。未做断电／磁盘硬件故障试验。旧更新器自身的异常处理无法由新包修复；旧复制阶段无法恢复时，需要退出应用后重新解压完整包。正式发布与内网安装包上架仍需另行执行。

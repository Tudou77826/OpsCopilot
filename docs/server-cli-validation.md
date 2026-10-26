# 服务端 CLI 自测记录

日期：2026-10-26。范围为独立 `cmd/opscopilot-cli`、共用执行能力及 Linux 可用性，未包含问题平台产品实现。

## 验证结果

| 验证 | 环境与覆盖 | 结果 |
|---|---|---|
| 全仓库 Go 回归 | Windows；`go test ./... -count=1`，48 个有测试的包 | 通过 |
| 静态检查 | 新 CLI、Session、执行、安全、SSH、ACP 包的 `go vet` | 通过 |
| Linux Session | WSL2 Linux amd64，实际运行测试二进制；加密、密钥错误、跨 Session 密文交换、节点独立、刷新凭据、关闭取消与清理 | 通过 |
| Linux 独立 CLI | 多个真实子进程；loopback SSH 密码认证、stdout/stderr、非零退出、单次执行、截断、认证失败、进程间恢复 | 通过 |
| Linux SFTP | `pkg/sftp` 测试服务器；真实非空文件下载、目录逃逸拒绝、大小上限、Session 关闭后删除文件 | 通过 |
| MySQL/JDBC | Windows Java 21、MySQL 测试实例；导入官方 Connector/J，查询、库表和结构、拒绝写入、SQL 错误、32 KiB 裁剪、超时、关闭 | 通过 |
| Linux Java/JDBC | WSL2 Linux、Alpine OpenJDK 21、同一 helper 和官方 JAR，通过测试 TCP 转发连接隔离 MySQL 实例；重复完整数据库用例 | 通过 |
| iCode 联合调用 | 实际 iCode 0.30.1 ACP 进程与原生 `run_skill_script`，确定性本地模型替身；脚本通过 stdin 登记节点，后续独立 CLI 进程复用 Session，检查凭据不回显 | 通过 |
| 阿里云 Linux 实机 | Alibaba Cloud Linux 3.2104 U11、amd64、Python 3.6.8；CLI／Session／SSH／SFTP、独立 MySQL 5.7.40 与 Java 21 JDBC、实际 iCode 0.30.1 原生脚本调用 | 通过，详见下节 |
| CLI 易用性 | Windows 与 WSL2 Linux：所有子命令离线帮助、参数提前报错、JSON hint、驱动适配状态；Windows 隔离 MySQL/JDBC 实测 SQL 文件／stdin 输入及写入拒绝 | 通过 |
| 构建 | Linux amd64、arm64，`CGO_ENABLED=0`，依赖检查无 Wails／前端，包内仅程序、部署说明及服务器 Skill | 通过 |

Linux 实际运行覆盖 amd64；arm64 完成交叉编译与 ELF 架构检查，未在 arm64 实机运行。iCode 联测验证真实工具调用路径，使用模型替身，不代表真实模型的业务定位质量或问题平台的原生 Session 恢复已经验收。

## 阿里云实测

2026-10-26 在 `39.108.107.148` 的专用临时目录实际运行 Linux 二进制，未依赖 WSL 或桌面程序。串行执行 `opscli`、`opssession`、`filetxn`，以及执行边界、输出裁剪、SSH 超时等测试。SSH／SFTP 使用该主机上的 loopback 协议测试服务器，不代表目标业务网络或提权策略已经验收。

数据库复用服务器已有的 MySQL 5.7.40 程序，另建临时数据目录、socket 和随机回环端口；通过 Java 21、当前 JDBC helper 和 Connector/J 9.7.0 完成实际查询、写入拒绝、超时及输出限制验证，耗时约 6.8 秒。系统默认 Java 11 保持原样，临时 Java 21 与测试数据库均随现场清理。

原生 Agent 链路使用校验过发布文件摘要的 [iCode 0.30.1 Linux 离线包](https://github.com/openJiuwen-ai/iCode/releases/tag/v0.30.1)。iCode 加载 Skill、执行 Python 脚本，调用独立 CLI 登记节点，再由后续进程读取同一个加密 Session；同时检查凭据不进入模型上下文和明文状态。模型为本地确定性替身。首次脚本因 Python 版本兼容性未完成，改为兼容系统 Python 3.6 的 subprocess 参数后，整条链路通过，耗时约 7.4 秒；原失败记录与复测记录均保留。

执行入口为 `e2e/harness/server-linux-cloud.py`，接收部署方已准备好的临时运行目录；运行材料包括 Linux 预编译 CLI／测试二进制、独立 iCode、Java 和 JDBC 组件。原始结果和日志已下载到本地 `e2e/.runtime/ali-linux-20261026/run-104719-b37e42/evidence/`。测试进程已确认退出，远程运行目录已删除，临时命令和文件放行策略已撤销；现有业务服务、数据库和系统默认运行环境未修改。

## 实现中的约束检查

- 缺失或损坏的显式部署策略直接失败，不替换为默认文件。
- Session 状态加密，普通列表不回显密码；更新一个节点不影响其他节点，不将旧身份凭据复制给新身份。
- SSH 命令不自动重跑、不降级执行身份；提权凭据仅通过 stdin，不拼入命令字符串。
- 断线和取消保留已取得输出及完成状态，不用客户端通道关闭冒充远端进程停止。
- 收集及返回均有大小限制；UTF-8 裁剪合法，省略内容有机器可读标记。
- 数据库临时配置位于 Session 工作目录，正常退出回收，进程异常后由平台关闭 Session 清理。
- Session 关闭幂等，阻止迟到调用和相同 ID 隐式复活。

## 尚需部署方联合验收

1. 问题平台真实触发、提问、用户延迟答复、Chrys／iCode 模型上下文恢复和结论回传。
2. 目标内网的跳板、账号、提权策略及实际数据库版本／驱动。
3. 数据库非 MySQL 方言：当前通用 JDBC 未适配服务端强制只读事务，明确拒绝，不以取消只读保护代替适配。

复现方法见 [部署与调用](server-cli.md)；独立工具测试不需要修改业务环境。后续亲自审查发现并修正了测试有效性及执行边界缺口，详细复现、修正和复测范围见 [代码审查记录](server-cli-review.md)。

# 普通终端接手实验

这是阶段 1 的隔离验证环境，验证在**现有 SSH PTY / 当前 Shell** 中交接输入权的可行性。控制器现已提取到 `pkg/terminalcontrol`，桌面新增固定场景的接手测试入口；Chrys 和业务环境均未接入。结论与支持边界见 [可行性报告](../../docs/terminal-handoff-feasibility.md)，页面试用见 [接手测试说明](../../docs/terminal-handoff-trial.md)。

## 文件职责

| 文件 | 职责 |
|---|---|
| `controller.go` | 关联 `pkg/terminalcontrol` 的控制器，保证实验验证桌面实际使用的实现 |
| `../../pkg/terminalcontrol/` | 输入门控、提示符/请求开始标记、状态观测、固定场景服务和确定性测试 |
| `ssh_fixture_test.go` | 本机临时 SSH 服务，复用生产 SSH 客户端连接真实 PTY |
| `integration_test.go` | 当前 Shell 连续性、su、停止与不支持状态测试 |
| `fixture/main_linux.go` | 仅在隔离 WSL 实验环境使用的 Linux PTY 桥 |
| `run-lab.ps1` | 构建实验桥、执行显式启用的集成测试、保存日志 |
| `prepare-lab.py` / `lab-dependencies.json` | 可选准备带固定哈希的 Alpine Bash/readline 文件 |

## 复现

仅运行单元测试（集成测试默认跳过）：

```powershell
go test ./experiments/terminalhandoff -count=1
go test ./pkg/terminalcontrol -count=1
go vet ./pkg/terminalcontrol ./experiments/terminalhandoff
```

完整实验需要 Windows x64、仓库所需 Go 版本、WSL 中一个**可丢弃的 Alpine/musl x86_64 环境**，以及其中现有的 root、nobody、BusyBox ash、su、id、od、tr、ncursesw 和 PTY。当前验证使用已有的 `docker-desktop` 发行版，无需 Docker daemon；不是任意 WSL 发行版通用脚本。测试以 root 启动，真实切换到 nobody，不创建系统账号。

```powershell
# 可选下载官方 Alpine 包；只解压到忽略目录，不调用包管理器。
# 需要 Python 3.10+；下载包按固定 SHA256 校验。
powershell -NoProfile -File experiments/terminalhandoff/run-lab.ps1 -PrepareBash -Count 3
```

已有依赖时去掉 `-PrepareBash` 即可。没有实验 Bash 时只测 `/bin/sh`；脚本输出的子测试名称必须与预期 Shell 范围一致，不能把仅测 ash 记为双 Shell 通过。`-Distro` 可指定满足相同条件的隔离发行版。

脚本保存并恢复进程环境变量；生成物位于 `e2e/.runtime/terminal-handoff/`，包括 `pty-lab`、可选依赖及 `validation-repeat.log`。依赖来自 [Alpine 官方仓库](https://dl-cdn.alpinelinux.org/alpine/v3.22/main/x86_64/)，版本、URL 与哈希记录在清单中。没有把 Bash、实验桥或这些依赖加入发行包。

集成测试只监听 `127.0.0.1` 随机端口，使用临时主机密钥与固定实验账号，拒绝 SSH exec 请求。不会读取已保存连接或访问业务服务器。测试结束关闭自己的连接和子进程，不停机、不重启 Docker、不修改 Shell 配置文件。

## 使用约束

`Bootstrap(ctx, true)` 中的确认由实验操作者保证：当前是空闲普通 Shell，且没有未提交输入。不能把参数固定为 true 后用于未知终端。生产接入需要真实确认流程与 Shell 适配器。

命令以单行原文进入既有 Shell。PS1 临时前缀在提示符展开时上报状态，不为用户命令另开 SSH exec，也不把用户命令包在新 Shell 中。移除标记需在确认空闲后调用 `Uninstall`；环境重置或不兼容时不能盲目发送清理命令。

`Revoke` 使旧操作权失效，不能撤回已提交字节，也不宣称远端进程终止。Ctrl-C 是独立输入，只有收到新提示符后才确认前台命令返回。I/O 阻塞时旧写入仍可能继续等待；正式接口需提供这种中间状态。

桌面的 Service 只协调一轮固定接手测试，尚未建立 Chrys 的全进程 AI 调度、审批、命令风险判定或正式审计。随机标识用于关联控制帧，不能作为对抗恶意服务器的安全边界。请求开始标记排除提交前迟到的提示符；后台输出、复杂提示符及完整行编辑状态均不在完成保证内，不能直接发布为通用命令执行工具。

`desktop_fixture_test.go` 可在显式设置 `OPSCOPILOT_HANDOFF_DESKTOP` 后，为真实桌面联调保持这个本机端点。它在忽略目录生成仅含实验凭据的端点文件和实验输入记录；默认测试会跳过。桌面验证脚本为 `e2e/harness/terminal-handoff.mjs`。

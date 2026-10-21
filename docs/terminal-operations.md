# 终端应用接口与交接契约

状态：2026-10-20，阶段 2 第二批已实现并完成本机验证。进程内调用者可以读取现场、接手同一个 SSH PTY、连续执行命令并交还终端。桌面固定测试已复用这套执行路径。第三批已增加进程内的[授权、审批与执行审计](terminal-authorization.md)门面；Chrys、用户审批呈现和任意命令外部入口尚未接入。

## 调用边界

应用装配通过 `app_terminal_control.go` 的非导出方法 `newTerminalAccess()` 构造授权门面，调用适配器使用其 `Operations()`。底层 `Service` 保留为可信实现，不直接提供给 AI 或外部动作。领域实现不依赖 Wails、MCP 或 ACP；后续适配器只负责协议转换、调用身份及授权，不复制终端执行逻辑。

| 接口 | 行为 |
|---|---|
| `Operations.Inspect(Target, ReadOptions)` | 只读获取已观察的提示符现场和有限输出，不发送探测命令 |
| `Operations.Take(ctx, TakeRequest)` | 绑定实际连接，确认普通命令行后初始化并取得不透明运行句柄 |
| `ControlledTerminal.Inspect(ReadOptions)` | 在本轮仍有效时读取现场 |
| `ControlledTerminal.Execute(ctx, CommandRequest)` | 在同一运行、同一 Shell 中执行一条命令并返回提交与完成证据 |
| `ControlledTerminal.Release()` | 撤销本轮输入权并交还；不宣称远端命令已终止 |

`Target` 同时包含页签会话 ID 和实际连接 ID。重连后旧目标失效，必须基于新连接重新确认。接手返回的句柄不能由调用者用字符串 ID 重新构造；每条命令另有操作 ID，用于后续审批和审计关联。

一次运行可以顺序执行多条命令，命令完成后继续占用，直至显式交还、取消、超时、断连或失败。相同运行的并发执行直接拒绝，不隐式排队；当前桌面进程最多一个受控运行。准备阶段已经阻止竞争人工输入，其他终端仍可手动使用。

## 结果与停止语义

`CommandResult` 将提交与完成分开表示：

| 字段 | 值 | 含义 |
|---|---|---|
| `Submission` | `not_submitted` | 未调用底层写入，例如旧句柄、无效参数或提交前取消 |
| `Submission` | `unknown` | 已尝试写入但未确认完整发送，可能部分到达，不自动重试 |
| `Submission` | `submitted` | 底层写入完整返回，不等于远端执行成功 |
| `Completion` | `not_started` | 未提交 |
| `Completion` | `unknown` | 已尝试提交，但未获得本条命令的完成证据 |
| `Completion` | `completed` | 已观察到命令开始标记及随后真实提示符，可以报告退出码 |

非零退出码属于命令已完成，不是传输失败。取消、写入错误或断连不能推造退出码；`ExitCode` 仅在完成已确认时存在。`Context` 在尝试提交后返回；未提交时不附带可能属于其他操作的现场。

停止先撤销操作权。旧句柄不能再执行、读取运行现场或影响后续运行；已经交给传输层的字节无法撤回。正在阻塞的底层写入可能延迟 `Execute` 返回，停止本身不等待该写入完成。人工输入仍可能等待底层传输恢复，因此不能把本机释放等同于网络已恢复或远端进程已结束。

本次结果在完成或撤销时冻结。即使底层写入迟到返回，也不会收集交还后用户产生的新输出和提示符现场。远端前台命令如仍在运行，由用户接管处理；真实提示符回来前不报告命令已经结束。

## 现场与输出

`TerminalContext` 返回准备状态、应用占用状态、关闭状态、最后提示符和输出窗口。最后提示符包含 Shell PID、UID、目录、退出码和递增序号。`Ready=false` 时这些是历史现场，不能据此断言当前前台程序或当前身份。未知终端只返回未知状态，不凭文本形状自动探测；初始化仍要求明确确认空闲普通命令行。

每个连接按需分配 64 KiB 环形输出缓冲，默认单次返回最近 16 KiB，可配置到 64 KiB。输出游标限定于实际连接，按原始可见字节累计；`From`、`To` 和 `Truncated` 明确标识读取范围及截断。剪断或非法 UTF-8 在文本中替换，不改变原始字节游标。

这里返回的是去掉内部协议帧后的 PTY 输出，可能含回显、提示符、终端控制序列和后台程序输出，不能当作纯 stdout，也不能保证严格归属于单条命令。结果窗口从本条提交前开始，按上限返回尾部；超过缓冲的内容不补造。缓冲仅在内存中存在，不新增永久录制或原始输出日志。

当前命令输入限制为不超过 2048 字节的 UTF-8 单行文本，允许制表符，拒绝换行和原始终端控制字符。这是输入契约，不是命令安全判断；单行文本仍可执行修改或敏感操作。

运行默认 30 秒，最高两分钟；单条命令可设更短期限，不能延长运行期限。取消执行会结束整轮占用；正常命令完成保留占用。宿主存活检测、等待提问与长期运行续租属于后续运行协调。

## 验证与复现

验证只使用内存端点和本机临时 SSH/PTY，未连接业务服务器。

- 相关 Go 包及桌面终端回归各五轮通过，静态检查通过。覆盖多命令复用、未知现场只读、连接替换、占用预留、并发拒绝、提交前取消、部分写入、阻塞写入时停止、结果冻结、返回快照隔离和空闲期限到期。
- ash、Bash 真实 SSH PTY 各三轮通过。应用接口连续执行三条命令，保留原 Shell PID、目录和环境，区分非零退出码；交还后人工继续操作。取消前台命令后报告未知结果，人工 Ctrl+C 后观察到真实提示符；仅一个 SSH 会话，无另开 exec。
- 前端 33 项相关测试和 TypeScript 检查通过；页面八项交接检查通过，页面脚本错误为零。覆盖同 Shell 延续、竞争输入拒绝、其他页签可用、手动接管、中止按钮、重连重新确认、刷新及关闭释放。

本机证据：`e2e/.runtime/terminal-operations-pty.log`、`e2e/.runtime/terminal-operations-e2e.log` 和 `e2e/.runtime/handoff-desktop-1792479040396/result.json`，均位于忽略目录，不随源码分发。页面测试后端仍有 Wails 开发模式的 `runtime:ready` 及 WebSocket 退出日志，不将页面脚本错误为零扩大为后端无日志。未运行 Windows Go 竞态检测，未验证全部 Shell 或目标内网。

```powershell
go test ./pkg/terminalcontrol ./pkg/session ./pkg/terminal -count=5
go test . -run '^TestTerminal(Lifecycle|Handoff|OutputGate)' -count=5
go vet ./pkg/terminalcontrol ./pkg/session
powershell -NoProfile -File experiments/terminalhandoff/run-lab.ps1 -Count 3
node e2e/harness/terminal-handoff.mjs --reuse-frontend --port=34145
```

页面复现需已有当前前端构建；不使用 `--reuse-frontend` 时由启动器构建。不要同时运行前端构建与根目录 Go 构建，Go 嵌入的前端目录会被构建清理影响。

## 下一批边界

底层接口仅供可信进程内代码使用。第三批已在其前实现进程内授权门面，分别保存实际执行审计与生命周期日志，详见[授权实现](terminal-authorization.md)。现有 Wails 入口仍仅接受两个固定测试场景，没有新增任意命令的 Wails、MCP 或 CLI 入口；固定测试不等同于已经接入完整授权用户流程。

接下来将可信的用户授权/审批呈现与 Chrys 生命周期接入受控入口，验证审批与实际调用关联。普通命令行之外的交互程序、多个 AI 页签的统一调度和提问生命周期按[架构路线](application-kernel-architecture.md)继续实施，不以本批通过代表整个阶段 2 完成。

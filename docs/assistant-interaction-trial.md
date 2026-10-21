# 助手交互与操作确认试用

状态：2026-10-20，第四批已接入可见的操作确认体验，共用交互协调、终端授权与实际执行审计。第五批已进一步接通输入框、Chrys 和受控 MCP 终端工具，参见[终端助手试用](assistant-terminal-trial.md)。下文保留固定确认体验的验收范围。

## 如何体验

连接一个普通 SSH 终端，在终端上方依次选择“接手测试 → 体验操作确认”，确认当前命令行空闲后开始。

1. 在原 Shell 自动执行 `command pwd`。
2. 显示待确认卡片，列出实际命令、执行身份和当前目录。体验命令只打印文字，不修改服务器文件。
3. 选择“允许本次”后执行一次；选择“拒绝”则不发送这条命令。
4. 等待期间终端可以手动使用。如果人工操作产生新提示符，再批准时会拒绝旧现场，提示重新开始。

“停止本轮”、终端栏停止、关闭页签和正常页面离开均撤销待批请求。迟到回复不能恢复旧运行。强制结束浏览器仍不能保证卸载通知送达，体验由后端两分钟期限及待批项一分钟期限兜底；不把本机交还等同于远端进程终止。

现有两个固定接手测试继续可用。新体验只接受场景、实际连接与空闲确认，不接受浏览器提供的任意命令、调用身份、UID 或授权范围。用户动作只能回答后端已登记的对应请求；没有请求时，提交 `allow` 不会得到执行权。

## 模块边界与生命周期

```text
页面操作确认 / 问题卡片
  → Wails 用户回答入口
  → assistantcontrol：单轮、目标、待答请求、停止与失效
  → 可信流程调用 terminalcontrol.Access
  → 实际写入前核验与审计
  → 原 SSH PTY
```

`pkg/assistantcontrol` 不依赖 Wails、ACP、终端或模型。新受控工作流同时只保留一轮运行，绑定开始时的页签和实际连接。状态为运行、等待、完成、失败或停止；运行 ID 和每条问题 ID 均由宿主生成。回答只回应对应等待，不进入普通消息队列。

问题发布前先调用资源交还回调；回调期间不公开可作答的问题，也不允许并发问题再次交还资源。停止、新运行替换、超时、重复提交或旧请求 ID 均不能触发旧回调。快照复制选项与详情，界面不能通过修改返回对象改写后端决策。

`app_assistant_control.go` 装配固定体验、用户回答和停止。停止先取消整轮上下文，再处理已捕获的实际终端运行 ID；不在停止后查找并误停新运行。关闭页签及原连接 EOF 同样撤销所属交互，旧连接回调不能取消新连接上的工作。

前端 `AssistantInteractionCard` 支持一次性批准/拒绝、单选、多选及自由回答。确认卡片独立于终端输入遮罩：执行时显示占用，等待时交还；等待中的页签标题仍标记“等待确认”。页面清理会撤销等待中的整轮，即使当时已没有终端占用。

授权、一次性批准和持久化审计复用[第三批实现](terminal-authorization.md)。便携部署下，审计路径在应用装配时解析为绝对路径，不依赖配置目录原本是否为绝对路径。

## ACP 适配范围

`pkg/agenttransport/acp` 负责 NDJSON JSON-RPC、请求关联、反向请求、通知、进程关闭和输入上限。每帧上限 1 MiB，最多 128 个待响应调用、8 个并发反向请求和 64 条待处理通知。格式错误或流量超过边界时关闭连接，不静默丢弃后续授权事件。

反向交互不会阻塞主读循环，等待用户时仍可接收流式更新。进程退出或取消会结束等待并撤销卡片；通知带宿主运行 ID，后续消费者仍须按运行 ID 拒绝迟到更新。Windows 子进程隐藏启动，不使用 Shell 拼接启动命令。第三方 stderr 当前只排空，不原样转发给日志或界面。

交互适配器只处理绑定会话的 `chrys/request_input`、`session/request_permission` 和 `session/update`。未知文件/终端请求默认拒绝，不通过 ACP 客户端替模型开放本机执行通道。

- 新版结构化 `questions[]` 依次显示问题，保留单选、多选及补充文本，按原顺序返回 `answers[]`；兼容旧版单问题与文本回答格式。
- 权限只映射厂商提供的 `allow_once` / `reject_once`，不提供永久授权。ACP 权限选择不构成 OpsCopilot 的终端批准凭据，尚未实现两端一次确认的可靠关联。
- 当前每轮绑定的客户端采用关闭传输来兜底取消，不宣称已经实现跨轮 ACP 会话复用、优雅 `session/cancel` 和会话恢复。

## 验证

相关后端包与桌面终端/交互回归各五轮、静态检查通过；前端 38 项相关测试、TypeScript 和构建通过。覆盖重复回答、停止后的旧答案、等待期间资源交还、交还阻塞时取消、连接替换、页签关闭、协议格式错误、阻塞写入取消，以及通知与反向请求并行。

真实页面十四项通过：原八项终端交接检查，加上允许本次、拒绝、人工改变现场后的失效、停止后迟到回答拒绝、刷新撤销及关闭页签撤销。页面脚本错误为零；仍有 Wails 开发模式 `runtime:ready` 与 WebSocket 退出日志。页面证据为忽略目录中的 `e2e/.runtime/handoff-desktop-1792482329540/result.json` 和 `approval-waiting.png`，日志为 `e2e/.runtime/assistant-interaction-e2e.log`。

ash、Bash 的真实本机 SSH PTY 各三轮继续通过，日志为 `e2e/.runtime/assistant-interaction-pty.log`。未连接业务环境，未运行 Windows Go 竞态检测。

真实 Chrys 0.25.1 初始化握手通过，返回协议版本 1，显示名 `iCode`。握手测试使用临时工作目录、临时用户配置目录，不发送 `session/new` 或模型提示。提问与权限交互通过进程内协议端点验证；这不等于真实模型已经完整调用 OpsCopilot。

```powershell
go test ./pkg/assistantcontrol ./pkg/agenttransport/acp ./pkg/terminalcontrol -count=5
go test . -run '^Test(Terminal(Lifecycle|Handoff|OutputGate|Access)|AssistantTargetClosure)' -count=5
go vet ./pkg/assistantcontrol ./pkg/agenttransport/acp ./pkg/terminalcontrol ./pkg/session
powershell -NoProfile -File experiments/terminalhandoff/run-lab.ps1 -Count 3
node e2e/harness/terminal-handoff.mjs --reuse-frontend --port=34165 --keep
```

页面测试需要当前前端构建；不带 `--reuse-frontend` 时由启动器构建。真实 Chrys 握手测试需显式指定本机程序路径到 `OPSCOPILOT_CHRYS_TEST_EXE`，再单独运行 `TestLiveChrysInitialize`；普通测试不会自动启动它。

## 下一条完整链路

第五批已将助手输入框、开始时的实际页签连接、Chrys 会话和本机 MCP 终端工具连起来。终端作用域由宿主签发，命令确认仅在实际执行边界发生，提问后重新核对现场；主助手知识问答也共用运行互斥。消息排队、立即发送、持久会话恢复和其他旧排障入口仍待后续实现，详见[终端助手试用](assistant-terminal-trial.md)。

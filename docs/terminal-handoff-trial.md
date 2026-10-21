# 终端接手测试

状态：2026-10-20，桌面终端接入固定场景测试，已完成本机真实页面验证。尚未接入 Chrys，也未开放任意命令执行接口；这是统一操作控制的第一条纵向链路，不代表整个阶段 2 完成。

## 使用方式

连接 SSH 终端后，点击终端上方的“接手测试”。确认当前处于空闲普通命令行、没有未提交输入，再选择：

- **查看当前现场**：在当前 Shell 查看进程、账号和目录，完成后自动交还。
- **体验停止与接回**：执行固定的 12 秒等待命令，观察光晕遮罩、页签标记和停止行为。

接手期间该页签只能观看，其他页签可继续操作。“停止”和“手动接管”均撤销本轮发令权、移除占用并交还输入。已经发出的远端命令可能仍在运行；页面会保留提示，需要时在终端按 Ctrl+C，收到真实提示符后才确认前台返回。

测试沿用当前 SSH PTY 和 Shell，不重新登录或另开 exec 会话。状态标记只设置在当前 Shell 的 PS1 中，不修改启动文件；为后续交接保留到该 Shell 结束。初始化命令和测试命令的回显目前仍会显示在终端。仅在确认普通命令行后启用，交互程序、复杂提示符及环境重置仍遵循[技术探索报告](terminal-handoff-feasibility.md)的边界。

## 实现边界

| 位置 | 责任 |
|---|---|
| `pkg/terminalcontrol/controller.go`、`protocol.go`、`replies.go` | 单终端输入门控、操作权代次、状态标记、请求开始关联与终端状态回复识别 |
| `pkg/terminalcontrol/service.go` | 一个桌面进程只运行一轮固定测试；协调准备、执行、停止、超时及重连 |
| `app_terminal_control.go` | Wails 的状态查询、开始和停止适配；记录不含输出或凭据的测试生命周期日志 |
| `app.go`、`pkg/session/manager.go` | SSH 会话注册受控输入流，处理输出；键盘、广播、快捷命令和脚本回放共用输入门控 |
| `frontend/src/components/TerminalHandoff/` | 测试选择、占用遮罩、停止/接管、状态恢复及异常提示 |
| 共享 FlexLayout 组件 | 可选页签状态标记，不承担后端权限判定 |

控制器及进程内[终端应用接口](terminal-operations.md)位于独立包，实验环境引用同一个实现，应用不依赖 `experiments/`。固定场景与多命令调用共享运行协调和执行路径。开始接口只接受两个场景标识，不接收用户命令。固定场景的确认不替代未来 AI 的授权审查；当前测试日志也不等同于完整的实际操作审计。

控制帧的随机标识与请求开始标记用于识别状态和排除提交前迟到的提示符，不作为对抗恶意服务器的安全边界。后台输出仍不能严格归属于某一条命令。传输背压下已经提交的字节无法撤回，撤销操作权不会伪报远端已终止。

终端自动发送的完整光标/设备状态回复作为协议流量处理，不算人工未提交输入，也不会广播到其他节点。混合输入、方向键和未识别序列不享受这一豁免。前端遮罩和 inert 提供交互提示，后端门控才是输入控制依据。

页面卸载会尝试停止已知测试，后端测试有 30 秒运行期限，并在上下文取消时独立撤销操作权。浏览器进程被强制结束时，不能保证卸载通知送达；完整 AI 生命周期、宿主存活检测与审批关联仍属于后续工作。

## 验证

已完成以下针对性验证：

- 控制器与服务测试：状态/完成判断、迟到提示符、终端协议回复、占用、跨终端误用、全局测试互斥、停止、超时、阻塞写入和重连保护。
- 桌面适配测试：SendCommand 与广播都受同一输入门控限制，其他终端仍接收输入；只有实际接受的广播输入进入录制。
- 前端 30 项测试：接手确认及重置、遮罩/inert、交还、通信失败保留已知占用、旧轮次与连接响应隔离、重连去重、页面离开与宿主切换清理、协议回复识别，以及既有终端和页签拖拽回归。审查发现与修复见[代码审查](terminal-handoff-quality-review.md)，连接与交接实现见[生命周期说明](terminal-lifecycle-stabilization.md)。
- ash/Bash 的真实 PTY 测试；同时加入 40 次 SSH 窗口变化请求，验证实验服务持续消费请求，不阻塞终端通道。
- 完整页面：同 Shell 的 PID/目录/环境保留，竞争输入被拒绝，另一页签可操作，手动接管与停止按钮均可释放，Ctrl+C 后人工继续操作，页面脚本错误为 0。
- 前端 TypeScript 与构建、相关 Go 包静态检查通过。竞态检测仍受本机 CGO/编译器条件限制，未声称通过。

完整页面证据保存在本机忽略目录 `e2e/.runtime/handoff-desktop-1792463941584/` 的 `result.json`、`handoff-active.png` 和 `handoff-released.png`。这是本机测试记录，不随源码分发。

复现方式：

```powershell
go test ./pkg/terminalcontrol ./pkg/session ./pkg/terminal
go test . -run '^TestTerminal(Lifecycle|Handoff|OutputGate)' -count=1
go vet ./pkg/terminalcontrol ./pkg/session
cd frontend
npm exec vitest -- run src/components/TerminalHandoff src/components/FlexLayout/FlexLayoutAdapter.tabdrag.business.test.tsx src/components/Terminal/Terminal.test.tsx
```

真实 PTY 环境准备见[实验说明](../experiments/terminalhandoff/README.md)。准备好后，在仓库根目录运行 `node e2e/harness/terminal-handoff.mjs`，脚本构建并启动独立桌面实例、运行页面验证后清理。默认页面端口为 34125；附加 `--port=34135` 可使用其他端口，已有页面占用端口时会拒绝启动。附加 `--keep` 会保留临时试用实例，取消端点和测试进程的固定时限，直到所属启动进程退出或显式停止。启动器监控依赖，依赖意外退出时一起关闭预览，避免留下失效入口。脚本仅使用本机临时 SSH 端点与隔离配置，不连接业务环境。也可创建其端点 JSON 路径加 `.stop` 后缀的文件使其正常退出。普通非保留模式仍保留 20 分钟端点上限。

下一步是基于这条链路补齐真实 AI 运作与授权审计，再让 Chrys 使用它。数据库和扩展动作继续遵循[统一架构路线](application-kernel-architecture.md)，不在本次测试入口中提前扩展。

操作确认体验与新版交互状态见[助手交互试用](assistant-interaction-trial.md)。它复用正式授权和执行审计，不接受任意命令。

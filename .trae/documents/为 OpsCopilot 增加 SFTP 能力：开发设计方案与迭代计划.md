# 目标
在现有 SSH 会话与跳板机能力基础上，完成 Iteration 0~2：
- **Iteration 0**：技术预研 + 接口定型 + 最小可测基础设施（TDD 框架就位）
- **Iteration 1**：SFTP MVP（list/stat/upload/download + 进度 + 取消）
- **Iteration 2**：对端不支持 SFTP 的识别 + UI 提示 + SCP 降级（带能力探测）

# 设计落地（面向当前仓库的具体实现）

## 1) 后端：新增文件传输层（pkg/filetransfer）

### 1.1 核心类型与错误码
新增 `pkg/filetransfer`（或 `pkg/transfer`）包，定义：
- `ErrorCode`：`SFTP_NOT_SUPPORTED / PERMISSION_DENIED / NOT_FOUND / AUTH_FAILED / NETWORK / UNKNOWN`
- `TransferError{Code, Message, Cause}`：统一用于前端展示
- `Entry{Path, Name, IsDir, Size, ModTime, Mode}`：目录项
- `TaskID` + `Progress{BytesDone, BytesTotal, SpeedBps}`

说明：当前仓库导出方法多返回 `string`（空串成功，非空为错误/提示），这里将“结构化结果”用 JSON string 返回，保持现有风格。

### 1.2 Transport 抽象
定义接口（概念级）：
- `Check(ctx) (supported bool, reason string, err error)`
- `List/Stat/Upload/Download/Mkdir/Rm/Rename`

Iteration 0~2 实现：
- `SFTPTransport`（主路径）
- `SCPTransport`（降级路径）

并提供 `AutoTransport`：
- 优先 SFTP；若 `SFTP_NOT_SUPPORTED` 则尝试 SCP（需通过能力探测）

## 2) 复用现有 SSH/跳板机连接

### 2.1 复用 session 里的 sshclient.Client
当前 `pkg/session.Manager` 保存 `sshclient.Client` 与 `ssh.Session`。
- 文件传输 API 以 `sessionID` 为入口：从 `sessionMgr.Get(sessionID)` 拿到 `Client`，在同一个 `ssh.Client` 上打开 SFTP/SCP 通道。
- 跳板机/多跳由现有 `sshclient.NewClient()` 负责，文件传输无需新增特殊处理。

### 2.2 并发与资源策略
Iteration 1 采用“短连接 SFTP client”：
- 每次 list/stat/upload/download 创建 `sftp.Client` 并 `Close()`，避免缓存失效和并发复杂度。

## 3) Wails 导出 API（app.go）

新增导出方法（Iteration 1 完成）：
- `FTList(sessionID, remotePath) string(JSON)`
- `FTStat(sessionID, remotePath) string(JSON)`
- `FTUpload(sessionID, localPath, remotePath) string(JSON)`
- `FTDownload(sessionID, remotePath, localPath) string(JSON)`
- `FTCancel(taskID) string`
- （Iteration 2）`FTCheck(sessionID) string(JSON)`：返回 `{preferred:"sftp"|"scp"|"none", reason}`

事件：
- `file-transfer-progress`：`{taskID, sessionID, bytesDone, bytesTotal, speedBps}`
- `file-transfer-done`：`{taskID, ok, message, code?}`

说明：事件命名与 `app.go` 现有终端事件一致风格（`terminal-data:sessionID`），但这里用统一事件名+payload，便于前端做任务队列。

## 4) SFTP 具体实现（Iteration 1）

### 4.1 依赖
- 增加依赖 `github.com/pkg/sftp`（客户端；测试中也用其 server 端能力）。

### 4.2 能力探测与“不支持”识别
- `sftp.NewClient(sshClient)` 失败时：
  - 解析常见错误（subsystem request failed / unknown channel / EOF at subsystem）映射为 `SFTP_NOT_SUPPORTED`
  - 其它错误按 `AUTH_FAILED/NETWORK/UNKNOWN` 分类

### 4.3 上传/下载实现要点
- `Upload`：`os.Open(local)` + `sftpClient.Create(remote)` + `io.Copy`，包装 reader 统计 bytes 并定期 emit 进度事件
- `Download`：`sftpClient.Open(remote)` + `os.Create(local)` + `io.Copy`
- 支持取消：
  - 每个 task 存一个 `context.CancelFunc`；copy loop 每 N KB 检查 ctx

## 5) SCP 降级实现（Iteration 2）

### 5.1 何时触发
- 仅当 SFTP 明确判定为 `SFTP_NOT_SUPPORTED` 才进入 SCP。

### 5.2 SCP 可用性探测
- 复用仓库的“对端能力探测模式”：在目标机执行 `command -v scp && echo 1 || echo 0`。
- 若无 scp：前端提示“对端未开启 SFTP，且无 scp，可联系运维启用 Subsystem sftp 或安装 openssh-clients”。

### 5.3 SCP 协议实现
- 采用 `scp -t`（上传）与 `scp -f`（下载）协议，走 `ssh.Session` 的 stdin/stdout 进行 ACK 交互。
- 仅支持单文件（Iteration 2 范围）；目录递归放到后续迭代。

## 6) 前端 UI（Iteration 1~2）

### 6.1 MVP 入口
在现有会话视图中新增“文件”入口（实现方式二选一，按仓库 UI 结构选择最小侵入）：
- A：在终端组件附近增加一个 Tab（推荐）
- B：在 Settings 或侧边栏新增“文件传输”面板（只对已连接 session 可用）

### 6.2 MVP 功能
- 远端路径输入框 + 列表
- 选择本地文件上传、选择保存路径下载
- 进度展示（基于事件）+ 取消按钮
- Iteration 2：若返回 `SFTP_NOT_SUPPORTED`，UI 显示明确原因，并展示“尝试 SCP”按钮（或自动降级并在消息中说明“已使用 SCP 传输”）

### 6.3 wailsjs 绑定更新
- 更新 `frontend/wailsjs/go/main/App.(d.ts|js)` 添加新增导出方法声明与包装。

# TDD 与验证（Iteration 0~2 必须完成）

## 1) 后端单测
新增测试包：`pkg/filetransfer/*_test.go`，以“先红后绿”推进：
- `TestSFTP_ListUploadDownload_HappyPath`
  - 启动内嵌 SSH server + SFTP subsystem（测试内实现）
  - 验证 list/stat/upload/download
- `TestSFTP_NotSupported_SubsystemDisabled`
  - SSH server 不注册 sftp subsystem
  - 断言错误码为 `SFTP_NOT_SUPPORTED`
- `TestAutoTransport_FallbackToSCP_WhenSFTPNotSupported`
  - 服务器禁用 sftp，但实现一个最小 scp handler（exec `scp -t/-f`）
  - 验证能够完成上传/下载
- `TestTransfer_Cancel`
  - 传输中取消，断言任务终止并产生 done 事件/错误码

## 2) 集成构建验证
- `go test ./...`
- `npm -C frontend run build`

# 迭代拆分（按你要求仅做到 Iteration 0~2）

## Iteration 0（接口与可测基座）
- 建立 `pkg/filetransfer` 的类型、错误码、事件协议
- 内嵌 SSH+SFTP 测试基座（先写失败测试）
- 验收：最小 list 测试先失败（红灯）

## Iteration 1（SFTP MVP）
- 完成 SFTPTransport：list/stat/upload/download + progress + cancel
- App 导出 API + 事件推送
- 前端最小 UI 接入
- 验收：直连主机场景可用；大文件进度正常；可取消

## Iteration 2（不支持处理 + SCP 降级）
- SFTP 不支持判定与错误码/提示语
- scp 探测 + SCPTransport（单文件）
- 前端显示降级原因与结果（自动或按钮触发）
- 验收：禁用 sftp 的测试 server 仍可通过 scp 完成传输；否则提示明确可操作建议

# 输出物（你将拿到什么）
- 后端：`pkg/filetransfer` + App 导出方法 + 事件协议
- 前端：文件面板（MVP）+ 进度/取消/降级提示
- 测试：覆盖直连、SFTP 不支持、SCP 降级、取消

如果你确认该计划，我会开始按 Iteration 0→1→2 顺序落地，并确保每个迭代都通过测试与构建验证后再进入下一个。
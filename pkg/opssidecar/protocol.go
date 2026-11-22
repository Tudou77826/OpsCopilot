// Package opssidecar 实现 ops-sidecar 的服务层：NDJSON over stdio 的常驻运维服务，
// 供 AAW Desktop 的 remoteOps 插件驱动。协议约定：客户端按行发送
// {"id","method","params"}，服务端按行回 {"id","ok","data","error"}；
// 终端字节流、传输进度等异步数据以 {"event","params"} 主动下发（无 id 配对）。
//
// 方法面与事件名对齐 OpsCopilot Wails App 面（frontend-shell 组件消费的那套），
// 经 Desktop 适配层调用时语义一一对应：
//   - 树 CRUD ↔ App.GetConnectionTree/CreateSaved* 等（SessionManagerRuntime 端口）
//   - 终端 ↔ App.Connect/Write/ResizeTerminal/TerminalOutputReady + terminal-data:{id} 事件
//   - 传输 ↔ App.FT*/Local*（ftResponse JSON 信封字符串）+ file-transfer-progress/done 事件
package opssidecar

import "encoding/json"

// Request 客户端请求行。
type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response 服务端响应行。OK=false 时 Error 必填。
type Response struct {
	ID    string     `json:"id"`
	OK    bool       `json:"ok"`
	Data  any        `json:"data,omitempty"`
	Error *ErrorWire `json:"error,omitempty"`
}

// Event 服务端主动事件行：无 id 配对，客户端按 Event 字段分发。
// terminal-data:{sessionId} 为动态事件名，params 直接是输出明文（JSON 转义）。
type Event struct {
	Event  string `json:"event"`
	Params any    `json:"params,omitempty"`
}

// ErrorWire 结构化错误：Code 供前端分类渲染，Message 原文透传，Hint 可选指引。
type ErrorWire struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// 方法名常量。与 Desktop 侧 remoteOps 插件 engine/protocol.ts 的字面量一一对应，
// 任何一侧改动必须同步另一侧。
const (
	// 连接树（SessionManagerRuntime 端口 ↔ Wails App 树方法）
	MethodList             = "connections/list"
	MethodCreateFolder     = "connections/create-folder"
	MethodCreateConnection = "connections/create-connection"
	MethodUpdateConnection = "connections/update-connection"
	MethodRenameNode       = "connections/rename-node"
	MethodMoveNode         = "connections/move-node"
	MethodReorderChildren  = "connections/reorder-children"
	MethodDeleteNode       = "connections/delete-node"

	// 执行（exec 面板 / Agent remote_exec 共用）
	MethodExec = "exec"

	// 文件传输（FilesPanel 的 FileTransferHost 端口；返回 ftResponse 信封字符串）
	MethodFTCheck        = "ft/check"
	MethodFTList         = "ft/list"
	MethodFTStat         = "ft/stat"
	MethodFTUpload       = "ft/upload"
	MethodFTDownload     = "ft/download"
	MethodFTUploadDir    = "ft/upload-dir"
	MethodFTDownloadDir  = "ft/download-dir"
	MethodFTCancel       = "ft/cancel"
	MethodFTRemoteMkdir  = "ft/remote-mkdir"
	MethodFTRemoteRemove = "ft/remote-remove"
	MethodFTRemoteRename = "ft/remote-rename"
	MethodFTReadFile     = "ft/remote-read-file"
	MethodFTWriteFile    = "ft/remote-write-file"

	// 本地文件系（FilesPanel 本地栏；同 ftResponse 信封）
	MethodLocalList   = "local/list"
	MethodLocalMkdir  = "local/mkdir"
	MethodLocalRemove = "local/remove"
	MethodLocalRename = "local/rename"
	MethodLocalCopy   = "local/copy"
	MethodLocalStat   = "local/stat"

	// 终端（TerminalRuntime 端口 ↔ App.Connect/Write/ResizeTerminal/...）
	MethodTerminalOpen   = "terminal/open"  // 新建（独立拨号）或按 sessionId 重连
	MethodTerminalWrite  = "terminal/write" // 参数 sessionId + data 明文
	MethodTerminalReady  = "terminal/ready" // 前端挂载完成，冲刷首帧闸门
	MethodTerminalResize = "terminal/resize"
	MethodTerminalClose  = "terminal/close"

	// handoff 审批（Agent 写终端的人工裁决；requestId 并发）
	MethodTerminalApproveHandoff = "terminal/approve-handoff"

	// 设置与配置
	MethodSettingsGet = "settings/get"
	MethodSettingsSet = "settings/set"

	// 闸门配置（白名单 / 文件访问；设置面板编辑）
	MethodWhitelistGet   = "whitelist/get"
	MethodWhitelistSet   = "whitelist/set"
	MethodWhitelistForIP = "whitelist/for-ip"
	MethodFileAccessGet  = "file-access/get"
	MethodFileAccessSet  = "file-access/set"

	// 远程项目连接管理与只读文件浏览（AAW 远程项目）
	MethodProjectConnectionsList  = "project/connections-list"
	MethodProjectConnectionCreate = "project/connection-create"
	MethodProjectConnectionUpdate = "project/connection-update"
	MethodProjectConnectionDelete = "project/connection-delete"
	MethodProjectConnectionTest   = "project/connection-test"
	MethodProjectFSList           = "project/fs-list"
	MethodProjectFSStat           = "project/fs-stat"
	MethodProjectFSPreview        = "project/fs-preview"
	MethodProjectFSListFiles      = "project/fs-list-files"

	// MCP 桥（Desktop 经 env 把地址/token 直传给 --mcp 代理）
	MethodBridgeInfo = "bridge/info"

	MethodShutdown = "shutdown"
)

// 事件名常量。与 OpsCopilot Wails 事件一一对齐（frontend-shell 消费的那套）。
const (
	// EventTerminalDataPrefix + sessionId 构成动态事件名；params 为输出明文。
	EventTerminalDataPrefix = "terminal-data:"
	// EventSessionDisconnected：SSH 流结束；载荷 {sessionId, reason, message, timestamp}。
	EventSessionDisconnected = "session-disconnected"
	// EventTerminalHandoff：Agent 请求写终端，等待裁决；载荷 {requestId, hint, sessionId, data}。
	EventTerminalHandoff = "terminal.handoff"
	// EventTransferProgress：载荷 {taskId, sessionId, bytesDone, bytesTotal, speedBps, step?}。
	EventTransferProgress = "file-transfer-progress"
	// EventTransferDone：载荷 {taskId, sessionId, ok, code?, message?, cancelled?}。
	EventTransferDone = "file-transfer-done"
)

// 错误码。前端按码分类渲染（如 whitelist_denied 展示为"被白名单拦截"）。
const (
	CodeInvalidParams  = "invalid_params"
	CodeMethodNotFound = "method_not_found"
	CodeNotFound       = "not_found"
	CodeWhitelistDeny  = "whitelist_denied"
	CodeNoPassword     = "no_password"
	CodeConnectFailed    = "connect_failed"
	CodeHostKeyUntrusted = "host_key_untrusted"
	CodeHostKeyMismatch  = "host_key_mismatch"
	CodePermissionDenied = "permission_denied"
	CodeUnsupported      = "unsupported"
	CodeExecFailed       = "exec_failed"
	CodeTransferFailed = "transfer_failed"
	CodeInternal       = "internal"
)

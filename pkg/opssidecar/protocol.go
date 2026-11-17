// Package opssidecar 实现 ops-sidecar 的服务层：NDJSON over stdio 的常驻运维服务，
// 供 AAW Desktop 的 remoteOps 插件驱动。协议约定：客户端按行发送
// {"id","method","params"}，服务端按行回 {"id","ok","data","error"}。
// 请求串行处理，保证连接树编辑与 exec 之间的顺序语义；stdout 只输出协议行，
// 诊断信息一律走 stderr。
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

// ErrorWire 结构化错误：Code 供前端分类渲染，Message 原文透传，Hint 可选指引。
type ErrorWire struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// 方法名常量。与 Desktop 侧 remoteOps 插件 engine/protocol.ts 的字面量一一对应，
// 任何一侧改动必须同步另一侧。
const (
	MethodList             = "connections/list"
	MethodCreateFolder     = "connections/create-folder"
	MethodCreateConnection = "connections/create-connection"
	MethodUpdateConnection = "connections/update-connection"
	MethodRenameNode       = "connections/rename-node"
	MethodMoveNode         = "connections/move-node"
	MethodDeleteNode       = "connections/delete-node"
	MethodExec             = "exec"
	MethodShutdown         = "shutdown"
)

// 错误码。前端按码分类渲染（如 whitelist_denied 展示为"被白名单拦截"）。
const (
	CodeInvalidParams  = "invalid_params"
	CodeMethodNotFound = "method_not_found"
	CodeNotFound       = "not_found"
	CodeWhitelistDeny  = "whitelist_denied"
	CodeNoPassword     = "no_password"
	CodeConnectFailed  = "connect_failed"
	CodeExecFailed     = "exec_failed"
	CodeInternal       = "internal"
)

// Package bridge 定义 Shell 和 Agent 组件之间的桥接层
// 该包提供事件总线、命令桥接和类型定义
package bridge

import (
	"context"
	"time"
)

// EventType 事件类型常量
const (
	// Shell 事件
	EventShellConnect      = "shell:connect"
	EventShellDisconnect   = "shell:disconnect"
	EventShellData         = "shell:data"
	EventShellResize       = "shell:resize"
	EventShellError        = "shell:error"
	EventShellInput        = "shell:input"
	EventShellOutput       = "shell:output"

	// Agent 事件
	EventAgentCommand      = "agent:command"
	EventAgentStatus       = "agent:status"
	EventAgentSuggestion   = "agent:suggestion"
	EventAgentError        = "agent:error"

	// 录制事件
	EventRecordingStart    = "recording:start"
	EventRecordingStop     = "recording:stop"
	EventRecordingInput    = "recording:input"
	EventRecordingOutput   = "recording:output"

	// 系统事件
	EventSystemReady       = "system:ready"
	EventSystemShutdown    = "system:shutdown"
)

// Event 表示一个事件
type Event struct {
	// Type 事件类型
	Type string `json:"type"`

	// Timestamp 事件时间戳
	Timestamp time.Time `json:"timestamp"`

	// Source 事件来源（shell/agent/system）
	Source string `json:"source"`

	// SessionID 关联的会话 ID
	SessionID string `json:"session_id,omitempty"`

	// Payload 事件负载数据
	Payload interface{} `json:"payload"`
}

// ShellConnectPayload 连接事件负载
type ShellConnectPayload struct {
	Config ConnectConfigPayload `json:"config"`
}

// ConnectConfigPayload 连接配置负载
type ConnectConfigPayload struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Bastion  *BastionConfigPayload `json:"bastion,omitempty"`
}

// BastionConfigPayload 堡垒机配置负载
type BastionConfigPayload struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
}

// ShellDisconnectPayload 断开连接事件负载
type ShellDisconnectPayload struct {
	Reason string `json:"reason"`
}

// ShellDataPayload 终端数据事件负载
type ShellDataPayload struct {
	Data string `json:"data"`
}

// ShellResizePayload 终端大小变更事件负载
type ShellResizePayload struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// ShellInputPayload 输入事件负载
type ShellInputPayload struct {
	Command string `json:"command"`
}

// ShellOutputPayload 输出事件负载
type ShellOutputPayload struct {
	Output string `json:"output"`
}

// AgentCommandPayload Agent 命令事件负载
type AgentCommandPayload struct {
	Command string `json:"command"`
	Auto    bool   `json:"auto"` // 是否自动执行
}

// AgentStatusPayload Agent 状态事件负载
type AgentStatusPayload struct {
	Status    string `json:"status"`     // idle, thinking, executing
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// AgentSuggestionPayload Agent 建议事件负载
type AgentSuggestionPayload struct {
	Suggestions []string `json:"suggestions"`
	Context     string   `json:"context"`
}

// RecordingStartPayload 录制开始事件负载
type RecordingStartPayload struct {
	RecordingType string `json:"recording_type"`
	SessionID     string `json:"session_id"`
	Host          string `json:"host"`
	User          string `json:"user"`
}

// RecordingStopPayload 录制停止事件负载
type RecordingStopPayload struct {
	RecordingID   string `json:"recording_id"`
	CommandCount  int    `json:"command_count"`
	Duration      int64  `json:"duration"` // 秒
}

// RecordingInputPayload 录制输入事件负载
type RecordingInputPayload struct {
	Command string `json:"command"`
	Index   int    `json:"index"`
}

// RecordingOutputPayload 录制输出事件负载
type RecordingOutputPayload struct {
	Output string `json:"output"`
	Index  int    `json:"index"`
}

// ErrorPayload 错误事件负载
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Source  string `json:"source"`
}

// Handler 事件处理函数类型
type Handler func(ctx context.Context, event Event) error

// Bus 事件总线接口
type Bus interface {
	// Publish 发布事件
	Publish(ctx context.Context, event Event) error

	// Subscribe 订阅事件类型
	Subscribe(eventType string, handler Handler) (string, error)

	// Unsubscribe 取消订阅
	Unsubscribe(subscriptionID string) error

	// SubscribeAll 订阅所有事件
	SubscribeAll(handler Handler) (string, error)

	// Close 关闭事件总线
	Close() error
}

// CommandBridge 命令桥接接口
// 用于 Agent 向 Shell 发送命令
type CommandBridge interface {
	// SendCommand 发送命令到 Shell
	SendCommand(ctx context.Context, sessionID string, command string) error

	// SendCommands 批量发送命令
	SendCommands(ctx context.Context, sessionID string, commands []string) error

	// RegisterHandler 注册命令处理器
	RegisterHandler(sessionID string, handler CommandHandler) error

	// UnregisterHandler 注销命令处理器
	UnregisterHandler(sessionID string) error
}

// CommandHandler 命令处理函数类型
type CommandHandler func(ctx context.Context, command string) error

// StateBridge 状态桥接接口
// 用于 Shell 和 Agent 之间共享状态
type StateBridge interface {
	// GetSessionInfo 获取会话信息
	GetSessionInfo(sessionID string) (SessionState, error)

	// UpdateSessionInfo 更新会话信息
	UpdateSessionInfo(sessionID string, state SessionState) error

	// GetAllSessions 获取所有会话状态
	GetAllSessions() map[string]SessionState

	// Watch 监听状态变化
	Watch(ctx context.Context) <-chan StateChange
}

// SessionState 会话状态
type SessionState struct {
	ID            string    `json:"id"`
	Host          string    `json:"host"`
	User          string    `json:"user"`
	Connected     bool      `json:"connected"`
	Recording     bool      `json:"recording"`
	RecordingType string    `json:"recording_type,omitempty"`
	LastActivity  time.Time `json:"last_activity"`
	CommandCount  int       `json:"command_count"`
}

// StateChange 状态变更
type StateChange struct {
	SessionID string      `json:"session_id"`
	Field     string      `json:"field"`
	OldValue  interface{} `json:"old_value"`
	NewValue  interface{} `json:"new_value"`
	Timestamp time.Time   `json:"timestamp"`
}

// Bridge 桥接器接口（组合所有桥接功能）
type Bridge interface {
	Bus
	CommandBridge
	StateBridge
}

// BridgeFactory 桥接器工厂函数类型
type BridgeFactory func() Bridge

// Package agent 定义 Agent 组件的核心接口和类型
// 该包抽象了 AI 智能体服务、录制系统和故障排查功能
package agent

import (
	"context"
	"time"
)

// RecordingType 录制类型
type RecordingType string

const (
	RecordingTypeTroubleshoot RecordingType = "troubleshoot" // 故障排查录制
	RecordingTypeScript       RecordingType = "script"       // 命令脚本录制
)

// RecordingSession 录制会话信息
type RecordingSession struct {
	ID        string                 `json:"id"`
	Type      RecordingType          `json:"type"`
	StartTime time.Time              `json:"start_time"`
	EndTime   time.Time              `json:"end_time,omitempty"`
	SessionID string                 `json:"session_id"` // 关联的 Shell 会话 ID
	Host      string                 `json:"host"`
	User      string                 `json:"user"`
	Commands  []RecordedCommand      `json:"commands"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// RecordedCommand 录制的命令
type RecordedCommand struct {
	Index     int    `json:"index"`
	Content   string `json:"content"`
	Output    string `json:"output,omitempty"`
	Timestamp int64  `json:"timestamp"` // 相对开始时间的毫秒数
	Duration  int    `json:"duration,omitempty"`
}

// RecorderStatus 录制器状态
type RecorderStatus struct {
	IsRecording  bool          `json:"is_recording"`
	SessionID    string        `json:"session_id,omitempty"`
	Type         RecordingType `json:"type,omitempty"`
	CommandCount int           `json:"command_count"`
	Duration     int64         `json:"duration"` // 秒
}

// Recorder 录制器接口
type Recorder interface {
	// Start 开始录制
	Start(recType RecordingType, sessionID, host, user string) (*RecordingSession, error)

	// RecordInput 记录输入命令
	RecordInput(sessionID string, command string) error

	// Stop 停止录制
	Stop() (*RecordingSession, error)

	// GetStatus 获取录制状态
	GetStatus() RecorderStatus

	// GetCurrentSession 获取当前录制会话
	GetCurrentSession() *RecordingSession
}

// RecorderStorage 录制存储接口
type RecorderStorage interface {
	// Load 加载录制会话
	Load(recType RecordingType, id string) (*RecordingSession, error)

	// Save 保存录制会话
	Save(session *RecordingSession) error

	// List 列出所有录制会话
	List(recType RecordingType) ([]*RecordingSession, error)

	// Delete 删除录制会话
	Delete(recType RecordingType, id string) error
}

// TroubleshootCase 故障排查案例
type TroubleshootCase struct {
	RecordingSession
	Problem     string   `json:"problem"`
	Context     []string `json:"context"`
	RootCause   string   `json:"root_cause"`
	Conclusion  string   `json:"conclusion"`
	Suggestions []string `json:"suggestions"`
}

// TroubleshootStatus 故障排查状态
type TroubleshootStatus struct {
	IsActive     bool   `json:"is_active"`
	CaseID       string `json:"case_id,omitempty"`
	Problem      string `json:"problem,omitempty"`
	CommandCount int    `json:"command_count"`
	Duration     int64  `json:"duration"`
}

// Troubleshotter 故障排查接口
type Troubleshotter interface {
	// StartCase 开始故障排查
	StartCase(problem string, context []string, sessionID, host, user string) (*TroubleshootCase, error)

	// StopCase 结束故障排查
	StopCase(rootCause, conclusion string) (*TroubleshootCase, error)

	// GetCurrentCase 获取当前案例
	GetCurrentCase() *TroubleshootCase

	// GetStatus 获取排查状态
	GetStatus() TroubleshootStatus

	// LoadCase 加载案例
	LoadCase(caseID string) (*TroubleshootCase, error)

	// ListCases 列出所有案例
	ListCases() ([]*TroubleshootCase, error)

	// DeleteCase 删除案例
	DeleteCase(caseID string) error

	// GenerateDocument 生成排查文档
	GenerateDocument(caseID string) (string, error)
}

// AIResponse AI 响应
type AIResponse struct {
	Success   bool     `json:"success"`
	Message   string   `json:"message"`
	Commands  []string `json:"commands,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// AIProvider AI 提供者接口
type AIProvider interface {
	// Chat 发送消息并获取响应
	Chat(ctx context.Context, sessionID string, message string) (*AIResponse, error)

	// StreamChat 流式发送消息
	StreamChat(ctx context.Context, sessionID string, message string, callback func(chunk string)) error

	// SetContext 设置上下文
	SetContext(sessionID string, context []string)

	// ClearContext 清除上下文
	ClearContext(sessionID string)
}

// AgentService 智能体服务接口
type AgentService interface {
	// ProcessUserInput 处理用户输入
	ProcessUserInput(ctx context.Context, sessionID string, input string) (*AIResponse, error)

	// GetStatus 获取服务状态
	GetStatus() ServiceStatus

	// Start 启动服务
	Start(ctx context.Context) error

	// Stop 停止服务
	Stop() error
}

// ServiceStatus 服务状态
type ServiceStatus struct {
	Running      bool   `json:"running"`
	Model        string `json:"model"`
	SessionCount int    `json:"session_count"`
}

// RecordingEvent 录制事件接口
// Agent 组件通过此接口接收来自 Shell 的录制事件
type RecordingEvent interface {
	// OnInput 收到输入命令
	OnInput(sessionID string, command string) error

	// OnOutput 收到输出数据
	OnOutput(sessionID string, output string) error

	// OnRecordingStart 录制开始
	OnRecordingStart(sessionID string, recType RecordingType) error

	// OnRecordingStop 录制停止
	OnRecordingStop(sessionID string) error
}

// CommandExecutor 命令执行器接口
// Agent 通过此接口请求 Shell 执行命令
type CommandExecutor interface {
	// ExecuteCommand 执行命令
	ExecuteCommand(ctx context.Context, sessionID string, command string) error

	// ExecuteCommands 批量执行命令
	ExecuteCommands(ctx context.Context, sessionID string, commands []string) error
}

// EventSubscriber 事件订阅者接口
// Agent 组件通过此接口订阅事件
type EventSubscriber interface {
	// Subscribe 订阅事件
	Subscribe(eventType string, handler EventHandler) error

	// Unsubscribe 取消订阅
	Unsubscribe(eventType string) error
}

// EventHandler 事件处理函数类型
type EventHandler func(payload interface{}) error

// ServiceFactory Agent 服务工厂函数类型
type ServiceFactory func(config ServiceConfig) AgentService

// ServiceConfig 服务配置
type ServiceConfig struct {
	Model       string
	APIKey      string
	MaxTokens   int
	Temperature float64
}

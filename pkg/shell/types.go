// Package shell 定义 Shell 组件的核心接口和类型
// 该包抽象了终端会话管理、命令发送和终端事件处理
package shell

import (
	"context"
	"io"
)

// ConnectConfig 连接配置
type ConnectConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	Password     string `json:"password,omitempty"`
	KeyPath      string `json:"key_path,omitempty"`
	KeyPassphrase string `json:"key_passphrase,omitempty"`
	Bastion      *BastionConfig `json:"bastion,omitempty"`
}

// BastionConfig 堡垒机配置
type BastionConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	Password     string `json:"password,omitempty"`
	KeyPath      string `json:"key_path,omitempty"`
	KeyPassphrase string `json:"key_passphrase,omitempty"`
}

// SessionInfo 会话信息（只读视图）
type SessionInfo struct {
	ID     string `json:"id"`
	Host   string `json:"host"`
	User   string `json:"user"`
	Active bool   `json:"active"`
}

// Session 表示一个终端会话
type Session interface {
	// ID 返回会话唯一标识符
	ID() string

	// Info 返回会话信息
	Info() SessionInfo

	// Send 向会话发送数据
	Send(data string) error

	// Resize 调整终端大小
	Resize(cols, rows int) error

	// Close 关闭会话
	Close() error

	// Stdin 返回标准输入写入器（用于命令发送）
	Stdin() io.Writer
}

// Manager 会话管理器接口
type Manager interface {
	// Connect 建立新的 SSH 会话
	Connect(ctx context.Context, config ConnectConfig) (Session, error)

	// Get 获取指定 ID 的会话
	Get(id string) (Session, bool)

	// List 列出所有活跃会话
	List() []Session

	// Disconnect 断开指定会话
	Disconnect(id string) error

	// DisconnectAll 断开所有会话
	DisconnectAll() error

	// Broadcast 向多个会话广播数据
	Broadcast(ids []string, data string) error
}

// TerminalEvent 终端事件接口
type TerminalEvent interface {
	// OnConnect 会话连接事件
	OnConnect(sessionID string, config ConnectConfig)

	// OnDisconnect 会话断开事件
	OnDisconnect(sessionID string, reason string)

	// OnData 终端数据事件
	OnData(sessionID string, data string)

	// OnResize 终端大小变更事件
	OnResize(sessionID string, cols, rows int)

	// OnError 错误事件
	OnError(sessionID string, err error)
}

// CommandSender 命令发送接口
// 实现此接口的类型可以向 Shell 发送命令
type CommandSender interface {
	// SendCommand 向指定会话发送命令
	SendCommand(sessionID string, command string) error

	// SendCommandWithReply 向指定会话发送命令并等待回复
	SendCommandWithReply(ctx context.Context, sessionID string, command string) (string, error)
}

// LineBufferProvider 行缓冲区提供者接口
type LineBufferProvider interface {
	// GetLineBuffer 获取指定会话的行缓冲区
	GetLineBuffer(sessionID string) (string, error)

	// ClearLineBuffer 清除指定会话的行缓冲区
	ClearLineBuffer(sessionID string)
}

// EventPublisher 事件发布者接口
// Shell 组件通过此接口发布事件
type EventPublisher interface {
	// Publish 发布事件
	Publish(eventType string, payload interface{}) error
}

// ManagerFactory 会话管理器工厂函数类型
type ManagerFactory func() Manager

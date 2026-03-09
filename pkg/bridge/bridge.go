// Package bridge 定义 Shell 和 Agent 组件之间的桥接层
package bridge

import (
	"context"
	"fmt"
	"sync"
)

// BridgeImpl 组合桥接实现
type BridgeImpl struct {
	*EventBus
	*CommandBridgeImpl
	*StateBridgeImpl

	mu    sync.RWMutex
	closed bool
}

// NewBridge 创建桥接器
func NewBridge() *BridgeImpl {
	return &BridgeImpl{
		EventBus:          NewEventBus(),
		CommandBridgeImpl: NewCommandBridge(),
		StateBridgeImpl:   NewStateBridge(),
	}
}

// Close 关闭桥接器
func (b *BridgeImpl) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// 关闭各个组件
	if err := b.EventBus.Close(); err != nil {
		return fmt.Errorf("failed to close event bus: %w", err)
	}

	return nil
}

// IsClosed 检查是否已关闭
func (b *BridgeImpl) IsClosed() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.closed
}

// SetupShellIntegration 设置 Shell 集成
// 返回命令处理器注册函数
func (b *BridgeImpl) SetupShellIntegration(sessionID string, handler CommandHandler) error {
	return b.RegisterHandler(sessionID, handler)
}

// SetupAgentIntegration 设置 Agent 集成
// 返回事件订阅函数
func (b *BridgeImpl) SetupAgentIntegration(handler Handler) (string, error) {
	return b.SubscribeAll(handler)
}

// PublishShellEvent 发布 Shell 事件
func (b *BridgeImpl) PublishShellEvent(ctx context.Context, eventType string, sessionID string, payload interface{}) error {
	event := NewEvent(eventType, sessionID, payload)
	return b.Publish(ctx, event)
}

// PublishAgentEvent 发布 Agent 事件
func (b *BridgeImpl) PublishAgentEvent(ctx context.Context, eventType string, sessionID string, payload interface{}) error {
	event := NewEvent(eventType, sessionID, payload)
	return b.Publish(ctx, event)
}

// ExecuteAgentCommand 执行 Agent 命令
func (b *BridgeImpl) ExecuteAgentCommand(ctx context.Context, sessionID string, command string) error {
	// 发布命令事件
	event := NewEvent(EventAgentCommand, sessionID, AgentCommandPayload{
		Command: command,
		Auto:    true,
	})
	if err := b.Publish(ctx, event); err != nil {
		return err
	}

	// 执行命令
	return b.SendCommand(ctx, sessionID, command)
}

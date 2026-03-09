// Package bridge 定义 Shell 和 Agent 组件之间的桥接层
package bridge

import (
	"context"
	"fmt"
	"sync"
)

// CommandBridgeImpl 命令桥接实现
type CommandBridgeImpl struct {
	mu       sync.RWMutex
	handlers map[string]CommandHandler
}

// NewCommandBridge 创建命令桥接
func NewCommandBridge() *CommandBridgeImpl {
	return &CommandBridgeImpl{
		handlers: make(map[string]CommandHandler),
	}
}

// SendCommand 发送命令到 Shell
func (b *CommandBridgeImpl) SendCommand(ctx context.Context, sessionID string, command string) error {
	b.mu.RLock()
	handler, ok := b.handlers[sessionID]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no handler registered for session: %s", sessionID)
	}

	// 在 goroutine 中执行以支持超时
	errCh := make(chan error, 1)
	go func() {
		errCh <- handler(ctx, command)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendCommands 批量发送命令
func (b *CommandBridgeImpl) SendCommands(ctx context.Context, sessionID string, commands []string) error {
	for _, cmd := range commands {
		if err := b.SendCommand(ctx, sessionID, cmd); err != nil {
			return fmt.Errorf("failed to send command %q: %w", cmd, err)
		}
	}
	return nil
}

// RegisterHandler 注册命令处理器
func (b *CommandBridgeImpl) RegisterHandler(sessionID string, handler CommandHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.handlers[sessionID]; exists {
		return fmt.Errorf("handler already registered for session: %s", sessionID)
	}

	b.handlers[sessionID] = handler
	return nil
}

// UnregisterHandler 注销命令处理器
func (b *CommandBridgeImpl) UnregisterHandler(sessionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.handlers[sessionID]; !exists {
		return fmt.Errorf("no handler registered for session: %s", sessionID)
	}

	delete(b.handlers, sessionID)
	return nil
}

// HasHandler 检查是否有注册的处理器（用于测试）
func (b *CommandBridgeImpl) HasHandler(sessionID string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.handlers[sessionID]
	return ok
}

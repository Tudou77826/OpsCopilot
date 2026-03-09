// Package bridge 定义 Shell 和 Agent 组件之间的桥接层
package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// StateBridgeImpl 状态桥接实现
type StateBridgeImpl struct {
	mu       sync.RWMutex
	sessions map[string]SessionState
	watchers []chan StateChange
	watcherMu sync.Mutex
}

// NewStateBridge 创建状态桥接
func NewStateBridge() *StateBridgeImpl {
	return &StateBridgeImpl{
		sessions: make(map[string]SessionState),
		watchers: make([]chan StateChange, 0),
	}
}

// GetSessionInfo 获取会话信息
func (b *StateBridgeImpl) GetSessionInfo(sessionID string) (SessionState, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	state, ok := b.sessions[sessionID]
	if !ok {
		return SessionState{}, fmt.Errorf("session not found: %s", sessionID)
	}

	return state, nil
}

// UpdateSessionInfo 更新会话信息
func (b *StateBridgeImpl) UpdateSessionInfo(sessionID string, state SessionState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	oldState, exists := b.sessions[sessionID]
	state.ID = sessionID
	b.sessions[sessionID] = state

	// 通知观察者
	if exists {
		b.notifyWatchers(StateChange{
			SessionID: sessionID,
			Field:     "update",
			OldValue:  oldState,
			NewValue:  state,
			Timestamp: time.Now(),
		})
	} else {
		b.notifyWatchers(StateChange{
			SessionID: sessionID,
			Field:     "create",
			OldValue:  nil,
			NewValue:  state,
			Timestamp: time.Now(),
		})
	}

	return nil
}

// GetAllSessions 获取所有会话状态
func (b *StateBridgeImpl) GetAllSessions() map[string]SessionState {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[string]SessionState, len(b.sessions))
	for k, v := range b.sessions {
		result[k] = v
	}
	return result
}

// Watch 监听状态变化
func (b *StateBridgeImpl) Watch(ctx context.Context) <-chan StateChange {
	b.watcherMu.Lock()
	defer b.watcherMu.Unlock()

	ch := make(chan StateChange, 100)
	b.watchers = append(b.watchers, ch)

	// 当上下文取消时关闭通道
	go func() {
		<-ctx.Done()
		b.watcherMu.Lock()
		defer b.watcherMu.Unlock()

		// 从观察者列表中移除
		for i, watcher := range b.watchers {
			if watcher == ch {
				b.watchers = append(b.watchers[:i], b.watchers[i+1:]...)
				close(ch)
				break
			}
		}
	}()

	return ch
}

// notifyWatchers 通知所有观察者
func (b *StateBridgeImpl) notifyWatchers(change StateChange) {
	b.watcherMu.Lock()
	defer b.watcherMu.Unlock()

	for _, watcher := range b.watchers {
		select {
		case watcher <- change:
		default:
			// 通道已满，跳过
		}
	}
}

// DeleteSession 删除会话状态
func (b *StateBridgeImpl) DeleteSession(sessionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	oldState, exists := b.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	delete(b.sessions, sessionID)

	b.notifyWatchers(StateChange{
		SessionID: sessionID,
		Field:     "delete",
		OldValue:  oldState,
		NewValue:  nil,
		Timestamp: time.Now(),
	})

	return nil
}

// SessionCount 返回会话数量（用于测试）
func (b *StateBridgeImpl) SessionCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.sessions)
}

// Package bridge 定义 Shell 和 Agent 组件之间的桥接层
package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// EventBus 事件总线实现
type EventBus struct {
	mu            sync.RWMutex
	subscriptions map[string][]subscription
	allHandlers   map[string]subscription
	closed        bool
	bufferSize    int
}

type subscription struct {
	id        string
	eventType string
	handler   Handler
}

// NewEventBus 创建事件总线
func NewEventBus() *EventBus {
	return &EventBus{
		subscriptions: make(map[string][]subscription),
		allHandlers:   make(map[string]subscription),
		bufferSize:    100,
	}
}

// NewEventBusWithBuffer 创建带缓冲区大小的事件总线
func NewEventBusWithBuffer(bufferSize int) *EventBus {
	return &EventBus{
		subscriptions: make(map[string][]subscription),
		allHandlers:   make(map[string]subscription),
		bufferSize:    bufferSize,
	}
}

// Publish 发布事件
func (b *EventBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("event bus is closed")
	}

	// 设置时间戳
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// 调用特定事件类型的处理器
	if subs, ok := b.subscriptions[event.Type]; ok {
		for _, sub := range subs {
			if err := b.callHandler(ctx, sub, event); err != nil {
				// 记录错误但继续处理其他订阅者
				fmt.Printf("handler error for event %s: %v\n", event.Type, err)
			}
		}
	}

	// 调用订阅所有事件的处理器
	for _, sub := range b.allHandlers {
		if err := b.callHandler(ctx, sub, event); err != nil {
			fmt.Printf("all-handler error for event %s: %v\n", event.Type, err)
		}
	}

	return nil
}

func (b *EventBus) callHandler(ctx context.Context, sub subscription, event Event) error {
	// 在 goroutine 中调用处理器以避免阻塞
	errCh := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("handler panic: %v", r)
			}
		}()
		errCh <- sub.handler(ctx, event)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Subscribe 订阅事件类型
func (b *EventBus) Subscribe(eventType string, handler Handler) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return "", fmt.Errorf("event bus is closed")
	}

	sub := subscription{
		id:        uuid.New().String(),
		eventType: eventType,
		handler:   handler,
	}

	b.subscriptions[eventType] = append(b.subscriptions[eventType], sub)
	return sub.id, nil
}

// Unsubscribe 取消订阅
func (b *EventBus) Unsubscribe(subscriptionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("event bus is closed")
	}

	// 在特定事件类型中查找
	for eventType, subs := range b.subscriptions {
		for i, sub := range subs {
			if sub.id == subscriptionID {
				b.subscriptions[eventType] = append(subs[:i], subs[i+1:]...)
				return nil
			}
		}
	}

	// 在所有事件处理器中查找
	if _, ok := b.allHandlers[subscriptionID]; ok {
		delete(b.allHandlers, subscriptionID)
		return nil
	}

	return fmt.Errorf("subscription not found: %s", subscriptionID)
}

// SubscribeAll 订阅所有事件
func (b *EventBus) SubscribeAll(handler Handler) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return "", fmt.Errorf("event bus is closed")
	}

	sub := subscription{
		id:        uuid.New().String(),
		eventType: "*",
		handler:   handler,
	}

	b.allHandlers[sub.id] = sub
	return sub.id, nil
}

// Close 关闭事件总线
func (b *EventBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	b.subscriptions = make(map[string][]subscription)
	b.allHandlers = make(map[string]subscription)

	return nil
}

// SubscriptionCount 返回订阅数量（用于测试）
func (b *EventBus) SubscriptionCount(eventType string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscriptions[eventType])
}

// AllHandlerCount 返回全事件处理器数量（用于测试）
func (b *EventBus) AllHandlerCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.allHandlers)
}

// NewEvent 创建新事件
func NewEvent(eventType string, sessionID string, payload interface{}) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Source:    getEventSource(eventType),
		SessionID: sessionID,
		Payload:   payload,
	}
}

// getEventSource 根据事件类型推断事件来源
func getEventSource(eventType string) string {
	switch {
	case len(eventType) >= 5 && eventType[:5] == "shell":
		return "shell"
	case len(eventType) >= 5 && eventType[:5] == "agent":
		return "agent"
	case len(eventType) >= 9 && eventType[:9] == "recording":
		return "recording"
	default:
		return "system"
	}
}

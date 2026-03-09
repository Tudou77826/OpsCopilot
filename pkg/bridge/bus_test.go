package bridge

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventBus_Publish(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	var received atomic.Bool
	var receivedEvent Event

	_, err := bus.Subscribe(EventShellData, func(ctx context.Context, event Event) error {
		received.Store(true)
		receivedEvent = event
		return nil
	})
	require.NoError(t, err)

	err = bus.Publish(context.Background(), Event{
		Type:      EventShellData,
		SessionID: "test-session",
		Payload:   ShellDataPayload{Data: "hello"},
	})
	require.NoError(t, err)

	// 等待事件处理
	time.Sleep(10 * time.Millisecond)

	assert.True(t, received.Load())
	assert.Equal(t, "test-session", receivedEvent.SessionID)
	assert.Equal(t, EventShellData, receivedEvent.Type)
}

func TestEventBus_SubscribeAll(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	var counter atomic.Int32

	_, err := bus.SubscribeAll(func(ctx context.Context, event Event) error {
		counter.Add(1)
		return nil
	})
	require.NoError(t, err)

	// 发布多个不同类型的事件
	err = bus.Publish(context.Background(), Event{Type: EventShellData})
	require.NoError(t, err)

	err = bus.Publish(context.Background(), Event{Type: EventAgentStatus})
	require.NoError(t, err)

	err = bus.Publish(context.Background(), Event{Type: EventRecordingStart})
	require.NoError(t, err)

	// 等待事件处理
	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, int32(3), counter.Load())
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	var counter atomic.Int32

	subID, err := bus.Subscribe(EventShellData, func(ctx context.Context, event Event) error {
		counter.Add(1)
		return nil
	})
	require.NoError(t, err)

	// 发布第一个事件
	err = bus.Publish(context.Background(), Event{Type: EventShellData})
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	assert.Equal(t, int32(1), counter.Load())

	// 取消订阅
	err = bus.Unsubscribe(subID)
	require.NoError(t, err)

	// 发布第二个事件
	err = bus.Publish(context.Background(), Event{Type: EventShellData})
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)

	// 计数器应该仍然是 1
	assert.Equal(t, int32(1), counter.Load())
}

func TestEventBus_ConcurrentSubscribers(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	var counter atomic.Int32
	var wg sync.WaitGroup

	// 启动 100 个订阅者
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := bus.Subscribe(EventShellData, func(ctx context.Context, event Event) error {
				counter.Add(1)
				return nil
			})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	// 发布事件
	err := bus.Publish(context.Background(), Event{Type: EventShellData})
	require.NoError(t, err)

	// 等待所有处理器完成
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, int32(100), counter.Load())
}

func TestEventBus_ContextCancellation(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	var received atomic.Bool

	_, err := bus.Subscribe(EventShellData, func(ctx context.Context, event Event) error {
		received.Store(true)
		return nil
	})
	require.NoError(t, err)

	// 使用带超时的上下文测试
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err = bus.Publish(ctx, Event{Type: EventShellData})
	require.NoError(t, err)

	// 等待处理
	time.Sleep(20 * time.Millisecond)
	assert.True(t, received.Load())
}

func TestNewEvent(t *testing.T) {
	event := NewEvent(EventShellData, "session-123", ShellDataPayload{Data: "test"})

	assert.Equal(t, EventShellData, event.Type)
	assert.Equal(t, "session-123", event.SessionID)
	assert.Equal(t, "shell", event.Source)
	assert.False(t, event.Timestamp.IsZero())
}

func TestGetEventSource(t *testing.T) {
	tests := []struct {
		eventType string
		expected  string
	}{
		{EventShellConnect, "shell"},
		{EventShellData, "shell"},
		{EventAgentCommand, "agent"},
		{EventAgentStatus, "agent"},
		{EventRecordingStart, "recording"},
		{EventRecordingStop, "recording"},
		{EventSystemReady, "system"},
		{"unknown:event", "system"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			assert.Equal(t, tt.expected, getEventSource(tt.eventType))
		})
	}
}

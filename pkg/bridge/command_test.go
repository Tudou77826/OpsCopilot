package bridge

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandBridge_SendCommand(t *testing.T) {
	bridge := NewCommandBridge()

	var receivedCommand string
	err := bridge.RegisterHandler("session-1", func(ctx context.Context, command string) error {
		receivedCommand = command
		return nil
	})
	require.NoError(t, err)

	err = bridge.SendCommand(context.Background(), "session-1", "ls -la")
	require.NoError(t, err)
	assert.Equal(t, "ls -la", receivedCommand)
}

func TestCommandBridge_SendCommand_NoHandler(t *testing.T) {
	bridge := NewCommandBridge()

	err := bridge.SendCommand(context.Background(), "nonexistent", "ls")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no handler registered")
}

func TestCommandBridge_RegisterHandler_Duplicate(t *testing.T) {
	bridge := NewCommandBridge()

	err := bridge.RegisterHandler("session-1", func(ctx context.Context, command string) error {
		return nil
	})
	require.NoError(t, err)

	err = bridge.RegisterHandler("session-1", func(ctx context.Context, command string) error {
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestCommandBridge_UnregisterHandler(t *testing.T) {
	bridge := NewCommandBridge()

	err := bridge.RegisterHandler("session-1", func(ctx context.Context, command string) error {
		return nil
	})
	require.NoError(t, err)
	assert.True(t, bridge.HasHandler("session-1"))

	err = bridge.UnregisterHandler("session-1")
	require.NoError(t, err)
	assert.False(t, bridge.HasHandler("session-1"))
}

func TestCommandBridge_SendCommands(t *testing.T) {
	bridge := NewCommandBridge()

	var receivedCommands []string
	var mu sync.Mutex

	err := bridge.RegisterHandler("session-1", func(ctx context.Context, command string) error {
		mu.Lock()
		defer mu.Unlock()
		receivedCommands = append(receivedCommands, command)
		return nil
	})
	require.NoError(t, err)

	commands := []string{"ls", "pwd", "whoami"}
	err = bridge.SendCommands(context.Background(), "session-1", commands)
	require.NoError(t, err)

	// 等待命令执行
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	assert.Len(t, receivedCommands, 3)
	mu.Unlock()
}

func TestCommandBridge_ContextCancellation(t *testing.T) {
	bridge := NewCommandBridge()

	err := bridge.RegisterHandler("session-1", func(ctx context.Context, command string) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err = bridge.SendCommand(ctx, "session-1", "slow-command")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

func TestCommandBridge_ConcurrentAccess(t *testing.T) {
	bridge := NewCommandBridge()
	var wg sync.WaitGroup

	// 并发注册处理器
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sessionID := string(rune('a' + id))
			_ = bridge.RegisterHandler(sessionID, func(ctx context.Context, command string) error {
				return nil
			})
		}(i)
	}

	// 并发发送命令
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sessionID := string(rune('a' + id))
			_ = bridge.SendCommand(context.Background(), sessionID, "test")
		}(i)
	}

	wg.Wait()
}

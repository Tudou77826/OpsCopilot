package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateBridge_GetSessionInfo(t *testing.T) {
	bridge := NewStateBridge()

	// 测试不存在的会话
	_, err := bridge.GetSessionInfo("nonexistent")
	assert.Error(t, err)

	// 创建会话
	state := SessionState{
		ID:        "session-1",
		Host:      "example.com",
		User:      "test",
		Connected: true,
	}
	err = bridge.UpdateSessionInfo("session-1", state)
	require.NoError(t, err)

	// 获取会话信息
	got, err := bridge.GetSessionInfo("session-1")
	require.NoError(t, err)
	assert.Equal(t, "session-1", got.ID)
	assert.Equal(t, "example.com", got.Host)
	assert.Equal(t, "test", got.User)
	assert.True(t, got.Connected)
}

func TestStateBridge_GetAllSessions(t *testing.T) {
	bridge := NewStateBridge()

	// 初始为空
	sessions := bridge.GetAllSessions()
	assert.Empty(t, sessions)

	// 添加多个会话
	for i := 0; i < 3; i++ {
		state := SessionState{
			ID:        string(rune('a' + i)),
			Connected: true,
		}
		err := bridge.UpdateSessionInfo(string(rune('a'+i)), state)
		require.NoError(t, err)
	}

	sessions = bridge.GetAllSessions()
	assert.Len(t, sessions, 3)
}

func TestStateBridge_DeleteSession(t *testing.T) {
	bridge := NewStateBridge()

	state := SessionState{ID: "session-1"}
	err := bridge.UpdateSessionInfo("session-1", state)
	require.NoError(t, err)
	assert.Equal(t, 1, bridge.SessionCount())

	err = bridge.DeleteSession("session-1")
	require.NoError(t, err)
	assert.Equal(t, 0, bridge.SessionCount())

	// 删除不存在的会话
	err = bridge.DeleteSession("nonexistent")
	assert.Error(t, err)
}

func TestStateBridge_Watch(t *testing.T) {
	bridge := NewStateBridge()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changes := bridge.Watch(ctx)

	// 启动监听协程
	go func() {
		state := SessionState{ID: "session-1", Connected: true}
		_ = bridge.UpdateSessionInfo("session-1", state)
	}()

	// 等待变更
	select {
	case change := <-changes:
		assert.Equal(t, "session-1", change.SessionID)
		assert.Equal(t, "create", change.Field)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for state change")
	}
}

func TestStateBridge_WatchUpdate(t *testing.T) {
	bridge := NewStateBridge()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 先创建会话
	state := SessionState{ID: "session-1", Connected: true}
	err := bridge.UpdateSessionInfo("session-1", state)
	require.NoError(t, err)

	changes := bridge.Watch(ctx)

	// 更新会话
	go func() {
		updatedState := SessionState{ID: "session-1", Connected: false}
		_ = bridge.UpdateSessionInfo("session-1", updatedState)
	}()

	select {
	case change := <-changes:
		assert.Equal(t, "session-1", change.SessionID)
		assert.Equal(t, "update", change.Field)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for state change")
	}
}

func TestStateBridge_WatchDelete(t *testing.T) {
	bridge := NewStateBridge()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建会话
	state := SessionState{ID: "session-1"}
	_ = bridge.UpdateSessionInfo("session-1", state)

	changes := bridge.Watch(ctx)

	// 删除会话
	go func() {
		_ = bridge.DeleteSession("session-1")
	}()

	select {
	case change := <-changes:
		assert.Equal(t, "session-1", change.SessionID)
		assert.Equal(t, "delete", change.Field)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for state change")
	}
}

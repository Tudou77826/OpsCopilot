package shell

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockSession(t *testing.T) {
	info := SessionInfo{
		ID:     "test-1",
		Host:   "example.com",
		User:   "testuser",
		Active: true,
	}
	session := NewMockSession("test-1", info)

	t.Run("ID", func(t *testing.T) {
		assert.Equal(t, "test-1", session.ID())
	})

	t.Run("Info", func(t *testing.T) {
		got := session.Info()
		assert.Equal(t, "test-1", got.ID)
		assert.Equal(t, "example.com", got.Host)
	})

	t.Run("Send", func(t *testing.T) {
		err := session.Send("hello")
		require.NoError(t, err)
		assert.Equal(t, []string{"hello"}, session.GetDataSent())
	})

	t.Run("Close", func(t *testing.T) {
		assert.False(t, session.IsClosed())
		err := session.Close()
		require.NoError(t, err)
		assert.True(t, session.IsClosed())
	})
}

func TestMockManager(t *testing.T) {
	manager := NewMockManager()

	t.Run("Connect", func(t *testing.T) {
		config := ConnectConfig{
			ID:   "session-1",
			Host: "example.com",
			Port: 22,
			User: "testuser",
		}
		session, err := manager.Connect(context.Background(), config)
		require.NoError(t, err)
		assert.Equal(t, "session-1", session.ID())
	})

	t.Run("Get", func(t *testing.T) {
		session, ok := manager.Get("session-1")
		assert.True(t, ok)
		assert.NotNil(t, session)

		_, ok = manager.Get("nonexistent")
		assert.False(t, ok)
	})

	t.Run("List", func(t *testing.T) {
		sessions := manager.List()
		assert.Len(t, sessions, 1)
	})

	t.Run("Broadcast", func(t *testing.T) {
		err := manager.Broadcast([]string{"session-1"}, "test data")
		require.NoError(t, err)

		session, _ := manager.Get("session-1")
		mockSession := session.(*MockSession)
		assert.Contains(t, mockSession.GetDataSent(), "test data")
	})

	t.Run("Disconnect", func(t *testing.T) {
		err := manager.Disconnect("session-1")
		require.NoError(t, err)
		assert.Equal(t, 0, manager.SessionCount())
	})

	t.Run("DisconnectAll", func(t *testing.T) {
		// 添加多个会话
		for i := 0; i < 3; i++ {
			config := ConnectConfig{
				ID:   string(rune('a' + i)),
				Host: "example.com",
				Port: 22,
				User: "testuser",
			}
			_, _ = manager.Connect(context.Background(), config)
		}
		assert.Equal(t, 3, manager.SessionCount())

		err := manager.DisconnectAll()
		require.NoError(t, err)
		assert.Equal(t, 0, manager.SessionCount())
	})
}

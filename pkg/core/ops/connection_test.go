package ops

import (
	"testing"
	"time"
)

// TestCleanIdleConnections_RemovesTimedOut 超过 IdleTimeoutMin 的连接被移除
func TestCleanIdleConnections_RemovesTimedOut(t *testing.T) {
	m := &Manager{
		config:      &Config{IdleTimeoutMin: 15},
		connections: make(map[string]*Connection),
	}

	// 一个"很久没活动"的连接（1 小时前）
	old := &Connection{Name: "old-server", Host: "10.0.0.1"}
	old.LastActive.Store(time.Now().Add(-1 * time.Hour).UnixNano())
	m.connections["old-server"] = old

	// 一个"刚刚活动过"的连接
	fresh := &Connection{Name: "fresh-server", Host: "10.0.0.2"}
	fresh.LastActive.Store(time.Now().UnixNano())
	m.connections["fresh-server"] = fresh

	m.cleanIdleConnections()

	if _, exists := m.connections["old-server"]; exists {
		t.Errorf("idle connection 'old-server' should have been removed")
	}
	if _, exists := m.connections["fresh-server"]; !exists {
		t.Errorf("fresh connection 'fresh-server' should have been kept")
	}
}

// TestCleanIdleConnections_KeepsActive 刚刚活动的连接不被误删（边界：略小于超时）
func TestCleanIdleConnections_KeepsActive(t *testing.T) {
	m := &Manager{
		config:      &Config{IdleTimeoutMin: 15},
		connections: make(map[string]*Connection),
	}

	// 14 分钟前活动，未达 15 分钟超时
	recent := &Connection{Name: "recent", Host: "10.0.0.3"}
	recent.LastActive.Store(time.Now().Add(-14 * time.Minute).UnixNano())
	m.connections["recent"] = recent

	m.cleanIdleConnections()

	if _, exists := m.connections["recent"]; !exists {
		t.Errorf("connection active 14min ago should not be removed (timeout=15min)")
	}
}

// TestCleanIdleConnections_EmptyMap 空连接表不 panic
func TestCleanIdleConnections_EmptyMap(t *testing.T) {
	m := &Manager{
		config:      &Config{IdleTimeoutMin: 15},
		connections: make(map[string]*Connection),
	}
	m.cleanIdleConnections()
}

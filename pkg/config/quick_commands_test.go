package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newQuickCmdTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m := NewManager()
	m.configPath = filepath.Join(dir, "config.json")
	m.quickCommandsPath = filepath.Join(dir, "quick_commands.json")
	m.highlightRulesPath = filepath.Join(dir, "highlight_rules.json")
	if err := m.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	return m
}

// 意图化操作应正确更新内存并落盘
func TestQuickCommandIntentOperations(t *testing.T) {
	m := newQuickCmdTestManager(t)

	m.AddQuickCommand(QuickCommand{ID: "a", Name: "cmd-a", Content: "echo a", Group: "default"})
	m.AddQuickCommand(QuickCommand{ID: "b", Name: "cmd-b", Content: "echo b", Group: "default"})

	if ok := m.UpdateQuickCommand("a", QuickCommand{Name: "cmd-a2", Content: "echo a2", Group: "g1"}); !ok {
		t.Fatal("update should find id a")
	}
	// id 不可被更新改写
	if m.Config.QuickCommands[0].ID != "a" {
		t.Fatalf("id must stay immutable, got %q", m.Config.QuickCommands[0].ID)
	}
	if ok := m.UpdateQuickCommand("missing", QuickCommand{Name: "x"}); ok {
		t.Fatal("update of missing id should return false")
	}

	if ok := m.DeleteQuickCommand("b"); !ok {
		t.Fatal("delete should find id b")
	}
	if ok := m.DeleteQuickCommand("missing"); ok {
		t.Fatal("delete of missing id should return false")
	}

	// 落盘内容与内存一致
	data, err := os.ReadFile(m.quickCommandsPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var persisted []QuickCommand
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(persisted) != 1 || persisted[0].Name != "cmd-a2" || persisted[0].Group != "g1" {
		t.Fatalf("unexpected persisted commands: %+v", persisted)
	}
}

// 外部修改文件后，变化检测应重载内存并返回最新列表；
// 未变化时不得重复加载
func TestQuickCommandsChangedDetection(t *testing.T) {
	m := newQuickCmdTestManager(t)
	m.AddQuickCommand(QuickCommand{ID: "a", Name: "cmd-a", Content: "echo a", Group: "default"})

	if changed, _ := m.CheckQuickCommandsChanged(); changed {
		t.Fatal("own write should not be detected as external change")
	}

	// 模拟其他进程写入：绕过 Manager 直接改文件（mtime 精度为秒级时用 size 差异保证可检测）
	external := []QuickCommand{
		{ID: "a", Name: "cmd-a", Content: "echo a", Group: "default"},
		{ID: "ext", Name: "from-other-window", Content: "echo ext", Group: "default"},
	}
	data, _ := json.MarshalIndent(external, "", "  ")
	if err := os.WriteFile(m.quickCommandsPath, data, 0644); err != nil {
		t.Fatalf("write external: %v", err)
	}
	// 确保 mtime 与记录值不同（FAT/秒级 mtime 文件系统）
	time.Sleep(1100 * time.Millisecond)

	changed, cmds := m.CheckQuickCommandsChanged()
	if !changed {
		t.Fatal("external write should be detected")
	}
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands after reload, got %d", len(cmds))
	}
	if len(m.Config.QuickCommands) != 2 {
		t.Fatalf("memory should be reloaded, got %d", len(m.Config.QuickCommands))
	}

	// 再次检测应无变化
	if changed, _ := m.CheckQuickCommandsChanged(); changed {
		t.Fatal("second check without modification should report no change")
	}
}

// 并发读写不应死锁或写坏文件（-race 下验证）
func TestQuickCommandsConcurrentAccess(t *testing.T) {
	m := newQuickCmdTestManager(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			m.CheckQuickCommandsChanged()
		}
	}()
	for i := 0; i < 50; i++ {
		m.AddQuickCommand(QuickCommand{ID: string(rune('a' + i)), Name: "c", Content: "echo", Group: "default"})
	}
	<-done

	var persisted []QuickCommand
	data, err := os.ReadFile(m.quickCommandsPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("file corrupted after concurrent access: %v", err)
	}
}

// 拖拽排序：只重排给定 id 的相对顺序，其它命令位置不变；未知/重复 id 拒绝执行
func TestQuickCommandReorder(t *testing.T) {
	m := newQuickCmdTestManager(t)
	m.AddQuickCommand(QuickCommand{ID: "a", Name: "a", Content: "a", Group: "g1"})
	m.AddQuickCommand(QuickCommand{ID: "b", Name: "b", Content: "b", Group: "g1"})
	m.AddQuickCommand(QuickCommand{ID: "x", Name: "x", Content: "x", Group: "g2"})
	m.AddQuickCommand(QuickCommand{ID: "c", Name: "c", Content: "c", Group: "g1"})

	// g1 内 b 移到 c 后面：[a b c] -> [a c b]，g2 的 x（第 3 位）位置不变
	if ok := m.ReorderQuickCommands([]string{"a", "c", "b"}); !ok {
		t.Fatal("reorder should succeed")
	}
	got := make([]string, 0, 4)
	for _, c := range m.Config.QuickCommands {
		got = append(got, c.ID)
	}
	want := []string{"a", "c", "x", "b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}

	if ok := m.ReorderQuickCommands([]string{"a", "missing"}); ok {
		t.Fatal("reorder with unknown id should be rejected")
	}
	if ok := m.ReorderQuickCommands([]string{"a", "a"}); ok {
		t.Fatal("reorder with duplicate id should be rejected")
	}

	// 落盘校验：成功那次的重排已写入文件，被拒绝的两次不影响
	data, err := os.ReadFile(m.quickCommandsPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var persisted []QuickCommand
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(persisted) != 4 || persisted[3].ID != "b" {
		t.Fatalf("persisted order wrong: %+v", persisted)
	}
}

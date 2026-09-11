package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readPersistedQuickCommands(t *testing.T, m *Manager) []QuickCommand {
	t.Helper()
	data, err := os.ReadFile(m.quickCommandsPath)
	if err != nil {
		t.Fatalf("读取持久化文件失败: %v", err)
	}
	var cmds []QuickCommand
	if err := json.Unmarshal(data, &cmds); err != nil {
		t.Fatalf("解析持久化文件失败: %v", err)
	}
	return cmds
}

// 批量导入应一次写入全部条目，并给每条补上唯一 ID。
func TestImportQuickCommandsAppendsAndMintsIDs(t *testing.T) {
	m := newQuickCmdTestManager(t)

	added, err := m.ImportQuickCommands([]QuickCommand{
		{Name: "tail", Content: "tail -f /var/log/app.log", Group: "Xshell"},
		{Name: "磁盘", Content: "df -h", Group: "Xshell"},
		{Name: "tail", Content: "tail -f /var/log/app.log", Group: "Xshell"}, // 批内重复
	})
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if added != 2 {
		t.Fatalf("批内重复应只保留一条，期望写入 2 条，实际 %d", added)
	}

	persisted := readPersistedQuickCommands(t, m)
	if len(persisted) != 2 {
		t.Fatalf("落盘条数应为 2，实际 %d", len(persisted))
	}
	seen := map[string]bool{}
	for _, c := range persisted {
		if c.ID == "" {
			t.Fatalf("导入的条目必须带 ID: %+v", c)
		}
		if seen[c.ID] {
			t.Fatalf("ID 重复: %q", c.ID)
		}
		seen[c.ID] = true
		if !strings.HasPrefix(c.ID, "qc-") {
			t.Fatalf("ID 应带 qc- 前缀，实际 %q", c.ID)
		}
		if c.Group != "Xshell" {
			t.Fatalf("分组应为 Xshell，实际 %q", c.Group)
		}
	}
}

// 目标分组内同名同内容视为已存在而跳过；同名不同内容、同内容不同名都应导入。
func TestImportQuickCommandsSkipsExistingByNameAndContent(t *testing.T) {
	m := newQuickCmdTestManager(t)
	m.AddQuickCommand(QuickCommand{ID: "a", Name: "tail", Content: "tail -f app.log", Group: "Xshell"})

	added, err := m.ImportQuickCommands([]QuickCommand{
		{Name: "tail", Content: "tail -f app.log", Group: "Xshell"},   // 完全相同 → 跳过
		{Name: "tail", Content: "tail -f other.log", Group: "Xshell"}, // 内容不同 → 导入
		{Name: "日志", Content: "tail -f app.log", Group: "Xshell"},     // 名称不同 → 导入
		{Name: "tail", Content: "tail -f app.log", Group: "另一个分组"},    // 分组不同 → 导入
	})
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if added != 3 {
		t.Fatalf("期望写入 3 条，实际 %d", added)
	}
	if got := len(readPersistedQuickCommands(t, m)); got != 4 {
		t.Fatalf("落盘应为 1+3=4 条，实际 %d", got)
	}
}

// 全部是重复项时不应触发写盘（沿用"内容未变化则跳过写盘"的既有行为）。
func TestImportQuickCommandsSkipsWriteWhenNothingNew(t *testing.T) {
	m := newQuickCmdTestManager(t)
	m.AddQuickCommand(QuickCommand{ID: "a", Name: "tail", Content: "tail -f app.log", Group: "Xshell"})

	beforeMod, beforeSize := m.quickCmdMod, m.quickCmdSize

	added, err := m.ImportQuickCommands([]QuickCommand{
		{Name: "tail", Content: "tail -f app.log", Group: "Xshell"},
	})
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if added != 0 {
		t.Fatalf("重复项不应写入，实际 %d", added)
	}
	// recordQuickCmdStat 只在真正写盘后调用，因此这两个值不变即可证明没有落盘。
	if !m.quickCmdMod.Equal(beforeMod) || m.quickCmdSize != beforeSize {
		t.Fatalf("无新增时不应写盘（mod/size 被刷新）")
	}
}

// 空分组应归一到 default，与 migration 的既有约定一致。
func TestImportQuickCommandsNormalizesEmptyGroup(t *testing.T) {
	m := newQuickCmdTestManager(t)
	if _, err := m.ImportQuickCommands([]QuickCommand{{Name: "n", Content: "c"}}); err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	persisted := readPersistedQuickCommands(t, m)
	if len(persisted) != 1 || persisted[0].Group != "default" {
		t.Fatalf("空分组应写入 default，实际 %+v", persisted)
	}
}

// 写盘失败时必须回滚内存，不能让调用方拿到 error 而内存已悄悄变化。
func TestImportQuickCommandsRollsBackWhenSaveFails(t *testing.T) {
	m := newQuickCmdTestManager(t)
	m.AddQuickCommand(QuickCommand{ID: "a", Name: "keep", Content: "keep", Group: "Xshell"})

	// 把目标路径换成目录：临时文件能建，rename 覆盖目录必然失败。
	dir := filepath.Join(t.TempDir(), "quick_commands.json")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("准备失败: %v", err)
	}
	m.quickCommandsPath = dir

	added, err := m.ImportQuickCommands([]QuickCommand{{Name: "n", Content: "c", Group: "Xshell"}})
	if err == nil {
		t.Fatal("目标不可写时应返回错误")
	}
	if added != 0 {
		t.Fatalf("失败时不应报告写入条数，实际 %d", added)
	}
	if len(m.Config.QuickCommands) != 1 {
		t.Fatalf("失败时应回滚内存，实际 %d 条", len(m.Config.QuickCommands))
	}
}

// 原子写不应留下临时文件残留。
func TestSaveQuickCommandsLeavesNoTempFiles(t *testing.T) {
	m := newQuickCmdTestManager(t)
	if _, err := m.ImportQuickCommands([]QuickCommand{{Name: "n", Content: "c", Group: "Xshell"}}); err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	m.AddQuickCommand(QuickCommand{ID: "x", Name: "x", Content: "x", Group: "Xshell"})

	entries, err := os.ReadDir(filepath.Dir(m.quickCommandsPath))
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("遗留临时文件: %s", e.Name())
		}
	}
}

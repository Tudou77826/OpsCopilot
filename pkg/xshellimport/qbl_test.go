package xshellimport

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// encodeGBKForTest 把文本编成 GBK，用于覆盖老版本 / 中文版 Xshell 的编码回退。
func encodeGBKForTest(text string) ([]byte, error) {
	return simplifiedchinese.GBK.NewEncoder().Bytes([]byte(text))
}

// qblUTF16LE 按 Xshell 的写法编码（UTF-16LE + BOM），与真实文件一致的字节形态。
func qblUTF16LE(t *testing.T, text string) []byte {
	t.Helper()
	out := []byte{0xFF, 0xFE}
	for _, r := range text {
		if r > 0xFFFF {
			t.Fatalf("测试样本不应包含四字节字符: %q", r)
		}
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

// 本机实测的 .qbl 形态（Xshell 8.2，一条按钮）。
func realQBLText() string {
	return "[Info]\n" +
		"Version=8.2\n" +
		"Count=1\n" +
		"Expanded=1\n" +
		"[QuickButton]\n" +
		"Button_0_Name=tail\n" +
		"Button_0_Param=\n" +
		"Button_0_Icon=0\n" +
		"Button_0_Desc=\n" +
		"Button_0_Type=1\n" +
		"Button_0_Action=tail -f /var/log/$app/app.log\n"
}

func TestParseQBL_RealShape(t *testing.T) {
	buttons := ParseQBL(realQBLText())
	if len(buttons) != 1 {
		t.Fatalf("应解析出 1 个按钮，实际 %d", len(buttons))
	}
	b := buttons[0]
	if b.Name != "tail" || b.Content != "tail -f /var/log/$app/app.log" || b.Type != "1" {
		t.Fatalf("字段解析错误: %+v", b)
	}
	if ok, reason := ButtonSupported(b); !ok {
		t.Fatalf("Type=1 的按钮应可导入，却被拒绝: %s", reason)
	}
}

// Count 缺失时应扫描实际键名兜底，而不是一个按钮都读不到。
func TestParseQBL_FallsBackToKeyScanWhenCountMissing(t *testing.T) {
	text := "[QuickButton]\n" +
		"Button_0_Name=a\nButton_0_Action=echo a\n" +
		"Button_1_Name=b\nButton_1_Action=echo b\n"
	buttons := ParseQBL(text)
	if len(buttons) != 2 {
		t.Fatalf("Count 缺失时应扫描出 2 个按钮，实际 %d", len(buttons))
	}
	if buttons[0].Name != "a" || buttons[1].Name != "b" {
		t.Fatalf("按钮顺序或字段错误: %+v", buttons)
	}
}

// Count 写小了也不该丢按钮：以 Count 为准索引，Extra 下标由键扫描补足。
func TestParseQBL_CountDrivesIndices(t *testing.T) {
	text := "[Info]\nCount=3\n[QuickButton]\n" +
		"Button_0_Name=a\nButton_0_Action=echo a\n" +
		"Button_1_Name=b\nButton_1_Action=echo b\n" +
		"Button_2_Name=c\nButton_2_Action=echo c\n"
	buttons := ParseQBL(text)
	if len(buttons) != 3 {
		t.Fatalf("应解析出 3 个按钮，实际 %d", len(buttons))
	}
	if buttons[2].Name != "c" {
		t.Fatalf("第 3 个按钮解析错误: %+v", buttons[2])
	}
}

func TestButtonSupported(t *testing.T) {
	cases := []struct {
		name       string
		button     QuickButton
		supported  bool
		reasonHint string
	}{
		{"发送字符串", QuickButton{Name: "n", Content: "c", Type: "1"}, true, ""},
		{"Type 缺失按可导入", QuickButton{Name: "n", Content: "c"}, true, ""},
		{"脚本类跳过", QuickButton{Name: "脚本", Content: "c", Type: "2"}, false, "类型为 2"},
		{"缺名称跳过", QuickButton{Name: "", Content: "c", Type: "1"}, false, "缺少名称"},
		{"缺内容跳过", QuickButton{Name: "n", Content: "", Type: "1"}, false, "没有命令内容"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := ButtonSupported(tc.button)
			if ok != tc.supported {
				t.Fatalf("supported=%v，期望 %v（原因 %q）", ok, tc.supported, reason)
			}
			if tc.reasonHint != "" && !strings.Contains(reason, tc.reasonHint) {
				t.Fatalf("原因 %q 应包含 %q", reason, tc.reasonHint)
			}
		})
	}
}

func TestParseQuickButtonPath_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.qbl")
	if err := os.WriteFile(path, qblUTF16LE(t, realQBLText()), 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}

	sets, warnings, err := ParseQuickButtonPath(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("不应有警告: %v", warnings)
	}
	if len(sets) != 1 {
		t.Fatalf("应有 1 个集合，实际 %d", len(sets))
	}
	if sets[0].Name != "commands" {
		t.Fatalf("集合名应取文件名，实际 %q", sets[0].Name)
	}
	if len(sets[0].Buttons) != 1 {
		t.Fatalf("应有 1 个按钮，实际 %d", len(sets[0].Buttons))
	}
}

// GBK 编码的 .qbl（老版本 / 中文版 Xshell）同样要能读出来。
func TestParseQuickButtonPath_GBKEncoded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "旧版.qbl")
	text := "[Info]\nCount=1\n[QuickButton]\nButton_0_Name=磁盘\nButton_0_Action=df -h\nButton_0_Type=1\n"

	gbk, err := encodeGBKForTest(text)
	if err != nil {
		t.Skipf("本环境无法编码 GBK 样本: %v", err)
	}
	if err := os.WriteFile(path, gbk, 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}

	sets, _, err := ParseQuickButtonPath(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Buttons) != 1 {
		t.Fatalf("GBK 样本应解析出 1 个按钮: %+v", sets)
	}
	if sets[0].Buttons[0].Name != "磁盘" {
		t.Fatalf("GBK 中文名称解码错误: %q", sets[0].Buttons[0].Name)
	}
}

// 目录里只认 .qbl，非 .qbl 文件与子目录里的 .qbl 都要正确处理。
func TestParseQuickButtonPath_Directory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "commands.qbl"), qblUTF16LE(t, realQBLText()), 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}
	// 干扰项：同目录的 .ini 与子目录里的 .qbl
	if err := os.WriteFile(filepath.Join(dir, "trigger.ini"), []byte("Count=0\n"), 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("准备目录失败: %v", err)
	}
	nested := "[Info]\nCount=1\n[QuickButton]\nButton_0_Name=nested\nButton_0_Action=echo nested\nButton_0_Type=1\n"
	if err := os.WriteFile(filepath.Join(sub, "nested.qbl"), []byte(nested), 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}

	sets, warnings, err := ParseQuickButtonPath(dir)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("不应有警告: %v", warnings)
	}
	if len(sets) != 2 {
		t.Fatalf("应解析出 2 个集合，实际 %d", len(sets))
	}
	// 行序稳定：按路径排序
	if sets[0].Name != "commands" || sets[1].Name != "nested" {
		t.Fatalf("集合顺序或名称错误: %q, %q", sets[0].Name, sets[1].Name)
	}
}

func TestParseQuickButtonPath_EmptyDirectoryWarns(t *testing.T) {
	sets, warnings, err := ParseQuickButtonPath(t.TempDir())
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(sets) != 0 {
		t.Fatalf("空目录不应解析出集合，实际 %d", len(sets))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "没有找到 .qbl") {
		t.Fatalf("空目录应给出明确警告，实际 %v", warnings)
	}
}

func TestParseQuickButtonPath_RejectsNonQBLAndMissing(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "a.ini")
	if err := os.WriteFile(other, []byte("x=1\n"), 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}
	if _, _, err := ParseQuickButtonPath(other); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("非 .qbl 文件应返回 ErrInvalid，实际 %v", err)
	}
	if _, _, err := ParseQuickButtonPath(filepath.Join(dir, "nope.qbl")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("不存在的文件应返回 ErrNotExist，实际 %v", err)
	}
	if _, _, err := ParseQuickButtonPath("  "); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("空路径应返回 ErrNotExist，实际 %v", err)
	}
}

// 空 .qbl（没有按钮）要给文件级警告，而不是静默当成功。
func TestParseQuickButtonPath_FileWithoutButtonsWarns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.qbl")
	if err := os.WriteFile(path, qblUTF16LE(t, "[Info]\nVersion=8.2\nCount=0\n"), 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}
	sets, _, err := ParseQuickButtonPath(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(sets) != 1 || len(sets[0].Warnings) == 0 {
		t.Fatalf("没有按钮的文件应带警告: %+v", sets)
	}
}

// 目录探测：注入临时根目录，验证 <root>/<版本>/Xshell/<leaf> 的枚举、版本排序与
// "必须含 .qbl 才列为来源"的过滤。
func TestDiscoverAssetDirsUnder(t *testing.T) {
	root := t.TempDir()
	mk := func(parts ...string) string {
		p := filepath.Join(append([]string{root}, parts...)...)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("准备目录失败: %v", err)
		}
		return p
	}

	mk("Documents", "NetSarang Computer", "8", "Xshell", "Sessions")
	mk("Documents", "NetSarang Computer", "10", "Xshell", "Sessions")
	mk("Documents", "NetSarang Computer", "9", "Xshell", "QuickButton Files")
	// 空目录与"只有别的扩展名文件"的目录都不该被列为来源
	mk("Documents", "NetSarang Computer", "11", "Xshell", "QuickButton Files")
	withQBL := mk("Documents", "NetSarang Computer", "12", "Xshell", "QuickButton Files")
	if err := os.WriteFile(filepath.Join(withQBL, "commands.qbl"), []byte("x"), 0o644); err != nil {
		t.Fatalf("准备文件失败: %v", err)
	}

	roots := []string{filepath.Join(root, "Documents", "NetSarang Computer")}

	sessions := discoverAssetDirsUnder(roots, "Sessions", "")
	if len(sessions) != 2 {
		t.Fatalf("应探测到 2 个会话目录，实际 %d: %v", len(sessions), sessions)
	}
	// 版本号大的排前面："10" 要排在 "8" 之前
	if assetDirVersion(sessions[0]) != 10 || assetDirVersion(sessions[1]) != 8 {
		t.Fatalf("版本排序错误: %v", sessions)
	}

	// 9 号目录没有 .qbl，11 号目录为空 —— 都不该列为来源
	quick := discoverAssetDirsUnder(roots, "QuickButton Files", ".qbl")
	if len(quick) != 1 {
		t.Fatalf("只应探测到含 .qbl 的目录，实际 %d: %v", len(quick), quick)
	}
	if quick[0] != withQBL {
		t.Fatalf("探测到的目录错误: %q（期望 %q）", quick[0], withQBL)
	}
}

// 本机存在 Xshell 快捷按钮目录时，真实文件必须能解析出按钮（无则跳过）。
func TestRealMachineXshellQuickButtons(t *testing.T) {
	dirs := DiscoverXshellQuickButtonDirs()
	if len(dirs) == 0 {
		t.Skip("本机未检测到 Xshell 快捷按钮目录")
	}
	sets, warnings, err := ParseQuickButtonPath(dirs[0])
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", dirs[0], err)
	}
	if len(sets) == 0 {
		t.Skipf("本机快捷按钮目录里没有 .qbl: %v", warnings)
	}
	total := 0
	for _, set := range sets {
		total += len(set.Buttons)
	}
	if total == 0 {
		t.Fatalf("本机 .qbl 里一个按钮都没解析出来: %+v", sets)
	}
	t.Logf("本机解析出 %d 个快捷按钮集合、%d 个按钮", len(sets), total)
}

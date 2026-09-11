package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opscopilot/pkg/config"
)

// newQuickCommandImportTestApp 构造只填充配置管理器的 App，用于覆盖导入边界。
func newQuickCommandImportTestApp(t *testing.T, workDir string) *App {
	t.Helper()
	mgr := config.NewManagerWithDir(workDir)
	if err := mgr.Load(); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	return &App{configMgr: mgr}
}

// writeQBL 写一个 UTF-16LE + BOM 的 .qbl，与真实 Xshell 8 的字节形态一致。
func writeQBL(t *testing.T, path, text string) string {
	t.Helper()
	out := []byte{0xFF, 0xFE}
	for _, r := range text {
		out = append(out, byte(r), byte(r>>8))
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("写夹具失败: %v", err)
	}
	return path
}

const cleanQBL = "[Info]\nVersion=8.2\nCount=2\n[QuickButton]\n" +
	"Button_0_Name=tail\nButton_0_Type=1\nButton_0_Action=tail -f /var/log/app.log\n" +
	"Button_1_Name=df\nButton_1_Type=1\nButton_1_Action=df -h\n"

const mixedQBL = "[Info]\nVersion=8.2\nCount=2\n[QuickButton]\n" +
	"Button_0_Name=df\nButton_0_Type=1\nButton_0_Action=df -h\n" +
	"Button_1_Name=脚本按钮\nButton_1_Type=2\nButton_1_Action=echo script\n"

// 分析结果的集合字段必须是空集合而不是 null（空目录这个最容易被忽略的输入）。
func TestQuickCommandImportAnalysis_EmptyDirectoryCollectionsAreNeverNull(t *testing.T) {
	app := newQuickCommandImportTestApp(t, t.TempDir())

	analysis, err := app.AnalyzeQuickCommandImport(t.TempDir(), QuickCommandImportOptions{DefaultGroup: "Xshell"})
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	// 没有任何集合，因此这两项必须是空数组
	assertJSONFieldIsEmptyCollection(t, "空目录的分析结果", analysis, "groups")
	assertJSONFieldIsEmptyCollection(t, "空目录的分析结果", analysis, "rows")
	// 空目录会明确告警（"没有找到 .qbl"），所以要断言非 null 而非为空
	assertJSONFieldIsNotEmpty(t, "空目录的分析结果", analysis, "warnings")

	if analysis.Sets != 0 || analysis.Buttons != 0 {
		t.Fatalf("空目录应全为 0: %+v", analysis)
	}
}

// 真实形状下的字段不能是 null，且数字要自洽。
func TestQuickCommandImportAnalysis_MatchesFrontendExpectations(t *testing.T) {
	workDir := t.TempDir()
	app := newQuickCommandImportTestApp(t, workDir)
	qblDir := filepath.Join(workDir, "qbl")
	if err := os.MkdirAll(qblDir, 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	writeQBL(t, filepath.Join(qblDir, "commands.qbl"), cleanQBL)

	analysis, err := app.AnalyzeQuickCommandImport(qblDir, QuickCommandImportOptions{DefaultGroup: "Xshell"})
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	if analysis.Sets != 1 || analysis.Buttons != 2 || analysis.Importable != 2 {
		t.Fatalf("统计错误: %+v", analysis)
	}
	if analysis.Importable+analysis.Unsupported+analysis.Existing != analysis.Buttons {
		t.Fatalf("三个桶相加应等于按钮总数: %+v", analysis)
	}
	assertJSONFieldIsNotEmpty(t, "分析结果", analysis, "rows")
	assertJSONFieldIsNotEmpty(t, "分析结果", analysis, "groups")

	rows, ok := toJSONMap(t, analysis)["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("rows 应是 1 个元素的数组: %#v", toJSONMap(t, analysis)["rows"])
	}
	// 每行都要有可编辑的分组名，前端据此渲染输入框的初值。
	row := rows[0].(map[string]any)
	if row["group"] != "Xshell" {
		t.Fatalf("行内的建议分组错误: %#v", row["group"])
	}
	if row["source"] == "" || row["source"] == nil {
		t.Fatalf("行内缺少 source（执行导入时要用它回指分组）: %#v", row)
	}
	// 行内的按钮数是该集合自己的条数：实机上这一项漏填过，界面显示成"共 0 条"。
	if row["buttons"] != float64(2) {
		t.Fatalf("行内按钮数应为本集合条数 2，实际 %#v", row["buttons"])
	}
	if row["importable"] != float64(2) {
		t.Fatalf("行内可导入数错误: %#v", row["importable"])
	}
}

// 端到端：分析与执行走同一份输入，结果必须一致；执行后命令真的落盘。
func TestQuickCommandImport_AppliesAndPersists(t *testing.T) {
	workDir := t.TempDir()
	app := newQuickCommandImportTestApp(t, workDir)
	qblDir := filepath.Join(workDir, "qbl")
	if err := os.MkdirAll(qblDir, 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	source := writeQBL(t, filepath.Join(qblDir, "commands.qbl"), cleanQBL)

	analysis, err := app.AnalyzeQuickCommandImport(qblDir, QuickCommandImportOptions{DefaultGroup: "Xshell"})
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	report, err := app.ApplyQuickCommandImport(qblDir, []QuickCommandGroupAssignment{
		{Source: source, Group: "运维"},
	}, QuickCommandImportOptions{DefaultGroup: "Xshell"})
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}

	if report.Imported != analysis.Importable {
		t.Fatalf("实际导入 %d 与预览 %d 不一致", report.Imported, analysis.Importable)
	}
	if len(report.Groups) != 1 || report.Groups[0] != "运维" {
		t.Fatalf("分组指派未生效: %v", report.Groups)
	}

	// 真的落盘了，而且分组是指派后的名字
	persisted := loadPersistedQuickCommands(t, workDir)
	if len(persisted) != 2 {
		t.Fatalf("落盘应为 2 条，实际 %d: %+v", len(persisted), persisted)
	}
	for _, cmd := range persisted {
		if cmd.Group != "运维" {
			t.Fatalf("落盘分组错误: %+v", cmd)
		}
		if cmd.ID == "" {
			t.Fatalf("落盘条目缺少 ID: %+v", cmd)
		}
	}
}

// 重复导入同一个 .qbl 必须幂等：第二次全部判为已存在，命令库不增长。
func TestQuickCommandImport_IsIdempotent(t *testing.T) {
	workDir := t.TempDir()
	app := newQuickCommandImportTestApp(t, workDir)
	qblDir := filepath.Join(workDir, "qbl")
	if err := os.MkdirAll(qblDir, 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	writeQBL(t, filepath.Join(qblDir, "commands.qbl"), cleanQBL)

	first, err := app.ApplyQuickCommandImport(qblDir, nil, QuickCommandImportOptions{DefaultGroup: "Xshell"})
	if err != nil {
		t.Fatalf("首次导入失败: %v", err)
	}
	if first.Imported != 2 {
		t.Fatalf("首次应导入 2 条，实际 %d", first.Imported)
	}

	second, err := app.ApplyQuickCommandImport(qblDir, nil, QuickCommandImportOptions{DefaultGroup: "Xshell"})
	if err != nil {
		t.Fatalf("再次导入失败: %v", err)
	}
	if second.Imported != 0 || second.SkippedExisting != 2 {
		t.Fatalf("重复导入应全部跳过: %+v", second)
	}

	if got := len(loadPersistedQuickCommands(t, workDir)); got != 2 {
		t.Fatalf("重复导入后命令库不应增长，实际 %d 条", got)
	}
}

// 不支持的类型要被跳过并说明原因，而不是静默丢弃。
func TestQuickCommandImport_ReportsUnsupportedButtons(t *testing.T) {
	workDir := t.TempDir()
	app := newQuickCommandImportTestApp(t, workDir)
	qblDir := filepath.Join(workDir, "qbl")
	if err := os.MkdirAll(qblDir, 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	writeQBL(t, filepath.Join(qblDir, "mixed.qbl"), mixedQBL)

	report, err := app.ApplyQuickCommandImport(qblDir, nil, QuickCommandImportOptions{DefaultGroup: "Xshell"})
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if report.Imported != 1 || report.SkippedUnsupported != 1 {
		t.Fatalf("应导入 1 条、跳过 1 条: %+v", report)
	}

	found := false
	for _, w := range report.Warnings {
		if strings.Contains(w, "脚本按钮") && strings.Contains(w, "类型为 2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("告警里应说明跳过原因与原始类型: %v", report.Warnings)
	}
}

// 错误路径要给出能照着做的中文提示，而不是底层错误原文。
func TestQuickCommandImport_ErrorMessages(t *testing.T) {
	app := newQuickCommandImportTestApp(t, t.TempDir())

	if _, err := app.AnalyzeQuickCommandImport("", QuickCommandImportOptions{}); err == nil {
		t.Fatal("空路径应报错")
	} else if !strings.Contains(err.Error(), "请先选择") {
		t.Fatalf("空路径提示不合适: %v", err)
	}

	if _, err := app.AnalyzeQuickCommandImport(filepath.Join(t.TempDir(), "nope.qbl"), QuickCommandImportOptions{}); err == nil {
		t.Fatal("路径不存在应报错")
	} else if !strings.Contains(err.Error(), "路径不存在") {
		t.Fatalf("路径不存在的提示不合适: %v", err)
	}

	other := filepath.Join(t.TempDir(), "a.ini")
	if err := os.WriteFile(other, []byte("x=1\n"), 0o644); err != nil {
		t.Fatalf("写夹具失败: %v", err)
	}
	if _, err := app.AnalyzeQuickCommandImport(other, QuickCommandImportOptions{}); err == nil {
		t.Fatal("非 .qbl 应报错")
	} else if !strings.Contains(err.Error(), "不支持的文件类型") {
		t.Fatalf("类型不支持的提示不合适: %v", err)
	}
}

// 未注入 config 管理器时（测试或精简宿主），探测不应 panic。
func TestDetectXshellQuickButtonDirs_NeverPanics(t *testing.T) {
	app := &App{}
	dirs, err := app.DetectXshellQuickButtonDirs()
	if err != nil {
		t.Fatalf("探测失败: %v", err)
	}
	// 无论本机有没有 Xshell，都必须是非 nil 数组
	if dirs == nil {
		t.Fatal("返回的目录列表不得为 nil（前端按数组处理）")
	}
}

func loadPersistedQuickCommands(t *testing.T, workDir string) []config.QuickCommand {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(workDir, "quick_commands.json"))
	if err != nil {
		t.Fatalf("读取命令库失败: %v", err)
	}
	var cmds []config.QuickCommand
	if err := json.Unmarshal(raw, &cmds); err != nil {
		t.Fatalf("解析命令库失败: %v", err)
	}
	return cmds
}

package xshellimport

import (
	"errors"
	"strings"
	"testing"
)

// fakeCommandWriter 是 CommandWriter 的内存实现，顺带记录批量写入次数——
// "整批只写一次盘"是导入的关键性质，必须能断言。
type fakeCommandWriter struct {
	existing  []ImportedCommand
	added     []ImportedCommand
	addCalls  int
	failOnAdd bool
}

func (w *fakeCommandWriter) ExistingCommands() []ImportedCommand { return w.existing }

func (w *fakeCommandWriter) AddCommands(items []ImportedCommand) (int, error) {
	w.addCalls++
	if w.failOnAdd {
		return 0, errors.New("写盘失败")
	}
	w.added = append(w.added, items...)
	return len(items), nil
}

func buttonSet(path, name string, buttons ...QuickButton) *QuickButtonSet {
	return &QuickButtonSet{Path: path, Name: name, Buttons: buttons}
}

func cmd(name, content string) QuickButton {
	return QuickButton{Name: name, Content: content, Type: "1"}
}

func TestAnalyzeQuickCommands_Counts(t *testing.T) {
	sets := []*QuickButtonSet{
		buttonSet(`C:\q\commands.qbl`, "commands",
			cmd("tail", "tail -f app.log"),
			cmd("df", "df -h"),
			QuickButton{Name: "脚本", Content: "echo script", Type: "2"}, // 类型不支持
			QuickButton{Name: "", Content: "echo x", Type: "1"},        // 缺名称
		),
		buttonSet(`C:\q\ops.qbl`, "ops",
			cmd("tail", "tail -f app.log"), // 与第一个集合重复（同一目标分组）
			cmd("free", "free -h"),
		),
	}
	writer := &fakeCommandWriter{
		existing: []ImportedCommand{{Group: "Xshell", Name: "已存在", Content: "whoami"}},
	}

	plan := AnalyzeQuickCommands(sets, writer, "Xshell")

	if plan.Sets != 2 {
		t.Fatalf("集合数应为 2，实际 %d", plan.Sets)
	}
	if plan.Buttons != 6 {
		t.Fatalf("按钮总数应为 6，实际 %d", plan.Buttons)
	}
	if plan.Unsupported != 2 {
		t.Fatalf("不支持数应为 2，实际 %d", plan.Unsupported)
	}
	if plan.Importable != 3 {
		t.Fatalf("可导入数应为 3（tail/df/free，跨集合重复只算一条），实际 %d", plan.Importable)
	}
	if plan.Existing != 1 {
		t.Fatalf("重复数应为 1，实际 %d", plan.Existing)
	}
	// 三个桶相加必须等于按钮总数，否则界面上的数字自相矛盾
	if plan.Importable+plan.Unsupported+plan.Existing != plan.Buttons {
		t.Fatalf("可导入+不支持+重复 应等于按钮总数: %d+%d+%d != %d",
			plan.Importable, plan.Unsupported, plan.Existing, plan.Buttons)
	}
	if len(plan.Groups) != 1 || plan.Groups[0] != "Xshell" {
		t.Fatalf("目标分组应为 [Xshell]，实际 %v", plan.Groups)
	}
	if len(plan.Rows) != 2 {
		t.Fatalf("应有两行集合明细，实际 %d", len(plan.Rows))
	}
	if plan.Rows[0].Importable != 2 || plan.Rows[0].Unsupported != 2 {
		t.Fatalf("第一行统计错误: %+v", plan.Rows[0])
	}
	if plan.Rows[1].Importable != 1 || plan.Rows[1].Existing != 1 {
		t.Fatalf("第二行统计错误: %+v", plan.Rows[1])
	}
	if len(plan.Warnings) != 2 {
		t.Fatalf("应有两条例外告警，实际 %v", plan.Warnings)
	}
}

// nil writer 不应 panic（预览阶段可能没有写入端口）。
func TestAnalyzeQuickCommands_NilWriter(t *testing.T) {
	plan := AnalyzeQuickCommands([]*QuickButtonSet{buttonSet("a.qbl", "a", cmd("n", "c"))}, nil, "Xshell")
	if plan.Importable != 1 || plan.Existing != 0 {
		t.Fatalf("nil writer 时应全部可导入，实际 %+v", plan)
	}
}

// 空输入必须返回非 nil 的集合，避免前端拿到 null（上一轮黑屏的根因）。
func TestAnalyzeQuickCommands_EmptyInputHasNonNilCollections(t *testing.T) {
	plan := AnalyzeQuickCommands(nil, &fakeCommandWriter{}, "Xshell")
	if plan.Groups == nil || plan.Rows == nil || plan.Warnings == nil {
		t.Fatalf("集合字段不得为 nil: groups=%v rows=%v warnings=%v", plan.Groups, plan.Rows, plan.Warnings)
	}
	if plan.Sets != 0 || plan.Buttons != 0 {
		t.Fatalf("空输入应全为 0，实际 %+v", plan)
	}
}

func TestApplyQuickCommands_AssignsGroupsAndWritesOnce(t *testing.T) {
	sets := []*QuickButtonSet{
		buttonSet(`C:\q\commands.qbl`, "commands", cmd("tail", "tail -f app.log"), cmd("df", "df -h")),
		buttonSet(`C:\q\ops.qbl`, "ops", cmd("free", "free -h")),
	}
	writer := &fakeCommandWriter{}
	assignments := map[string]string{
		`C:\q\commands.qbl`: "运维",
		`C:\q\ops.qbl`:      "运维", // 两个集合填同一分组 → 合并
	}

	report, err := ApplyQuickCommands(sets, assignments, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if writer.addCalls != 1 {
		t.Fatalf("整批只应写盘一次，实际 %d 次", writer.addCalls)
	}
	if report.Imported != 3 {
		t.Fatalf("应写入 3 条，实际 %d", report.Imported)
	}
	if len(report.Groups) != 1 || report.Groups[0] != "运维" {
		t.Fatalf("分组应为 [运维]，实际 %v", report.Groups)
	}
	for _, item := range writer.added {
		if item.Group != "运维" {
			t.Fatalf("写入分组错误: %+v", item)
		}
	}
}

// 未指派分组的集合回落到 defaultGroup；指派为空白的同样回落。
func TestApplyQuickCommands_FallsBackToDefaultGroup(t *testing.T) {
	sets := []*QuickButtonSet{
		buttonSet("a.qbl", "a", cmd("n1", "c1")),
		buttonSet("b.qbl", "b", cmd("n2", "c2")),
	}
	writer := &fakeCommandWriter{}
	report, err := ApplyQuickCommands(sets, map[string]string{"b.qbl": "   "}, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if report.Imported != 2 {
		t.Fatalf("应写入 2 条，实际 %d", report.Imported)
	}
	for _, item := range writer.added {
		if item.Group != "Xshell" {
			t.Fatalf("应回落到 Xshell，实际 %+v", item)
		}
	}
}

func TestApplyQuickCommands_SkipsUnsupportedAndExisting(t *testing.T) {
	sets := []*QuickButtonSet{
		buttonSet("a.qbl", "a",
			cmd("tail", "tail -f app.log"),
			QuickButton{Name: "脚本", Content: "echo s", Type: "3"},
			QuickButton{Name: "空", Content: "", Type: "1"},
		),
		buttonSet("b.qbl", "b",
			cmd("tail", "tail -f app.log"), // 与 a 集合重复
		),
	}
	writer := &fakeCommandWriter{
		existing: []ImportedCommand{{Group: "Xshell", Name: "tail", Content: "tail -f app.log"}},
	}

	report, err := ApplyQuickCommands(sets, nil, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	// 本地已存在 tail，因此 b 集合里的 tail 也判为已存在
	if report.Imported != 0 {
		t.Fatalf("全部命中重复/不支持时不应写入，实际 %d", report.Imported)
	}
	if report.SkippedExisting != 2 {
		t.Fatalf("重复跳过应为 2，实际 %d", report.SkippedExisting)
	}
	if report.SkippedUnsupported != 2 {
		t.Fatalf("不支持跳过应为 2，实际 %d", report.SkippedUnsupported)
	}
	if writer.addCalls != 0 {
		t.Fatalf("没有可写入项时不应调用写盘，实际 %d 次", writer.addCalls)
	}
	if len(report.Groups) != 0 || report.Groups == nil {
		t.Fatalf("没有写入时分组应为空集合，实际 %v", report.Groups)
	}
}

// 写盘失败必须把错误抛给调用方，不能静默当成成功。
func TestApplyQuickCommands_PropagatesWriteError(t *testing.T) {
	sets := []*QuickButtonSet{buttonSet("a.qbl", "a", cmd("n", "c"))}
	writer := &fakeCommandWriter{failOnAdd: true}

	if _, err := ApplyQuickCommands(sets, nil, writer, "Xshell"); err == nil {
		t.Fatal("写盘失败时应返回错误")
	}
}

func TestApplyQuickCommands_RejectsNilWriter(t *testing.T) {
	if _, err := ApplyQuickCommands(nil, nil, nil, "Xshell"); err == nil {
		t.Fatal("缺少写入端口时应返回错误")
	}
}

// 分析跳过的数量应与执行结果一致（同一条输入下），这是"预览可信"的前提。
func TestAnalyzeAndApplyAgree(t *testing.T) {
	sets := []*QuickButtonSet{
		buttonSet("a.qbl", "a",
			cmd("tail", "tail -f app.log"),
			cmd("df", "df -h"),
			QuickButton{Name: "脚本", Content: "echo s", Type: "2"},
		),
		buttonSet("b.qbl", "b", cmd("tail", "tail -f app.log")),
	}
	existing := []ImportedCommand{{Group: "Xshell", Name: "已有", Content: "whoami"}}

	plan := AnalyzeQuickCommands(sets, &fakeCommandWriter{existing: existing}, "Xshell")
	writer := &fakeCommandWriter{existing: existing}
	report, err := ApplyQuickCommands(sets, nil, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}

	if plan.Importable != report.Imported {
		t.Fatalf("预览可导入 %d 与实际导入 %d 不一致", plan.Importable, report.Imported)
	}
	if plan.Existing != report.SkippedExisting {
		t.Fatalf("预览重复 %d 与实际跳过 %d 不一致", plan.Existing, report.SkippedExisting)
	}
	if plan.Unsupported != report.SkippedUnsupported {
		t.Fatalf("预览不支持 %d 与实际跳过 %d 不一致", plan.Unsupported, report.SkippedUnsupported)
	}
}

// 集合自带的警告（例如"该文件里没有可识别的快捷按钮"）必须一路带到预览与报告。
func TestWarningsFlowThrough(t *testing.T) {
	set := buttonSet("a.qbl", "a", cmd("n", "c"))
	set.Warnings = []string{"该文件里没有可识别的快捷按钮"}

	plan := AnalyzeQuickCommands([]*QuickButtonSet{set}, &fakeCommandWriter{}, "Xshell")
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "没有可识别") {
		t.Fatalf("预览应带上集合的警告，实际 %v", plan.Warnings)
	}

	report, err := ApplyQuickCommands([]*QuickButtonSet{set}, nil, &fakeCommandWriter{}, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "没有可识别") {
		t.Fatalf("报告应带上集合的警告，实际 %v", report.Warnings)
	}
}

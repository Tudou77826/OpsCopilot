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

// allItems 把集合里所有可导入项都选上，分组跟随集合——等价于"全都要"。
func allItems(set *QuickButtonSet) SetSelection {
	sel := SetSelection{}
	for _, b := range set.Buttons {
		if ok, _ := ButtonSupported(b); ok {
			sel.Items = append(sel.Items, SelectedCommand{Name: b.Name, Content: b.Content})
		}
	}
	return sel
}

func selectionsFor(sets ...*QuickButtonSet) map[string]SetSelection {
	out := make(map[string]SetSelection, len(sets))
	for _, set := range sets {
		out[set.Path] = allItems(set)
	}
	return out
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
	// 每行的按钮数必须是该集合自己的条数，不是总数——实机上这一项漏填过，
	// 界面因此显示成"可导入 1 / 共 0 条"。
	if plan.Rows[0].Buttons != 4 || plan.Rows[1].Buttons != 2 {
		t.Fatalf("行内按钮数错误: %+v", plan.Rows)
	}
	// 行内三个桶相加也必须等于该行按钮数
	for i, row := range plan.Rows {
		if row.Importable+row.Unsupported+row.Existing != row.Buttons {
			t.Fatalf("第 %d 行数字不自洽: %+v", i+1, row)
		}
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

// 预览必须给出逐条明细（含不可导入的条目），界面才能把命令列出来、把不支持的置灰。
func TestAnalyzeQuickCommands_EmitsItemDetails(t *testing.T) {
	set := buttonSet("a.qbl", "a",
		cmd("tail", "tail -f app.log"),
		QuickButton{Name: "脚本", Content: "echo s", Type: "3"},
	)
	plan := AnalyzeQuickCommands([]*QuickButtonSet{set}, &fakeCommandWriter{}, "Xshell")

	items := plan.Rows[0].Items
	if len(items) != 2 {
		t.Fatalf("应给出 2 条明细（含不可导入项），实际 %d: %+v", len(items), items)
	}
	// 顺序与文件一致
	if items[0].Name != "tail" || items[0].Content != "tail -f app.log" || !items[0].Supported {
		t.Fatalf("第 1 条明细错误: %+v", items[0])
	}
	if items[1].Name != "脚本" || items[1].Supported || items[1].Type != "3" {
		t.Fatalf("第 2 条明细应标记为不可导入并保留原始类型: %+v", items[1])
	}
	if !strings.Contains(items[1].SkipReason, "类型为 3") {
		t.Fatalf("不可导入项应带原因: %q", items[1].SkipReason)
	}
}

// 明细里的 Existing 标记要与行内计数一致。
func TestAnalyzeQuickCommands_ItemExistingFlag(t *testing.T) {
	set := buttonSet("a.qbl", "a", cmd("tail", "tail -f app.log"), cmd("df", "df -h"))
	writer := &fakeCommandWriter{existing: []ImportedCommand{{Group: "Xshell", Name: "tail", Content: "tail -f app.log"}}}

	plan := AnalyzeQuickCommands([]*QuickButtonSet{set}, writer, "Xshell")
	items := plan.Rows[0].Items
	if items[0].Existing != true || items[1].Existing != false {
		t.Fatalf("已存在标记错误: %+v", items)
	}
	if plan.Rows[0].Existing != 1 {
		t.Fatalf("行内已存在数应与明细一致，实际 %d", plan.Rows[0].Existing)
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

// 集合自己也要给非 nil 的明细切片（没有按钮时是空数组）。
func TestAnalyzeQuickCommands_ItemSliceNeverNil(t *testing.T) {
	set := buttonSet("empty.qbl", "empty")
	plan := AnalyzeQuickCommands([]*QuickButtonSet{set}, &fakeCommandWriter{}, "Xshell")
	if plan.Rows[0].Items == nil {
		t.Fatal("没有按钮时明细应为空数组而不是 nil")
	}
}

func TestApplyQuickCommands_AssignsGroupsAndWritesOnce(t *testing.T) {
	first := buttonSet(`C:\q\commands.qbl`, "commands", cmd("tail", "tail -f app.log"), cmd("df", "df -h"))
	second := buttonSet(`C:\q\ops.qbl`, "ops", cmd("free", "free -h"))
	writer := &fakeCommandWriter{}

	report, err := ApplyQuickCommands(
		[]*QuickButtonSet{first, second},
		map[string]SetSelection{
			`C:\q\commands.qbl`: {Group: "运维", Items: allItems(first).Items},
			`C:\q\ops.qbl`:      {Group: "运维", Items: allItems(second).Items},
		},
		writer, "Xshell",
	)
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

// 逐条指定分组：同一集合可以拆到多个分组，未指定的条目跟随集合分组。
func TestApplyQuickCommands_PerItemGroupOverride(t *testing.T) {
	set := buttonSet("a.qbl", "a", cmd("tail", "tail -f app.log"), cmd("df", "df -h"), cmd("free", "free -h"))
	writer := &fakeCommandWriter{}

	report, err := ApplyQuickCommands([]*QuickButtonSet{set}, map[string]SetSelection{
		"a.qbl": {
			Group: "集合默认",
			Items: []SelectedCommand{
				{Name: "tail", Content: "tail -f app.log"},            // 跟随集合
				{Name: "df", Content: "df -h", Group: "磁盘巡检"},         // 覆盖
				{Name: "free", Content: "free -h", Group: "  磁盘巡检  "}, // 覆盖（去空白）
			},
		},
	}, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}

	got := map[string]string{}
	for _, item := range writer.added {
		got[item.Name] = item.Group
	}
	if got["tail"] != "集合默认" {
		t.Fatalf("未指定分组的条目应跟随集合分组，实际 %q", got["tail"])
	}
	if got["df"] != "磁盘巡检" || got["free"] != "磁盘巡检" {
		t.Fatalf("逐条分组覆盖未生效: %+v", got)
	}
	// 实际写入涉及两个分组，顺序按首次出现
	if len(report.Groups) != 2 || report.Groups[0] != "集合默认" || report.Groups[1] != "磁盘巡检" {
		t.Fatalf("写入分组列表错误: %v", report.Groups)
	}
}

// 逐条改名与改内容：以回传的条目为准，不再回到 .qbl 推导。
func TestApplyQuickCommands_AppliesItemEdits(t *testing.T) {
	set := buttonSet("a.qbl", "a", cmd("tail", "tail -f old.log"))
	writer := &fakeCommandWriter{}

	_, err := ApplyQuickCommands([]*QuickButtonSet{set}, map[string]SetSelection{
		"a.qbl": {Group: "Xshell", Items: []SelectedCommand{{Name: "跟踪日志", Content: "tail -f new.log"}}},
	}, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(writer.added) != 1 {
		t.Fatalf("应写入 1 条，实际 %d", len(writer.added))
	}
	got := writer.added[0]
	if got.Name != "跟踪日志" || got.Content != "tail -f new.log" {
		t.Fatalf("界面上的改名/改内容未生效: %+v", got)
	}
}

// 只导入勾选的条目：没勾的不该出现。
func TestApplyQuickCommands_ImportsOnlySelectedItems(t *testing.T) {
	set := buttonSet("a.qbl", "a", cmd("tail", "tail -f app.log"), cmd("df", "df -h"), cmd("free", "free -h"))
	writer := &fakeCommandWriter{}

	report, err := ApplyQuickCommands([]*QuickButtonSet{set}, map[string]SetSelection{
		"a.qbl": {Group: "Xshell", Items: []SelectedCommand{{Name: "df", Content: "df -h"}}},
	}, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if report.Imported != 1 || len(writer.added) != 1 || writer.added[0].Name != "df" {
		t.Fatalf("应只导入勾选的 1 条，实际 %+v", writer.added)
	}
}

// 用户取消了整个集合 / 条目列表为空：不写入任何东西，也不调用写盘。
func TestApplyQuickCommands_ExcludedSetWritesNothing(t *testing.T) {
	set := buttonSet("a.qbl", "a", cmd("tail", "tail -f app.log"))
	writer := &fakeCommandWriter{}

	report, err := ApplyQuickCommands([]*QuickButtonSet{set}, map[string]SetSelection{
		"a.qbl": {Group: "Xshell", Items: nil},
	}, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if report.Imported != 0 || writer.addCalls != 0 {
		t.Fatalf("取消的集合不应写盘: imported=%d calls=%d", report.Imported, writer.addCalls)
	}
	if report.Groups == nil || len(report.Groups) != 0 {
		t.Fatalf("没有写入时分组应为空集合，实际 %v", report.Groups)
	}
}

// selections 里没有出现该集合（用户没勾）时不导入。
func TestApplyQuickCommands_MissingSelectionIsSkipped(t *testing.T) {
	set := buttonSet("a.qbl", "a", cmd("tail", "tail -f app.log"))
	writer := &fakeCommandWriter{}

	report, err := ApplyQuickCommands([]*QuickButtonSet{set}, map[string]SetSelection{}, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if report.Imported != 0 || writer.addCalls != 0 {
		t.Fatalf("未勾选的集合不应写入，实际 imported=%d", report.Imported)
	}
}

// 未指派分组的集合回落到 defaultGroup；指派为空白的同样回落。
func TestApplyQuickCommands_FallsBackToDefaultGroup(t *testing.T) {
	first := buttonSet("a.qbl", "a", cmd("n1", "c1"))
	second := buttonSet("b.qbl", "b", cmd("n2", "c2"))
	writer := &fakeCommandWriter{}

	report, err := ApplyQuickCommands(
		[]*QuickButtonSet{first, second},
		map[string]SetSelection{
			"a.qbl": allItems(first),
			"b.qbl": {Group: "   ", Items: allItems(second).Items},
		},
		writer, "Xshell",
	)
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

func TestApplyQuickCommands_SkipsExistingAndInvalid(t *testing.T) {
	set := buttonSet("a.qbl", "a", cmd("tail", "tail -f app.log"), QuickButton{Name: "脚本", Content: "echo s", Type: "3"})
	writer := &fakeCommandWriter{
		existing: []ImportedCommand{{Group: "Xshell", Name: "tail", Content: "tail -f app.log"}},
	}

	report, err := ApplyQuickCommands([]*QuickButtonSet{set}, map[string]SetSelection{
		"a.qbl": {
			Group: "Xshell",
			Items: []SelectedCommand{
				{Name: "tail", Content: "tail -f app.log"}, // 已存在
				{Name: "脚本", Content: "echo s"},            // 该集合里本就是不可导入类型
				{Name: "  ", Content: "echo x"},            // 缺名称
				{Name: "ok", Content: ""},                  // 缺内容
			},
		},
	}, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if report.Imported != 0 {
		t.Fatalf("全部应被跳过，实际写入 %d", report.Imported)
	}
	if report.SkippedExisting != 1 {
		t.Fatalf("已存在跳过应为 1，实际 %d", report.SkippedExisting)
	}
	if report.SkippedUnsupported != 3 {
		t.Fatalf("不可导入跳过应为 3（不支持类型 + 缺名称 + 缺内容），实际 %d", report.SkippedUnsupported)
	}
	if writer.addCalls != 0 {
		t.Fatalf("没有可写入项时不应调用写盘，实际 %d 次", writer.addCalls)
	}
}

// 同一批内重复只保留一条（含改名后撞车的情况）。
func TestApplyQuickCommands_DedupesWithinBatch(t *testing.T) {
	first := buttonSet("a.qbl", "a", cmd("tail", "tail -f app.log"))
	second := buttonSet("b.qbl", "b", cmd("tail", "tail -f app.log"))
	writer := &fakeCommandWriter{}

	report, err := ApplyQuickCommands(
		[]*QuickButtonSet{first, second},
		selectionsFor(first, second),
		writer, "Xshell",
	)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if report.Imported != 1 || report.SkippedExisting != 1 {
		t.Fatalf("批内重复应只保留一条: %+v", report)
	}
}

// 写盘失败必须把错误抛给调用方，不能静默当成成功。
func TestApplyQuickCommands_PropagatesWriteError(t *testing.T) {
	set := buttonSet("a.qbl", "a", cmd("n", "c"))
	writer := &fakeCommandWriter{failOnAdd: true}

	if _, err := ApplyQuickCommands([]*QuickButtonSet{set}, selectionsFor(set), writer, "Xshell"); err == nil {
		t.Fatal("写盘失败时应返回错误")
	}
}

func TestApplyQuickCommands_RejectsNilWriter(t *testing.T) {
	if _, err := ApplyQuickCommands(nil, nil, nil, "Xshell"); err == nil {
		t.Fatal("缺少写入端口时应返回错误")
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

	report, err := ApplyQuickCommands([]*QuickButtonSet{set}, selectionsFor(set), &fakeCommandWriter{}, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "没有可识别") {
		t.Fatalf("报告应带上集合的警告，实际 %v", report.Warnings)
	}
}

// 分析与执行在"照预览全选"时结果必须一致，这是预览可信的前提。
func TestAnalyzeAndApplyAgree(t *testing.T) {
	set := buttonSet("a.qbl", "a",
		cmd("tail", "tail -f app.log"),
		cmd("df", "df -h"),
		QuickButton{Name: "脚本", Content: "echo s", Type: "2"},
	)
	dup := buttonSet("b.qbl", "b", cmd("tail", "tail -f app.log"))
	sets := []*QuickButtonSet{set, dup}
	existing := []ImportedCommand{{Group: "Xshell", Name: "已有", Content: "whoami"}}

	plan := AnalyzeQuickCommands(sets, &fakeCommandWriter{existing: existing}, "Xshell")
	writer := &fakeCommandWriter{existing: existing}
	report, err := ApplyQuickCommands(sets, selectionsFor(set, dup), writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}

	if plan.Importable != report.Imported {
		t.Fatalf("预览可导入 %d 与实际导入 %d 不一致", plan.Importable, report.Imported)
	}
	if plan.Existing != report.SkippedExisting {
		t.Fatalf("预览重复 %d 与实际跳过 %d 不一致", plan.Existing, report.SkippedExisting)
	}
	// 预览的"不支持"由类型判定得出；执行时这些条目根本不会出现在选择里，
	// 因此这里只要求执行侧不把它们写进去。
	if plan.Unsupported != 1 {
		t.Fatalf("预览应识别出 1 条不支持，实际 %d", plan.Unsupported)
	}
	if report.Imported != 2 {
		t.Fatalf("应导入 tail 与 df 两条，实际 %d", report.Imported)
	}
}

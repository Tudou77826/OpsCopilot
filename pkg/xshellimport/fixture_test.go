package xshellimport

import (
	"path/filepath"
	"strings"
	"testing"
)

// 场景夹具：testdata/quickbuttons 是一套刻意做得复杂的 Xshell 快捷按钮导出目录，
// 用一个目录覆盖真实使用里会遇到的多数形态。它的价值在于"一起出现"——单看每个
// 特性都有专门的用例，但编码混排、缺 Count、空集合、干扰文件、超长命令同时在场时
// 才谈得上与实机接近。
//
//	common.qbl       UTF-16LE，含 Type=2/3、缺名称、超长命令、含 = 的值
//	docker.qbl       与现有「容器命令」内容重合，用来触发落点参考现状
//	k8s.qbl          含超长中文名
//	legacy-gbk.qbl   GBK 无 BOM 且 [Info] 缺 Count（走键名扫描兜底）
//	batch-30.qbl     30 条，用来试长列表
//	empty.qbl        没有按钮（文件级告警）
//	trigger.ini / readme.txt  非 .qbl，必须被忽略
func quickButtonFixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "quickbuttons"))
	if err != nil {
		t.Fatalf("解析夹具目录失败: %v", err)
	}
	return dir
}

func TestFixtureDirectoryParses(t *testing.T) {
	sets, warnings, err := ParseQuickButtonPath(quickButtonFixtureDir(t))
	if err != nil {
		t.Fatalf("解析夹具目录失败: %v", err)
	}

	// 6 个 .qbl（非 .qbl 文件被忽略）
	if len(sets) != 6 {
		names := make([]string, 0, len(sets))
		for _, s := range sets {
			names = append(names, s.Name)
		}
		t.Fatalf("应解析出 6 套按钮，实际 %d: %v", len(sets), names)
	}
	// 路径级警告为空时可以是 nil：叶子包允许返回 nil，跨到 Wails 边界前由
	// 合并函数统一归一为空切片（见 app_quickcmdimport.go 的 mergeWarnings）。
	// 这个夹具的告警发生在文件级（empty.qbl），下面单独断言。
	_ = warnings

	byName := map[string]*QuickButtonSet{}
	total := 0
	for _, set := range sets {
		byName[set.Name] = set
		total += len(set.Buttons)
	}
	// 12 + 8 + 10 + 5 + 30 + 0
	if total != 65 {
		t.Fatalf("按钮总数应为 65，实际 %d", total)
	}

	// 非 .qbl 干扰文件不能被当成按钮集
	for _, unwanted := range []string{"trigger", "readme"} {
		if _, ok := byName[unwanted]; ok {
			t.Fatalf("非 .qbl 文件被当成按钮集: %s", unwanted)
		}
	}

	// 没有按钮的文件要给出文件级告警，而不是静默成功
	if len(byName["empty"].Buttons) != 0 {
		t.Fatalf("empty.qbl 不应有按钮，实际 %d", len(byName["empty"].Buttons))
	}
	if len(byName["empty"].Warnings) == 0 {
		t.Fatal("没有按钮的文件应带文件级告警")
	}
}

// GBK 且缺 Count：中文要正确解码，按钮数要靠键名扫描兜出来。
func TestFixtureLegacyGBKAndMissingCount(t *testing.T) {
	sets, _, err := ParseQuickButtonPath(quickButtonFixtureDir(t))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	var legacy *QuickButtonSet
	for _, set := range sets {
		if set.Name == "legacy-gbk" {
			legacy = set
		}
	}
	if legacy == nil {
		t.Fatal("没找到 legacy-gbk.qbl")
	}
	if len(legacy.Buttons) != 5 {
		t.Fatalf("[Info] 缺 Count 时应扫描键名兜出 5 个按钮，实际 %d", len(legacy.Buttons))
	}
	if legacy.Buttons[0].Name != "重启 nginx" {
		t.Fatalf("GBK 中文名称解码错误: %q", legacy.Buttons[0].Name)
	}
	if legacy.Buttons[4].Content != "echo a; echo b" {
		t.Fatalf("含分号的值被截断: %q", legacy.Buttons[4].Content)
	}
}

// 超长命令与含 '=' 的值都要完整保留。
func TestFixtureLongAndSpecialValues(t *testing.T) {
	sets, _, err := ParseQuickButtonPath(quickButtonFixtureDir(t))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	var common *QuickButtonSet
	for _, set := range sets {
		if set.Name == "common" {
			common = set
		}
	}
	if common == nil {
		t.Fatal("没找到 common.qbl")
	}

	longest := ""
	awk := ""
	for _, b := range common.Buttons {
		if len(b.Content) > len(longest) {
			longest = b.Content
		}
		if b.Name == "分析列 = 分割" {
			awk = b.Content
		}
	}
	if !strings.Contains(longest, "journalctl -u app") || !strings.Contains(longest, "tail -n 200") {
		t.Fatalf("超长命令未完整保留: %q", longest)
	}
	// 值里含 '='：只能按第一个 '=' 切分键值
	if awk != "awk -F= '{print $2}' /etc/os-release" {
		t.Fatalf("含 = 的值解析错误: %q", awk)
	}
}

// 整套夹具在本机命令库下的分析结果：可导入 / 不支持 / 已存在各自的数量与归类。
func TestFixtureAnalyzeAgainstLibrary(t *testing.T) {
	sets, _, err := ParseQuickButtonPath(quickButtonFixtureDir(t))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	// 模拟 OpsCopilot 现有命令：与被导入内容刻意重合的几条
	library := []ImportedCommand{
		{Group: "默认", Name: "磁盘占用", Content: "df -h"},
		{Group: "默认", Name: "内存", Content: "free -h"},
		{Group: "容器命令", Name: "容器列表", Content: "docker ps -a"},
		{Group: "K8s 操作", Name: "Pods", Content: "kubectl get pods -A"},
		{Group: "Nginx 维护", Name: "重载配置", Content: "nginx -s reload"},
		{Group: "Xshell", Name: "tail", Content: "tail -f /var/log/$app/app.log"},
	}
	plan := AnalyzeQuickCommands(sets, &fakeCommandWriter{existing: library}, "Xshell")

	if plan.Sets != 6 || plan.Buttons != 65 {
		t.Fatalf("集合/按钮数错误: sets=%d buttons=%d", plan.Sets, plan.Buttons)
	}
	// 不支持的四类：Type=2、Type=3、缺名称、缺内容
	if plan.Unsupported != 4 {
		t.Fatalf("应识别出 4 条不可导入，实际 %d", plan.Unsupported)
	}
	// 三个桶必须相加等于总数，界面上的数字才自洽
	if plan.Importable+plan.Unsupported+plan.Existing != plan.Buttons {
		t.Fatalf("可导入+不支持+已存在 应等于 %d: %d+%d+%d",
			plan.Buttons, plan.Importable, plan.Unsupported, plan.Existing)
	}

	// 已存在 = 2：命令库一条都没命中，这 2 条是**批内重复**——`容器列表` 与 `进容器`
	// 同时出现在 common.qbl 与 docker.qbl 里。批内重复与本地已存在下场相同（都跳过），
	// 因此合并计数，这也正是夹具要有"两套按钮内容重叠"的原因。
	if plan.Existing != 2 {
		t.Fatalf("批内重复应计为 2 条已存在，实际 %d", plan.Existing)
	}
	// 可导入 = 65 - 4（不可导入） - 2（批内重复）
	if plan.Importable != 59 {
		t.Fatalf("可导入应为 59，实际 %d", plan.Importable)
	}

	// 逐条明细必须覆盖全部 65 条（不可导入的也在内），界面才能置灰并说明原因
	items := 0
	for _, row := range plan.Rows {
		if row.Items == nil {
			t.Fatalf("%s 的明细不应为 nil", row.Name)
		}
		items += len(row.Items)
		if row.Buttons != len(row.Items) {
			t.Fatalf("%s 的按钮数与明细数不一致: %d vs %d", row.Name, row.Buttons, len(row.Items))
		}
	}
	if items != 65 {
		t.Fatalf("明细总数应为 65，实际 %d", items)
	}
}

// 全选导入时：整批只写盘一次，写入条数等于可导入数。
func TestFixtureApplyWritesOnce(t *testing.T) {
	sets, _, err := ParseQuickButtonPath(quickButtonFixtureDir(t))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	writer := &fakeCommandWriter{}
	selections := map[string]SetSelection{}
	for _, set := range sets {
		selections[set.Path] = allItems(set)
	}

	report, err := ApplyQuickCommands(sets, selections, writer, "Xshell")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if writer.addCalls != 1 {
		t.Fatalf("整批只应写盘一次，实际 %d 次", writer.addCalls)
	}
	// 65 条里：4 条不可导入、2 条批内重复，其余 59 条写入
	if report.Imported != 59 {
		t.Fatalf("65 条里应写入 59 条，实际 %d", report.Imported)
	}
	// batch-30 的 30 条 + 其余可导入项，都落在同一个默认分组
	if len(report.Groups) != 1 || report.Groups[0] != "Xshell" {
		t.Fatalf("默认分组应为 Xshell，实际 %v", report.Groups)
	}
}

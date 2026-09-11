package xshellimport

import (
	"fmt"
	"sort"
	"strings"
)

// CommandWriter 是导入器写入快捷命令所需的最小能力，由宿主实现（桌面端用
// pkg/config 适配）。用端口而不是直接依赖 pkg/config，是为了让本包保持可独立测试的
// 纯叶子包，也便于 sidecar 等其它宿主复用同一套导入逻辑。
type CommandWriter interface {
	// ExistingCommands 返回当前已有的全部快捷命令，用于判定哪些属于重复导入。
	ExistingCommands() []ImportedCommand
	// AddCommands 批量追加，实现方负责生成 ID 并只写盘一次，返回实际写入条数。
	AddCommands(items []ImportedCommand) (int, error)
}

// ImportedCommand 是一条待写入的快捷命令。
type ImportedCommand struct {
	Group   string
	Name    string
	Content string
}

// QuickCommandItem 是预览里的一条命令明细。
//
// 不支持导入的条目同样出现在列表里（界面置灰并附原因），而不是在解析层被静默丢掉：
// 让用户看到"这一条不会导入、因为什么"比只给一个跳过的总数清楚得多。
type QuickCommandItem struct {
	Name       string
	Content    string
	Type       string
	Supported  bool
	SkipReason string
	// Existing 表示按所属集合的默认分组判断，这一条已经存在（导入时会被跳过）。
	// 用户改了分组名之后实际结果可能不同，最终以报告为准。
	Existing bool
}

// QuickCommandPlan 是导入前的只读分析结果。
//
// Importable + Unsupported + Existing 恒等于 Buttons，界面上的数字因此可以互相校验。
// Existing 同时包含"本地已有"和"本次其它集合里已出现过"的重复项——两者的下场一样
// （都会被跳过），分开统计只会让界面上多一个无人看的数字。
type QuickCommandPlan struct {
	Sets        int
	Buttons     int
	Importable  int
	Unsupported int
	Existing    int
	// Groups 是本次将涉及的目标分组（按首次出现顺序去重）。
	Groups []string
	Rows   []QuickCommandSetRow
	// Warnings 是解析与跳过原因的完整列表，已排序。
	Warnings []string
}

// QuickCommandSetRow 描述一个 .qbl 集合的分析结果，也是界面上"每集合一段"的数据来源。
type QuickCommandSetRow struct {
	// Source 是 .qbl 文件路径，执行导入时用它回指该集合。
	Source      string
	Name        string
	Buttons     int
	Importable  int
	Unsupported int
	Existing    int
	// Group 是按 defaultGroup 推导出的建议分组名。界面可改，改动通过 Apply 的
	// selections 传回。
	Group string
	// Items 是该集合里逐条命令的明细（含不支持导入的条目）。
	Items []QuickCommandItem
}

// QuickCommandReport 是导入结果。
type QuickCommandReport struct {
	Imported           int
	SkippedExisting    int
	SkippedUnsupported int
	// Groups 是本次实际写入涉及的分组（去重，首次出现顺序）。
	Groups   []string
	Warnings []string
}

// SelectedCommand 是用户在预览里勾选并（可选地）编辑过的一条命令。
//
// 名称、内容都可能是改过的，因此导入以它为准，不再回到 .qbl 重新推导。
type SelectedCommand struct {
	Name    string
	Content string
	// Group 非空时覆盖所属集合的默认分组。
	Group string
}

// SetSelection 是用户对某一个 .qbl 集合的最终决定。
//
// Items 为空表示这个集合不导入任何命令（用户取消了它，或把里面的条目都取消了）。
// 没有出现在 selections 里的集合同样不导入。
type SetSelection struct {
	Group string
	Items []SelectedCommand
}

// AnalyzeQuickCommands 统计导入将会产生的结果，不写入任何数据。
//
// defaultGroup 是各集合的默认落点（界面上默认给 Xshell）。"已存在"按它计算，因此
// 用户在界面上改了分组名之后实际跳过数可能不同——最终以 Apply 的报告为准。
func AnalyzeQuickCommands(sets []*QuickButtonSet, writer CommandWriter, defaultGroup string) *QuickCommandPlan {
	plan := &QuickCommandPlan{
		Groups:   []string{},
		Rows:     []QuickCommandSetRow{},
		Warnings: []string{},
	}
	defaultGroup = resolveDefaultGroup(defaultGroup)

	seen := existingCommandKeys(writer)

	for _, set := range sets {
		plan.Sets++
		plan.Warnings = append(plan.Warnings, set.Warnings...)

		row := QuickCommandSetRow{
			Source:  set.Path,
			Name:    set.Name,
			Buttons: len(set.Buttons),
			Group:   defaultGroup,
			Items:   make([]QuickCommandItem, 0, len(set.Buttons)),
		}

		for _, button := range set.Buttons {
			plan.Buttons++

			item := QuickCommandItem{
				Name:    button.Name,
				Content: button.Content,
				Type:    button.Type,
			}
			if ok, reason := ButtonSupported(button); !ok {
				item.Supported = false
				item.SkipReason = reason
				plan.Unsupported++
				row.Unsupported++
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: %s", set.Name, reason))
				row.Items = append(row.Items, item)
				continue
			}

			item.Supported = true
			key := commandKey(row.Group, button.Name, button.Content)
			if seen[key] {
				item.Existing = true
				plan.Existing++
				row.Existing++
				row.Items = append(row.Items, item)
				continue
			}
			seen[key] = true // 批内去重：同一按钮出现在多个集合里时只算一条
			plan.Importable++
			row.Importable++
			row.Items = append(row.Items, item)
		}

		plan.Rows = append(plan.Rows, row)
		if row.Importable > 0 {
			plan.Groups = appendGroup(plan.Groups, row.Group)
		}
	}

	sort.Strings(plan.Warnings)
	return plan
}

// ApplyQuickCommands 按用户在预览里的决定写入快捷命令库。
//
// 决定权完全在调用方：名称、内容、目标分组都可以在界面上改过，因此这里以 selections
// 为准。仍然校验的只有"落盘时会不会出问题"这一层——名称为空、内容为空、以及该集合里
// 本来就是不可导入类型的同名条目（兜底，防止前端传错）。
//
// 整批只调用一次 writer.AddCommands，因此只写盘一次——导入几百条命令不会产生几百次
// 落盘，中途失败也不会留下写了一半的命令库。
func ApplyQuickCommands(sets []*QuickButtonSet, selections map[string]SetSelection, writer CommandWriter, defaultGroup string) (*QuickCommandReport, error) {
	if writer == nil {
		return nil, fmt.Errorf("缺少快捷命令写入端口")
	}
	defaultGroup = resolveDefaultGroup(defaultGroup)

	report := &QuickCommandReport{Groups: []string{}, Warnings: []string{}}
	seen := existingCommandKeys(writer)

	// 初始化为空切片而非 nil：这些字段会一路传到 Wails 边界和前端，
	// nil 会被序列化成 null，而前端按数组处理就会抛异常。
	var pending []ImportedCommand

	for _, set := range sets {
		report.Warnings = append(report.Warnings, set.Warnings...)

		selection, ok := selections[set.Path]
		if !ok {
			continue // 用户没有勾选这个集合
		}

		setGroup := fallbackGroup(selection.Group)
		if setGroup == "" {
			setGroup = defaultGroup
		}
		unsupported := unsupportedButtonNames(set)

		for _, item := range selection.Items {
			name := strings.TrimSpace(item.Name)
			content := strings.TrimSpace(item.Content)
			if name == "" || content == "" {
				report.SkippedUnsupported++
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s: 有一条命令缺少名称或内容，已跳过", set.Name))
				continue
			}
			if unsupported[name] {
				report.SkippedUnsupported++
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s: %s 的类型不受支持，已跳过", set.Name, name))
				continue
			}

			group := fallbackGroup(item.Group)
			if group == "" {
				group = setGroup
			}
			key := commandKey(group, name, content)
			if seen[key] {
				report.SkippedExisting++
				continue
			}
			seen[key] = true
			pending = append(pending, ImportedCommand{Group: group, Name: name, Content: content})
		}
	}

	if len(pending) > 0 {
		added, err := writer.AddCommands(pending)
		if err != nil {
			return nil, err
		}
		report.Imported = added
		report.Groups = pendingGroups(pending)
	}

	sort.Strings(report.Warnings)
	return report, nil
}

// unsupportedButtonNames 返回该集合里本来就不可导入的按钮名。
//
// 这些条目在界面上不可勾选；这里再拦一次是为了兜底，避免前端传错时把类型不明的
// 脚本/菜单类按钮当成普通命令写进命令库。
func unsupportedButtonNames(set *QuickButtonSet) map[string]bool {
	out := make(map[string]bool)
	for _, button := range set.Buttons {
		if ok, _ := ButtonSupported(button); !ok {
			out[button.Name] = true
		}
	}
	return out
}

// pendingGroups 按首次出现顺序去重，给出本次实际写入涉及的分组。
func pendingGroups(items []ImportedCommand) []string {
	groups := []string{}
	for _, item := range items {
		groups = appendGroup(groups, item.Group)
	}
	return groups
}

// existingCommandKeys 把已有命令摊成判重键集合。
func existingCommandKeys(writer CommandWriter) map[string]bool {
	seen := make(map[string]bool)
	if writer == nil {
		return seen
	}
	for _, c := range writer.ExistingCommands() {
		seen[commandKey(fallbackGroup(c.Group), c.Name, c.Content)] = true
	}
	return seen
}

// commandKey 是判重键：分组 + 名称 + 内容。
//
// 与 pkg/config 的 quickCommandKey 保持同一格式（分组、名称、内容三者都相同才算重复，
// 任一项不同都算不同的命令）。两处必须一致：本包的键只用于"预览与去重"，真正落盘时
// pkg/config 会再判一次重，若格式漂移，预览会与实际结果不符。
func commandKey(group, name, content string) string {
	return group + "\x00" + name + "\x00" + content
}

// fallbackGroup 去除首尾空白；空分组返回空串，由调用方决定回落值。
func fallbackGroup(group string) string {
	return strings.TrimSpace(group)
}

// resolveDefaultGroup 归一默认分组名：去空白，空值落到 default。
func resolveDefaultGroup(group string) string {
	if g := fallbackGroup(group); g != "" {
		return g
	}
	return "default"
}

// appendGroup 按首次出现顺序去重追加分组名。
func appendGroup(groups []string, group string) []string {
	for _, g := range groups {
		if g == group {
			return groups
		}
	}
	return append(groups, group)
}

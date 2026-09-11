package main

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"opscopilot/pkg/config"
	"opscopilot/pkg/xshellimport"
)

// 本文件承载 Xshell 快捷命令（.qbl）导入的 App 层门面。
//
// 与 app_sessionimport.go 同一套路：独立子文件、只做 Wails 边界转换与原生对话框，
// 真正的解析与合并逻辑在 pkg/xshellimport。app.go 不新增领域逻辑。

// QuickCommandImportOptions 是导入入参（Wails 边界，驼峰命名）。
type QuickCommandImportOptions struct {
	// DefaultGroup 是未逐集合指定目标分组时的落点，界面上默认给 Xshell。
	DefaultGroup string `json:"defaultGroup,omitempty"`
}

// XshellQuickButtonDir 描述探测到的一个本机 Xshell 快捷按钮目录。
type XshellQuickButtonDir struct {
	Path    string `json:"path"`
	Version int    `json:"version"`
	Sets    int    `json:"sets"`
	Buttons int    `json:"buttons"`
}

// QuickCommandSetItem 是预览里的一条命令明细。
//
// 不可导入的条目同样出现在列表里（supported=false 且带 skipReason），界面据此置灰
// 并说明原因，而不是只给一个跳过的总数。
type QuickCommandSetItem struct {
	Name       string `json:"name"`
	Content    string `json:"content"`
	Type       string `json:"type"`
	Supported  bool   `json:"supported"`
	SkipReason string `json:"skipReason,omitempty"`
	Existing   bool   `json:"existing"`
}

// QuickCommandSetRow 是预览界面上"每集合一段"的数据。
type QuickCommandSetRow struct {
	// Source 是 .qbl 文件路径，执行导入时用它回指该集合。
	Source      string `json:"source"`
	Name        string `json:"name"`
	Buttons     int    `json:"buttons"`
	Importable  int    `json:"importable"`
	Unsupported int    `json:"unsupported"`
	Existing    int    `json:"existing"`
	// Group 是建议的目标分组名，界面上可改。
	Group string `json:"group"`
	// Items 是该集合逐条命令的明细，供界面列出并逐条勾选、改名、改分组。
	Items []QuickCommandSetItem `json:"items"`
}

// QuickCommandImportAnalysis 是写入前的影响分析。
type QuickCommandImportAnalysis struct {
	Sets        int                  `json:"sets"`
	Buttons     int                  `json:"buttons"`
	Importable  int                  `json:"importable"`
	Unsupported int                  `json:"unsupported"`
	Existing    int                  `json:"existing"`
	Groups      []string             `json:"groups"`
	Rows        []QuickCommandSetRow `json:"rows"`
	Warnings    []string             `json:"warnings"`
}

// QuickCommandImportItem 是回传的一条命令：名称、内容、目标分组都可能是用户在界面上
// 改过的，因此以它为准写入，不回到 .qbl 重新推导。
type QuickCommandImportItem struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	// Group 为空表示跟随所属集合的默认分组。
	Group string `json:"group,omitempty"`
}

// QuickCommandImportSelection 是回传的一个集合的最终决定。
//
// Items 为空表示这个集合不导入任何命令；整个集合没有出现在请求里同样不导入。
// 集合的默认分组是 Group，条目可用自己的 Group 覆盖。
type QuickCommandImportSelection struct {
	Source string                   `json:"source"`
	Group  string                   `json:"group"`
	Items  []QuickCommandImportItem `json:"items"`
}

// QuickCommandImportReport 是导入结果。
type QuickCommandImportReport struct {
	Imported           int      `json:"imported"`
	SkippedExisting    int      `json:"skippedExisting"`
	SkippedUnsupported int      `json:"skippedUnsupported"`
	Groups             []string `json:"groups"`
	Warnings           []string `json:"warnings"`
}

// quickCommandWriter 把 config.Manager 适配成 xshellimport 需要的写入端口。
//
// 这里直接读 Config.QuickCommands 而不加锁，与 LoadQuickCommands 的读法一致：
// 该视图只用于预览与统计（读到的陈旧值最多让报告里的跳过数差一两条），真正的判重
// 与写盘在 ImportQuickCommands 内部持锁完成。
type quickCommandWriter struct {
	mgr *config.Manager
}

func (w quickCommandWriter) ExistingCommands() []xshellimport.ImportedCommand {
	cmds := w.mgr.Config.QuickCommands
	out := make([]xshellimport.ImportedCommand, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, xshellimport.ImportedCommand{Group: c.Group, Name: c.Name, Content: c.Content})
	}
	return out
}

func (w quickCommandWriter) AddCommands(items []xshellimport.ImportedCommand) (int, error) {
	cmds := make([]config.QuickCommand, 0, len(items))
	for _, item := range items {
		cmds = append(cmds, config.QuickCommand{Name: item.Name, Content: item.Content, Group: item.Group})
	}
	return w.mgr.ImportQuickCommands(cmds)
}

// DetectXshellQuickButtonDirs 探测本机 Xshell 的快捷按钮目录。
//
// 与会话导入同理：同机场景下这是最省事的入口，用户不需要在 Xshell 里做导出操作。
// 目录里一个 .qbl 都没有时不会被列为来源。
func (a *App) DetectXshellQuickButtonDirs() ([]XshellQuickButtonDir, error) {
	dirs := xshellimport.DiscoverXshellQuickButtonDirs()
	out := make([]XshellQuickButtonDir, 0, len(dirs))
	for _, dir := range dirs {
		sets, _, err := xshellimport.ParseQuickButtonPath(dir)
		if err != nil {
			continue
		}
		buttons := 0
		for _, set := range sets {
			buttons += len(set.Buttons)
		}
		out = append(out, XshellQuickButtonDir{
			Path:    dir,
			Version: xshellDirVersion(dir),
			Sets:    len(sets),
			Buttons: buttons,
		})
	}
	return out, nil
}

// SelectQuickCommandImportFile 弹出系统文件选择框，返回选中的 .qbl；取消时返回空串。
func (a *App) SelectQuickCommandImportFile() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 Xshell 快捷按钮文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "Xshell 快捷按钮集 (*.qbl)", Pattern: "*.qbl"},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// SelectQuickCommandImportDirectory 弹出系统目录选择框；取消时返回空串。
func (a *App) SelectQuickCommandImportDirectory() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 Xshell 快捷按钮目录",
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// AnalyzeQuickCommandImport 只读解析并统计导入影响，不写入任何数据。
//
// 与 .xsh 导入一样拆成两阶段：用户先看清楚会导入多少、哪些会被跳过，再决定执行。
func (a *App) AnalyzeQuickCommandImport(path string, opts QuickCommandImportOptions) (*QuickCommandImportAnalysis, error) {
	sets, parseWarnings, err := parseQuickCommandImport(path)
	if err != nil {
		return nil, err
	}

	plan := xshellimport.AnalyzeQuickCommands(sets, quickCommandWriter{a.configMgr}, opts.DefaultGroup)

	rows := make([]QuickCommandSetRow, 0, len(plan.Rows))
	for _, row := range plan.Rows {
		items := make([]QuickCommandSetItem, 0, len(row.Items))
		for _, item := range row.Items {
			items = append(items, QuickCommandSetItem{
				Name:       item.Name,
				Content:    item.Content,
				Type:       item.Type,
				Supported:  item.Supported,
				SkipReason: item.SkipReason,
				Existing:   item.Existing,
			})
		}
		rows = append(rows, QuickCommandSetRow{
			Source:      row.Source,
			Name:        row.Name,
			Buttons:     row.Buttons,
			Importable:  row.Importable,
			Unsupported: row.Unsupported,
			Existing:    row.Existing,
			Group:       row.Group,
			Items:       items,
		})
	}

	// 契约：集合字段一律给空集合而不是 nil。Go 的 nil slice/map 会序列化成 null，
	// 前端按数组处理就会抛异常，而 React 的渲染异常会把整棵组件树卸载成黑屏。
	groups := plan.Groups
	if groups == nil {
		groups = []string{}
	}

	return &QuickCommandImportAnalysis{
		Sets:        plan.Sets,
		Buttons:     plan.Buttons,
		Importable:  plan.Importable,
		Unsupported: plan.Unsupported,
		Existing:    plan.Existing,
		Groups:      groups,
		Rows:        rows,
		Warnings:    mergeWarnings(plan.Warnings, parseWarnings),
	}, nil
}

// ApplyQuickCommandImport 解析并按用户在预览里的决定批量写入快捷命令。
//
// 每个集合一段决定：默认分组 + 勾选后的条目。名称、内容、分组都可以在界面上改过，
// 因此以回传的条目为准（解析只用于兜底校验与集合级告警）。整批只写盘一次。
//
// 冲突策略固定为"跳过同分组内同名同内容"：导入是补齐而不是覆盖，重复导入同一份
// .qbl 是幂等的。
func (a *App) ApplyQuickCommandImport(path string, selections []QuickCommandImportSelection, opts QuickCommandImportOptions) (*QuickCommandImportReport, error) {
	sets, parseWarnings, err := parseQuickCommandImport(path)
	if err != nil {
		return nil, err
	}

	byPath := make(map[string]xshellimport.SetSelection, len(selections))
	for _, selection := range selections {
		source := strings.TrimSpace(selection.Source)
		if source == "" {
			continue
		}
		items := make([]xshellimport.SelectedCommand, 0, len(selection.Items))
		for _, item := range selection.Items {
			items = append(items, xshellimport.SelectedCommand{
				Name:    item.Name,
				Content: item.Content,
				Group:   item.Group,
			})
		}
		byPath[source] = xshellimport.SetSelection{Group: selection.Group, Items: items}
	}

	report, err := xshellimport.ApplyQuickCommands(sets, byPath, quickCommandWriter{a.configMgr}, opts.DefaultGroup)
	if err != nil {
		return nil, err
	}

	// 与单条增删改一样推一次事件，打开着的面板据此立刻刷新，不必等轮询。
	a.emitQuickCommandsUpdated()

	groups := report.Groups
	if groups == nil {
		groups = []string{}
	}

	return &QuickCommandImportReport{
		Imported:           report.Imported,
		SkippedExisting:    report.SkippedExisting,
		SkippedUnsupported: report.SkippedUnsupported,
		Groups:             groups,
		Warnings:           mergeWarnings(report.Warnings, parseWarnings),
	}, nil
}

func parseQuickCommandImport(path string) ([]*xshellimport.QuickButtonSet, []string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil, errors.New("请先选择要导入的 Xshell 快捷按钮文件或目录")
	}

	sets, warnings, err := xshellimport.ParseQuickButtonPath(path)
	if err != nil {
		return nil, nil, describeQuickCommandImportError(err)
	}
	return sets, warnings, nil
}

// describeQuickCommandImportError 把底层错误翻译成用户能照着做的提示。
func describeQuickCommandImportError(err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errors.New("路径不存在，请重新选择 Xshell 快捷按钮文件或目录")
	case errors.Is(err, fs.ErrInvalid):
		return errors.New("不支持的文件类型，请选择 .qbl 文件或包含 .qbl 的目录")
	}
	return err
}

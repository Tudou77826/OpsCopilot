package xshellimport

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// QuickButton 是一个 Xshell 快捷按钮。
//
// 本机 Xshell 8.2 的 .qbl 实测形态（每个按钮一组 Button_<i>_* 键，共 6 个）：
//
//	[Info]
//	Version=8.2
//	Count=1
//	Expanded=1
//	[QuickButton]
//	Button_0_Name=tail
//	Button_0_Param=
//	Button_0_Icon=0
//	Button_0_Desc=
//	Button_0_Type=1
//	Button_0_Action=tail -f /var/log/$app/app.log
//
// 其中只有 Name / Action / Type 对 OpsCopilot 有意义：Name→快捷命令名称，
// Action→命令内容，Type→能否映射。Param/Desc/Icon 在我们的模型里没有对应字段。
type QuickButton struct {
	Index   int
	Name    string
	Content string
	Type    string
}

// QuickButtonSet 是一套快捷按钮，对应一个 .qbl 文件。
//
// Xshell 侧按"集"组织按钮（会话通过 [USERINTERFACE] QuickCommandSet 引用），
// 但文件里没有集合的显示名——[Info] 只有 Version/Count/Expanded。因此这里只保留
// 文件路径与文件名，真正的分组名交给用户在导入界面上确认。
type QuickButtonSet struct {
	// Path 是 .qbl 文件路径，也是导入时指派目标分组的稳定键。
	Path string
	// Name 是文件名（去扩展名），用于让用户辨认来源。
	Name    string
	Buttons []QuickButton
	// Warnings 是文件级的解析提示（例如一个按钮都没读到）。
	Warnings []string
}

// ParseQBL 解析一个 .qbl 的内容，返回其中的按钮（按 Index 升序）。
//
// 与 .xsh 一样，解析器尽量不因为异常而全盘放弃：读不出 Count 就扫描实际键名，
// 读不出的单个按钮字段由 ButtonSupported 判定并告警。
func ParseQBL(text string) []QuickButton {
	doc := ParseINI(text)

	count, err := strconv.Atoi(strings.TrimSpace(doc.Get("Info", "Count")))
	if err != nil || count < 0 {
		count = 0
	}

	indices := buttonIndices(doc, count)
	buttons := make([]QuickButton, 0, len(indices))
	for _, i := range indices {
		prefix := fmt.Sprintf("button_%d_", i)
		buttons = append(buttons, QuickButton{
			Index:   i,
			Name:    doc.Get("QuickButton", prefix+"name"),
			Content: doc.Get("QuickButton", prefix+"action"),
			Type:    doc.Get("QuickButton", prefix+"type"),
		})
	}
	return buttons
}

// buttonIndices 决定要读哪些 Button_<i>_*。
//
// Count 是 Xshell 自己写的按钮数，优先按它索引；缺失或为 0 时退化为扫描实际键名，
// 避免因为一个元数据字段缺失就一个按钮都导不出来。
func buttonIndices(doc *Document, count int) []int {
	if count > 0 {
		out := make([]int, count)
		for i := range out {
			out[i] = i
		}
		return out
	}

	maxIdx := -1
	for _, key := range doc.Keys("QuickButton") {
		if i, ok := parseButtonIndex(key); ok && i > maxIdx {
			maxIdx = i
		}
	}
	if maxIdx < 0 {
		return nil
	}
	out := make([]int, maxIdx+1)
	for i := range out {
		out[i] = i
	}
	return out
}

// parseButtonIndex 从形如 button_0_name 的键名里取出下标。
func parseButtonIndex(key string) (int, bool) {
	rest, ok := strings.CutPrefix(key, "button_")
	if !ok {
		return 0, false
	}
	digits, _, ok := strings.Cut(rest, "_")
	if !ok || digits == "" {
		return 0, false
	}
	i, err := strconv.Atoi(digits)
	if err != nil || i < 0 {
		return 0, false
	}
	return i, true
}

// ButtonSupported 判断一个按钮能否映射成 OpsCopilot 的快捷命令。
// 不支持时返回面向用户的原因，用于告警。
func ButtonSupported(b QuickButton) (bool, string) {
	if strings.TrimSpace(b.Name) == "" {
		return false, "有按钮缺少名称，已跳过"
	}
	if strings.TrimSpace(b.Content) == "" {
		return false, fmt.Sprintf("按钮 %s 没有命令内容，已跳过", b.Name)
	}
	if !supportedButtonType(b.Type) {
		return false, fmt.Sprintf("按钮 %s 的类型为 %s，OpsCopilot 暂只支持「发送字符串」类型，已跳过", b.Name, b.Type)
	}
	return true, ""
}

// supportedButtonType 只放行实测确认可映射的类型：Type=1（发送字符串）。
//
// Type 缺失时按可导入处理——这是版本相关的可选字段，缺失不应当作"未知类型"丢弃。
// 其余取值（脚本、菜单、带参数提示等）语义未经实测，一律跳过并在告警里带上原始值，
// 不做猜测性映射。
func supportedButtonType(raw string) bool {
	t := strings.TrimSpace(raw)
	return t == "" || t == "1"
}

// ParseQuickButtonPath 解析一个 .qbl 文件，或包含 .qbl 的目录（Xshell 的
// QuickButton Files 目录就是这个形态，目录内的子目录也会被递归扫描）。
//
// 返回值第二项是解析阶段的警告（条目读不了、目录里没有 .qbl 等）。单个文件解析
// 失败只产生警告不中断整次导入，与 .xsh 的处理一致。
func ParseQuickButtonPath(target string) ([]*QuickButtonSet, []string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, nil, fs.ErrNotExist
	}

	info, err := os.Stat(target)
	if err != nil {
		return nil, nil, err
	}

	if !info.IsDir() {
		if !strings.EqualFold(filepath.Ext(target), ".qbl") {
			return nil, nil, fs.ErrInvalid
		}
		set, err := parseQuickButtonFile(target)
		if err != nil {
			return nil, nil, err
		}
		return []*QuickButtonSet{set}, nil, nil
	}

	var sets []*QuickButtonSet
	var warnings []string

	walkErr := filepath.WalkDir(target, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, "跳过无法读取的条目 "+p)
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".qbl") {
			return nil
		}
		set, readErr := parseQuickButtonFile(p)
		if readErr != nil {
			warnings = append(warnings, "读取失败 "+p)
			return nil
		}
		sets = append(sets, set)
		return nil
	})
	if walkErr != nil {
		return nil, warnings, walkErr
	}
	if len(sets) == 0 {
		warnings = append(warnings, "该目录下没有找到 .qbl 快捷按钮文件")
	}

	// 遍历顺序受文件系统影响，排序后界面上的行序才稳定。
	sort.Slice(sets, func(i, j int) bool { return sets[i].Path < sets[j].Path })
	return sets, warnings, nil
}

func parseQuickButtonFile(path string) (*QuickButtonSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text, _ := DecodeText(raw)

	set := &QuickButtonSet{
		Path:    path,
		Name:    quickButtonSetName(path),
		Buttons: ParseQBL(text),
	}
	if len(set.Buttons) == 0 {
		set.Warnings = append(set.Warnings, "该文件里没有可识别的快捷按钮")
	}
	return set, nil
}

// quickButtonSetName 用文件名（去扩展名）作为集合名。
// .qbl 里没有集合的显示名，文件名是唯一可辨认的线索。
func quickButtonSetName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

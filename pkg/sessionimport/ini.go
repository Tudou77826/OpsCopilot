package sessionimport

import "strings"

// Document 是一份 INI 风格文档。
//
// Xshell 的 .xsh / .xts 元数据都是这种格式：`[Section]` 分节、`Key=Value` 取键值、
// 行首 `;` 或 `#` 为注释。节名可能带冒号（如 CONNECTION:AUTHENTICATION），
// 键名在不同版本间大小写不一致，因此**键名与节名一律大小写不敏感**匹配。
type Document struct {
	sections map[string]map[string]string
	order    []string
}

// ParseINI 解析 INI 文本。无法识别的行会被忽略——导入器的原则是宁可少读一个
// 未知字段，也不能因为一行异常就让整条会话解析失败。
func ParseINI(text string) *Document {
	doc := &Document{sections: make(map[string]map[string]string)}

	// 先统一换行，避免 \r 混进键值。
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var current map[string]string
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if name == "" {
				continue
			}
			if _, ok := doc.sections[name]; !ok {
				doc.sections[name] = make(map[string]string)
				doc.order = append(doc.order, name)
			}
			current = doc.sections[name]
			continue
		}

		if current == nil {
			continue // 节外的键无法归属，跳过
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		// 同一键重复出现时后者覆盖前者，与常见 INI 实现一致。
		current[key] = strings.TrimSpace(value)
	}
	return doc
}

// Get 取某个节下的键值；节或键不存在时返回空串。
func (d *Document) Get(section, key string) string {
	if d == nil {
		return ""
	}
	sec, ok := d.sections[strings.ToLower(section)]
	if !ok {
		return ""
	}
	return sec[strings.ToLower(key)]
}

// HasSection 判断节是否存在。
func (d *Document) HasSection(section string) bool {
	if d == nil {
		return false
	}
	_, ok := d.sections[strings.ToLower(section)]
	return ok
}

// Sections 返回出现过的节名（小写，按出现顺序）。
func (d *Document) Sections() []string {
	if d == nil {
		return nil
	}
	return d.order
}

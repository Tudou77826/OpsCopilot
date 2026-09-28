package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ReadScenarioResult 场景全文读取结果
type ReadScenarioResult struct {
	Content   string
	Truncated bool
}

// ReadScenario 按 Catalog 条目读取场景原文，超时过 maxBytes 时在 rune 边界截断并置 Truncated。
// 行号语义：LineStart=场景标题行、LineEnd=最后内容行，均为 1-based，但相对的是
// front matter 剥离后的正文（indexer.parseDocument 的解析基准），因此这里要用
// BodyLineOffset 换算回文件绝对行号。
func ReadScenario(dir string, e *ScenarioEntry, maxBytes int) (*ReadScenarioResult, error) {
	if maxBytes <= 0 {
		maxBytes = 32 * 1024
	}
	data, err := os.ReadFile(filepath.Join(dir, e.File))
	if err != nil {
		return nil, fmt.Errorf("读取场景文件失败: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	offset := BodyLineOffset(string(data))
	start := offset + e.LineStart - 1
	if start < 0 {
		start = 0
	}
	if start >= len(lines) {
		return nil, fmt.Errorf("场景行号越界: %s L%d", e.File, e.LineStart)
	}
	end := offset + e.LineEnd
	if end > len(lines) {
		end = len(lines)
	}

	content := strings.Trim(strings.Join(lines[start:end], "\n"), "\r\n \t")
	res := &ReadScenarioResult{Content: content}
	if len(content) > maxBytes {
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(content[cut]) {
			cut--
		}
		res.Content = string([]byte(content[:cut])) + "\n…(已截断)"
		res.Truncated = true
	}
	return res, nil
}

// BodyLineOffset 返回正文首行相对文件首行的 0-based 行偏移，与 indexer.extractFrontMatter
// 的剥离逻辑一致（跳过 front matter，忽略代码块内的 ---）。无 front matter 时为 0。
func BodyLineOffset(content string) int {
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return 0
	}

	lines := strings.Split(content, "\n")
	inCodeBlock := false
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}
		if line == "---" {
			// 正文 = front matter 结束后第一个非空行
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) != "" {
					return j
				}
			}
			return 0
		}
	}
	return 0 // 无闭合 ---，extractFrontMatter 会把全文当正文
}

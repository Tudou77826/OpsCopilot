package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/tools"
	"os"
	"path/filepath"
	"strings"
)

// ReadFileTool 知识库文件读取工具
type ReadFileTool struct {
	knowledgeDir string
	catalog      *knowledge.Catalog // 可为 nil
}

// NewReadFileTool 创建文件读取工具
func NewReadFileTool(knowledgeDir string, catalog *knowledge.Catalog) *ReadFileTool {
	return &ReadFileTool{knowledgeDir: knowledgeDir, catalog: catalog}
}

// Name 返回工具名称
func (t *ReadFileTool) Name() string {
	return "read_knowledge_file"
}

// Description 返回工具描述(给LLM看的说明)
func (t *ReadFileTool) Description() string {
	return "Read the content of a specific documentation file. Each line is prefixed with its line number (format: '  N | content'). Supports start_line/end_line to read a specific line range, or section to read by heading title."
}

// Parameters 返回JSON Schema格式的参数定义
func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "文件路径，如 payment_system_sop.md"},
			"section": {"type": "string", "description": "场景标题（可选），如 API接口超时(504)。指定后只返回该场景的段落内容"},
			"start_line": {"type": "integer", "description": "起始行号（可选，1-based）。配合 end_line 按行号截断读取"},
			"end_line": {"type": "integer", "description": "结束行号（可选，1-based，包含该行）。配合 start_line 按行号截断读取"}
		},
		"required": ["path"],
		"additionalProperties": false
	}`)
}

// Execute 执行文件读取
func (t *ReadFileTool) Execute(ctx context.Context, args map[string]interface{}, emitStatus tools.StatusEmitter) (string, error) {
	path, _ := args["path"].(string)
	section, _ := args["section"].(string)
	startLine, hasStart := toInt(args["start_line"])
	endLine, hasEnd := toInt(args["end_line"])

	if path == "" {
		return "", fmt.Errorf("path参数不能为空")
	}

	// 按行号范围读取
	if hasStart || hasEnd {
		return t.readByLineRange(path, startLine, endLine, hasStart, hasEnd, emitStatus)
	}

	// 如果指定了 section 且有 catalog，尝试精准读取
	if section != "" && t.catalog != nil {
		entry := t.catalog.FindEntry(path, section)
		if entry != nil {
			loc := t.catalog.FindEntryLocation(entry)
			log.Printf("[ReadFile] Catalog hit: path=%s section=%q location=%q lines=%d-%d",
				path, section, loc, entry.LineStart, entry.LineEnd)
			if emitStatus != nil && loc != "" {
				emitStatus("catalog_match", loc)
			}
			if emitStatus != nil {
				emitStatus("reading", fmt.Sprintf("正在阅读文档: %s → %s...", path, section))
			}
			content, err := t.readSectionWithLineNumbers(entry)
			if err == nil && content != "" {
				log.Printf("[ReadFile] Catalog section read OK: %d bytes", len(content))
				return content, nil
			}
			log.Printf("[ReadFile] Catalog section read failed (err=%v), falling back to full file", err)
		} else {
			log.Printf("[ReadFile] Catalog miss: path=%s section=%q (no matching entry), reading full file", path, section)
		}
	} else if section != "" {
		log.Printf("[ReadFile] Section requested but no catalog: path=%s section=%q, reading full file", path, section)
	} else {
		log.Printf("[ReadFile] Full file read: path=%s (no section specified)", path)
	}

	// 全文件读取（无 section 或精准读取失败）
	if emitStatus != nil {
		emitStatus("reading", fmt.Sprintf("正在阅读文档: %s...", path))
	}
	return t.readFullFileWithLineNumbers(path)
}

// readSectionWithLineNumbers 按 catalog 条目的行号范围精准读取段落（带行号）
func (t *ReadFileTool) readSectionWithLineNumbers(entry *knowledge.ScenarioEntry) (string, error) {
	fullPath := filepath.Join(t.knowledgeDir, entry.File)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(content), "\n")

	// LineStart/LineEnd 是 1-based
	start := entry.LineStart - 1
	if start < 0 {
		start = 0
	}
	end := entry.LineEnd
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		return "", fmt.Errorf("invalid line range: %d-%d", entry.LineStart, entry.LineEnd)
	}

	numbered := addLineNumbers(lines, start+1, end)
	result := strings.Join(numbered, "\n")
	if len(result) > 20000 {
		result = result[:20000] + "\n...(truncated)..."
	}
	return result, nil
}

// readByLineRange 按行号范围读取文件
func (t *ReadFileTool) readByLineRange(path string, startLine, endLine int, hasStart, hasEnd bool, emitStatus tools.StatusEmitter) (string, error) {
	if emitStatus != nil {
		desc := fmt.Sprintf("正在阅读文档: %s", path)
		if hasStart && hasEnd {
			desc += fmt.Sprintf(" (行 %d-%d)", startLine, endLine)
		} else if hasStart {
			desc += fmt.Sprintf(" (从行 %d)", startLine)
		} else {
			desc += fmt.Sprintf(" (到行 %d)", endLine)
		}
		emitStatus("reading", desc+"...")
	}

	content, err := knowledge.ReadFile(t.knowledgeDir, path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	lines := strings.Split(content, "\n")

	// 转换为 0-based index
	start := 0
	if hasStart {
		start = startLine - 1
		if start < 0 {
			start = 0
		}
	}

	end := len(lines)
	if hasEnd {
		end = endLine
		if end > len(lines) {
			end = len(lines)
		}
	}

	if start >= end {
		return "", fmt.Errorf("invalid line range: start=%d end=%d total=%d", startLine, endLine, len(lines))
	}

	numbered := addLineNumbers(lines, start+1, end)
	result := strings.Join(numbered, "\n")
	if len(result) > 20000 {
		result = result[:20000] + "\n...(truncated)..."
	}
	log.Printf("[ReadFile] Line range read: path=%s lines=%d-%d resultLen=%d", path, start+1, end, len(result))
	return result, nil
}

// readFullFileWithLineNumbers 读取全文件并加行号
func (t *ReadFileTool) readFullFileWithLineNumbers(path string) (string, error) {
	content, err := knowledge.ReadFile(t.knowledgeDir, path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	lines := strings.Split(content, "\n")
	numbered := addLineNumbers(lines, 1, len(lines))
	result := strings.Join(numbered, "\n")
	if len(result) > 20000 {
		result = result[:20000] + "\n...(truncated)..."
	}
	log.Printf("[ReadFile] Full file read OK: path=%s lines=%d resultLen=%d", path, len(lines), len(result))
	return result, nil
}

// addLineNumbers 为行切片添加行号前缀，范围是 [lineStart, lineEnd]（1-based，闭区间）
func addLineNumbers(lines []string, lineStart, lineEnd int) []string {
	width := len(fmt.Sprintf("%d", lineEnd))
	result := make([]string, 0, lineEnd-lineStart+1)
	for i := lineStart; i <= lineEnd; i++ {
		idx := i - 1 // 0-based index
		if idx < 0 || idx >= len(lines) {
			continue
		}
		result = append(result, fmt.Sprintf("%*d | %s", width, i, lines[idx]))
	}
	return result
}

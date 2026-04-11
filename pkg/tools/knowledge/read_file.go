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
	return "Read the content of a specific documentation file. Optionally specify a section title to read only that scenario's content."
}

// Parameters 返回JSON Schema格式的参数定义
func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "文件路径，如 payment_system_sop.md"},
			"section": {"type": "string", "description": "场景标题（可选），如 API接口超时(504)。指定后只返回该场景的段落内容"}
		},
		"required": ["path"],
		"additionalProperties": false
	}`)
}

// Execute 执行文件读取
func (t *ReadFileTool) Execute(ctx context.Context, args map[string]interface{}, emitStatus tools.StatusEmitter) (string, error) {
	path, _ := args["path"].(string)
	section, _ := args["section"].(string)

	if path == "" {
		return "", fmt.Errorf("path参数不能为空")
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
			content, err := t.readSection(entry)
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
	content, err := knowledge.ReadFile(t.knowledgeDir, path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	if len(content) > 20000 {
		return content[:20000] + "\n...(truncated)...", nil
	}
	return content, nil
}

// readSection 按 catalog 条目的行号范围精准读取段落
func (t *ReadFileTool) readSection(entry *knowledge.ScenarioEntry) (string, error) {
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

	section := strings.Join(lines[start:end], "\n")
	if len(section) > 20000 {
		section = section[:20000] + "\n...(truncated)..."
	}
	return section, nil
}

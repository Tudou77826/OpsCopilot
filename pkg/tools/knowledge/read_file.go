// Package knowledge 提供知识库相关工具的实现
//
// 文件读取工具（ReadFileTool）读取知识库中的特定文档内容。
// 设计要点:
//   - 攌持路径安全检查，防止目录遍历攻击
//   - 自动截断过长内容（20000字符），避免超出上下文限制
//   - 返回纯文本格式（非JSON），便于LLM直接阅读
//
// 优化方向:
//   - 可考虑添加分页读取（超大文件）
//   - 可考虑添加行号范围读取（只读特定行）
//   - 可考虑添加编码检测（自动处理不同编码）
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/tools"
)

// ReadFileTool 知识库文件读取工具
type ReadFileTool struct {
	knowledgeDir string
}

// NewReadFileTool 创建文件读取工具
func NewReadFileTool(knowledgeDir string) *ReadFileTool {
	return &ReadFileTool{knowledgeDir: knowledgeDir}
}

// Name 返回工具名称
func (t *ReadFileTool) Name() string {
	return "read_knowledge_file"
}

// Description 返回工具描述(给LLM看的说明)
func (t *ReadFileTool) Description() string {
	return "Read the content of a specific documentation file."
}

// Parameters 返回JSON Schema格式的参数定义
func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "The relative path of the file to read"}
		},
		"required": ["path"],
		"additionalProperties": false
	}`)
}

// Execute 执行文件读取
// 执行流程:
//  1. 验证path参数
//  2. 调用knowledge.ReadFile读取内容（内置安全检查）
//  3. 截断过长内容（20000字符）
// 返回: 文件内容（纯文本，非JSON)
func (t *ReadFileTool) Execute(ctx context.Context, args map[string]interface{}, emitStatus tools.StatusEmitter) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path参数不能为空")
	}

	if emitStatus != nil {
		emitStatus("reading", fmt.Sprintf("正在阅读文档: %s...", path))
	}

	content, err := knowledge.ReadFile(t.knowledgeDir, path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	// 截断过长内容（避免超出LLM上下文限制）
	if len(content) > 20000 {
		return content[:20000] + "\n...(truncated)...", nil
	}
	return content, nil
}

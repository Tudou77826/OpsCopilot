// Package knowledge 提供知识库相关工具的实现
//
// 文件列表工具（ListFilesTool）列出知识库中的所有文档文件。
// 设计要点：
//   - 简单直接，无复杂参数
//   - 返回 JSON 数组格式的文件路径列表
//
// 优化方向：
//   - 可考虑添加分页支持（大量文件时）
//   - 可考虑添加文件过滤（按类型、日期等）
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/tools"
)

// ListFilesTool 知识库文件列表工具
type ListFilesTool struct {
	knowledgeDir string
}

// NewListFilesTool 创建文件列表工具
func NewListFilesTool(knowledgeDir string) *ListFilesTool {
	return &ListFilesTool{knowledgeDir: knowledgeDir}
}

// Name 返回工具名称
func (t *ListFilesTool) Name() string {
	return "list_knowledge_files"
}

// Description 返回工具描述（给LLM看的说明)
func (t *ListFilesTool) Description() string {
	return "List all available documentation files in the knowledge base."
}

// Parameters 返回JSON Schema格式的参数定义（无参数)
func (t *ListFilesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}, "additionalProperties": false}`)
}

// Execute 执行文件列表
// 返回: JSON数组格式的文件路径列表
func (t *ListFilesTool) Execute(ctx context.Context, args map[string]interface{}, emitStatus tools.StatusEmitter) (string, error) {
	if emitStatus != nil {
		emitStatus("searching", "正在检索文档列表...")
	}

	files, err := knowledge.ListFiles(t.knowledgeDir)
	if err != nil {
		return "", fmt.Errorf("获取文件列表失败: %w", err)
	}

	result, _ := json.Marshal(files)
	return string(result), nil
}

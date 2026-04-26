package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"opscopilot/pkg/llm"
)

// ContentReorganizer 使用 LLM 将非规范文档重组为标准格式
type ContentReorganizer interface {
	Reorganize(ctx context.Context, content string) (*ReorganizedDocument, error)
}

// ReorganizedDocument 重组后的文档
type ReorganizedDocument struct {
	Content string // 完整的 Markdown（含 Front Matter）
	Service string
	Module  string
	DocType string // "sop" | "archive"
}

// LLMContentReorganizer 使用 LLM 重组文档内容
// 复用滑动窗口基础设施处理长文档
type LLMContentReorganizer struct {
	provider llm.Provider
}

// NewLLMContentReorganizer 创建 LLM 内容重组器
func NewLLMContentReorganizer(provider llm.Provider) *LLMContentReorganizer {
	return &LLMContentReorganizer{provider: provider}
}

// reorganizeMeta LLM 输出末尾的 META 信息
type reorganizeMeta struct {
	ResendFromLine *int `json:"resend_from_line"`
}

// Reorganize 将文档内容重组为标准格式
// 对长文档进行分段处理，分段合并产出最终文档
func (r *LLMContentReorganizer) Reorganize(ctx context.Context, content string) (*ReorganizedDocument, error) {
	if r.provider == nil {
		return nil, fmt.Errorf("LLM provider not configured")
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	position := 0

	var reorganizedContent string
	totalSegments := (len(content) + chunkSize - 1) / chunkSize
	segmentIdx := 0

	for position < totalLines {
		segmentIdx++
		chunk, endLine := takeChunk(lines, position, chunkSize)

		var prompt string
		if segmentIdx == 1 {
			prompt = buildReorganizeFirstPrompt(chunk, segmentIdx, totalSegments)
		} else {
			prompt = buildReorganizeSubsequentPrompt(reorganizedContent, chunk, segmentIdx, totalSegments)
		}

		resp, err := r.callLLM(ctx, prompt)
		if err != nil {
			if segmentIdx == 1 {
				return nil, fmt.Errorf("LLM reorganize failed: %w", err)
			}
			// 后续段失败时保留已有结果
			break
		}

		markdown, meta := parseReorganizerOutput(resp)
		reorganizedContent = markdown

		// 根据 resend_from_line 计算下一段起点
		if meta.ResendFromLine != nil && *meta.ResendFromLine > position && *meta.ResendFromLine < endLine {
			position = *meta.ResendFromLine
		} else {
			position = endLine
		}
	}

	if reorganizedContent == "" {
		return nil, fmt.Errorf("LLM produced empty content")
	}

	// 从重组后的文档中提取元信息
	fm, _ := extractFrontMatter(reorganizedContent)
	doc := &ReorganizedDocument{
		Content: reorganizedContent,
	}
	if fm != nil {
		doc.Service = fm["service"]
		doc.Module = fm["module"]
		doc.DocType = fm["type"]
	}

	return doc, nil
}

// callLLM 调用 LLM 获取重组后的文档
func (r *LLMContentReorganizer) callLLM(ctx context.Context, userContent string) (string, error) {
	segmentCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	messages := []llm.ChatMessage{
		{Role: "system", Content: reorganizeSystemPrompt},
		{Role: "user", Content: userContent},
	}

	resp, err := r.provider.ChatCompletion(segmentCtx, messages)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return resp, nil
}

// parseReorganizerOutput 解析 LLM 输出，分离 Markdown 正文和 META 标签
func parseReorganizerOutput(output string) (string, *reorganizeMeta) {
	meta := &reorganizeMeta{}

	// 匹配末尾的 <!-- META: {...} --> 标签
	re := regexp.MustCompile(`(?s)\s*<!--\s*META:\s*(\{.*?\})\s*-->\s*$`)
	matches := re.FindStringSubmatch(output)

	if len(matches) >= 2 {
		markdown := strings.TrimRight(output[:len(output)-len(matches[0])], "\n\r")
		if err := json.Unmarshal([]byte(matches[1]), meta); err != nil {
			// 解析失败时忽略 META，返回原始内容
			return output, meta
		}
		return markdown, meta
	}

	return output, meta
}

const reorganizeSystemPrompt = `你是一个运维文档整理助手。你的任务是将运维文档重组为标准格式。

## 判断文档类型

阅读文档内容，判断属于以下哪种类型：
- **SOP（标准操作手册）**：包含多个排查场景、操作步骤，适合作为日常参考手册
- **Archive（历史排查记录）**：单次问题排查的完整记录，包含问题现象、原因、解决方案

## SOP 模板

按以下格式输出：

---
service: <服务/系统名>
module: <模块名>
type: sop
---

# <文档标题>

## 场景：<场景名称>

- **现象**: <现象描述>
- **关键词**: <逗号分隔的关键词>
- **涉及组件**: <逗号分隔的组件>

**可能原因**:
1. <原因1>

**排查步骤**:
1. <步骤1>

（可以包含多个 ## 场景 章节）

## Archive 模板

按以下格式输出：

---
service: <服务/系统名>
module: <模块名>
type: archive
---

# <问题标题>

## 问题现象

<现象描述>

## 关键词

<逗号分隔的关键词>

## 涉及组件

<逗号分隔的组件>

## 根本原因

<原因分析>

## 解决方案

<解决步骤>

## 整理要求

1. 保留所有有价值的排查步骤、命令、结论
2. 去除冗余内容（如重复的日志片段、无意义的对话）
3. 如果原文档有代码块，保留格式
4. Front Matter 中填入正确的 service、module、type
5. service 和 module 要从内容中推断，无法推断时 service 填 "Unknown"，module 填 "默认模块"
6. 输出纯 Markdown，不要用 JSON 包裹
7. 在末尾添加一行 META 标签：<!-- META: {"resend_from_line": null} -->

## 多段处理

如果这是第 1 段，请完整输出重组后的文档。
如果末尾的内容看起来被截断了（句子不完整、代码块未闭合），在 META 中设置 resend_from_line 为最后一个完整段落的行号（从1开始）。`

func buildReorganizeFirstPrompt(content string, segmentNum, totalSegments int) string {
	return fmt.Sprintf("这是文档的第 %d 段（共 %d 段）。\n\n"+
		"请将以下内容重组为标准格式的运维文档：\n\n%s",
		segmentNum, totalSegments, content)
}

func buildReorganizeSubsequentPrompt(existingContent, newChunk string, segmentNum, totalSegments int) string {
	return fmt.Sprintf("这是文档的第 %d 段（共 %d 段）。\n\n"+
		"以下是已重组的内容：\n\n%s\n\n"+
		"以下是新的原始内容片段：\n\n%s\n\n"+
		"请将新片段中有价值的信息合并到已重组内容中。如果新片段没有遗漏的重要信息，直接返回已重组内容。",
		segmentNum, totalSegments, existingContent, newChunk)
}

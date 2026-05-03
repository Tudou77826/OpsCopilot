package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"opscopilot/pkg/llm"
)

// ContentReorganizer 使用 LLM 将非规范文档重组为标准格式
type ContentReorganizer interface {
	Reorganize(ctx context.Context, content string) ([]*ReorganizedDocument, error)
}

// ReorganizedDocument 重组后的文档
type ReorganizedDocument struct {
	Content string // 完整的 Markdown（含 Front Matter）
	Service string
	Module  string
	DocType string // "sop" | "archive"
	SubPath string // 拆分目标路径，如 "核心支付模块/troubleshooting_history.md"；空则用原路径
}

// LLMContentReorganizer 使用 LLM 重组文档内容
// 复用滑动窗口基础设施处理长文档
type LLMContentReorganizer struct {
	provider   llm.Provider
	moduleList []ModuleConfigEntry
	extractor  *LLMMetadataExtractor
}

// NewLLMContentReorganizer 创建 LLM 内容重组器
func NewLLMContentReorganizer(provider llm.Provider, moduleList []ModuleConfigEntry) *LLMContentReorganizer {
	r := &LLMContentReorganizer{provider: provider, moduleList: moduleList}
	r.extractor = NewLLMMetadataExtractor(provider, r.moduleList)
	return r
}

// reorganizeMeta LLM 输出末尾的 META 信息
type reorganizeMeta struct {
	ResendFromLine *int `json:"resend_from_line"`
}

// Reorganize 将文档内容重组为标准格式
// 对长文档进行分段处理，分段合并产出最终文档
// 支持按模块拆分为多个文档（1:N）
func (r *LLMContentReorganizer) Reorganize(ctx context.Context, content string) ([]*ReorganizedDocument, error) {
	if r.provider == nil {
		return nil, fmt.Errorf("LLM provider not configured")
	}

	// 检测是否为拆分候选（包含多个 ## 标题 → 可能是多模块混合文档）
	recordTitles := extractRecordTitles(content)
	if len(recordTitles) >= 2 {
		return r.reorganizeSplit(ctx, content)
	}

	// 单文档模式
	doc, err := r.reorganizeSingle(ctx, content)
	if err != nil {
		return nil, err
	}
	return []*ReorganizedDocument{doc}, nil
}

// reorganizeSplit 按模块拆分文档后分别重组
func (r *LLMContentReorganizer) reorganizeSplit(ctx context.Context, content string) ([]*ReorganizedDocument, error) {
	groups, err := r.extractor.AnalyzeModules(ctx, content)
	if err != nil {
		log.Printf("[reorganize] AnalyzeModules 失败，回退到单文档模式: %v", err)
		doc, err := r.reorganizeSingle(ctx, content)
		if err != nil {
			return nil, err
		}
		return []*ReorganizedDocument{doc}, nil
	}
	if len(groups) == 0 {
		log.Printf("[reorganize] AnalyzeModules 返回 0 个分组（LLM 判定所有章节同模块），回退到单文档模式")
		doc, err := r.reorganizeSingle(ctx, content)
		if err != nil {
			return nil, err
		}
		return []*ReorganizedDocument{doc}, nil
	}
	log.Printf("[reorganize] 拆分为 %d 个模块", len(groups))

	var docs []*ReorganizedDocument
	for _, group := range groups {
		doc, err := r.reorganizeSingle(ctx, group.Content)
		if err != nil {
			// 任一模块失败则整体失败，避免部分输出导致数据丢失
			return nil, fmt.Errorf("module %q reorganize failed: %w", group.Module, err)
		}
		doc.SubPath = group.Module
		docs = append(docs, doc)
	}

	return docs, nil
}

// reorganizeSingle 对单个文档内容进行重组（现有逻辑）
func (r *LLMContentReorganizer) reorganizeSingle(ctx context.Context, content string) (*ReorganizedDocument, error) {
	// 短内容直接走单次 LLM 调用，跳过滑动窗口开销
	if len(content) <= chunkSize {
		return r.reorganizeShort(ctx, content)
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	position := 0

	var reorganizedContent string
	segmentIdx := 0
	totalSegments := calcTotalSegments(lines, chunkSize)

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

		if segmentIdx == 1 {
			// 第一段：LLM 输出完整的基础文档
			reorganizedContent = markdown
		} else {
			// 后续段：追加新提取的内容（如果有）
			if strings.TrimSpace(markdown) != "" {
				reorganizedContent = reorganizedContent + "\n\n" + markdown
			}
		}

		// 根据 resend_from_line 计算下一段起点
		// LLM 返回 1-based 行号，需转换为 0-based 索引
		if meta.ResendFromLine != nil {
			resendIdx := *meta.ResendFromLine - 1 // 1-based → 0-based
			if resendIdx >= position && resendIdx < endLine {
				position = resendIdx
			} else {
				position = endLine
			}
		} else {
			position = endLine
		}
	}

	if reorganizedContent == "" {
		return nil, fmt.Errorf("LLM produced empty content")
	}

	return buildDocFromContent(reorganizedContent)
}

// reorganizeShort 对短内容进行单次 LLM 重组
// 短内容无需分段，使用精简 prompt 减少不必要的多段处理指令
func (r *LLMContentReorganizer) reorganizeShort(ctx context.Context, content string) (*ReorganizedDocument, error) {
	prompt := fmt.Sprintf("请将以下内容重组为标准格式的运维文档：\n\n%s", content)

	resp, err := r.callLLM(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM reorganize failed: %w", err)
	}

	markdown, _ := parseReorganizerOutput(resp)
	if strings.TrimSpace(markdown) == "" {
		return nil, fmt.Errorf("LLM produced empty content")
	}

	return buildDocFromContent(markdown)
}

// buildDocFromContent 从重组后的 Markdown 内容构建 ReorganizedDocument
func buildDocFromContent(content string) (*ReorganizedDocument, error) {
	fm, _ := extractFrontMatter(content)
	doc := &ReorganizedDocument{
		Content: content,
	}
	if fm != nil {
		doc.Service = fm["service"]
		doc.Module = fm["module"]
		doc.DocType = fm["type"]
	}
	return doc, nil
}

// llmCallTimeout LLM 调用超时时间（5 分钟）
const llmCallTimeout = 5 * time.Minute

// callLLM 调用 LLM 获取重组后的文档
func (r *LLMContentReorganizer) callLLM(ctx context.Context, userContent string) (string, error) {
	segmentCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	defer cancel()

	systemPrompt := reorganizeSystemPrompt
	moduleHint := FormatModuleList(r.moduleList)
	if moduleHint != "" {
		systemPrompt = systemPrompt + "\n\n" + moduleHint
	}

	messages := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	resp, err := r.provider.ChatCompletion(segmentCtx, messages)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return resp, nil
}

// reMetaRegex 匹配 LLM 输出末尾的 <!-- META: {...} --> 标签
var reMetaRegex = regexp.MustCompile(`(?s)\s*<!--\s*META:\s*(\{.*?\})\s*-->\s*$`)

// parseReorganizerOutput 解析 LLM 输出，分离 Markdown 正文和 META 标签
func parseReorganizerOutput(output string) (string, *reorganizeMeta) {
	meta := &reorganizeMeta{}

	matches := reMetaRegex.FindStringSubmatch(output)

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
如果是后续段，只提取新片段中有价值的信息，输出可追加的 Markdown 片段。
如果末尾的内容看起来被截断了（句子不完整、代码块未闭合），在 META 中设置 resend_from_line 为最后一个完整段落的行号（从1开始）。`

// subsequentContextMaxChars 后续段 prompt 中保留已重组内容的最大字符数
// 避免后续段 prompt 无限膨胀超出 LLM 上下文窗口
const subsequentContextMaxChars = 4000

func buildReorganizeFirstPrompt(content string, segmentNum, totalSegments int) string {
	return fmt.Sprintf("这是文档的第 %d 段（共 %d 段）。\n\n"+
		"请将以下内容重组为标准格式的运维文档：\n\n%s",
		segmentNum, totalSegments, content)
}

func buildReorganizeSubsequentPrompt(existingContent, newChunk string, segmentNum, totalSegments int) string {
	// 只保留已重组内容的尾部作为上下文，避免 prompt 膨胀
	contextPreview := tailContent(existingContent, subsequentContextMaxChars)

	return fmt.Sprintf("这是文档的第 %d 段（共 %d 段）。\n\n"+
		"以下是已重组内容的尾部（供衔接参考）：\n\n%s\n\n"+
		"以下是新的原始内容片段：\n\n%s\n\n"+
		"请从新片段中提取有价值的新信息（排查步骤、命令、结论等），以 Markdown 格式输出。"+
		"内容应该能直接追加到已有文档末尾。如果新片段没有有价值的信息，输出空内容。",
		segmentNum, totalSegments, contextPreview, newChunk)
}

// tailContent 返回 content 的最后 maxChars 个字符
// 如果 content 长度不超过 maxChars，返回原文
func tailContent(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	// 从 maxChars 位置开始找第一个换行符，避免截断在行中间
	start := len(content) - maxChars
	if idx := strings.Index(content[start:], "\n"); idx >= 0 {
		start += idx + 1
	}
	return "...(前面内容已省略)\n" + content[start:]
}

// calcTotalSegments 通过模拟 takeChunk 遍历计算实际的段数
// 避免基于字符数的估算不准确
func calcTotalSegments(lines []string, maxChars int) int {
	totalLines := len(lines)
	position := 0
	segments := 0
	for position < totalLines {
		_, endLine := takeChunk(lines, position, maxChars)
		segments++
		position = endLine
	}
	return segments
}

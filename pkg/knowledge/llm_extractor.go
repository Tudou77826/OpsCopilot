package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"opscopilot/pkg/llm"
)

// MetadataExtractor 从文档内容中提取元数据的接口
type MetadataExtractor interface {
	ExtractMetadata(ctx context.Context, content string) (*ExtractedMetadata, error)
}

// ExtractedMetadata LLM 提取的元数据
type ExtractedMetadata struct {
	Service    string   `json:"service"`
	Module     string   `json:"module"`
	Keywords   []string `json:"keywords"`
	Components []string `json:"components"`
	Phenomena  string   `json:"phenomena"`
}

// LLMMetadataExtractor 使用 LLM 从文档内容中提取元数据
// 支持长文档分段处理，通过 resend_from_line 机制处理被截断的内容
type LLMMetadataExtractor struct {
	provider llm.Provider
}

// NewLLMMetadataExtractor 创建 LLM 元数据提取器
func NewLLMMetadataExtractor(provider llm.Provider) *LLMMetadataExtractor {
	return &LLMMetadataExtractor{provider: provider}
}

const chunkSize = 80000 // ~80K 字符一段

// segmentResult 单段 LLM 提取结果
type segmentResult struct {
	Service       string   `json:"service"`
	Module        string   `json:"module"`
	Keywords      []string `json:"keywords"`
	Components    []string `json:"components"`
	Phenomena     string   `json:"phenomena"`
	ResendFromLine *int    `json:"resend_from_line"`
}

// ExtractMetadata 从文档内容中提取元数据
// 对长文档进行分段处理，合并所有段的提取结果
func (e *LLMMetadataExtractor) ExtractMetadata(ctx context.Context, content string) (*ExtractedMetadata, error) {
	if e.provider == nil {
		return nil, fmt.Errorf("LLM provider not configured")
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	position := 0

	var firstResult *segmentResult
	allKeywords := make(map[string]bool)
	allComponents := make(map[string]bool)

	segmentIdx := 0
	totalSegments := (len(content) + chunkSize - 1) / chunkSize

	for position < totalLines {
		segmentIdx++
		// 从当前行位置计算字符偏移，取 ~chunkSize 字符
		chunk, endLine := takeChunk(lines, position, chunkSize)

		var prompt string
		if segmentIdx == 1 {
			prompt = buildFirstSegmentPrompt(chunk, segmentIdx, totalSegments)
		} else {
			existingKeywords := keysOf(allKeywords)
			existingComponents := keysOf(allComponents)
			prompt = buildSubsequentSegmentPrompt(chunk, segmentIdx, totalSegments, existingKeywords, existingComponents)
		}

		result, err := e.callLLM(ctx, prompt)
		if err != nil {
			// 单段失败不阻塞，继续下一段
			position = endLine
			continue
		}

		// 保存第一段结果（包含 service/module/phenomena）
		if firstResult == nil {
			firstResult = result
		}

		// 合并 keywords 和 components
		for _, kw := range result.Keywords {
			allKeywords[kw] = true
		}
		for _, comp := range result.Components {
			allComponents[comp] = true
		}

		// 根据 resend_from_line 计算下一段起点
		if result.ResendFromLine != nil && *result.ResendFromLine > position && *result.ResendFromLine < endLine {
			position = *result.ResendFromLine
		} else {
			position = endLine
		}
	}

	if firstResult == nil {
		return nil, fmt.Errorf("all segments failed to extract metadata")
	}

	return &ExtractedMetadata{
		Service:    firstResult.Service,
		Module:     firstResult.Module,
		Keywords:   keysOf(allKeywords),
		Components: keysOf(allComponents),
		Phenomena:  firstResult.Phenomena,
	}, nil
}

// takeChunk 从 lines[position:] 取 ~maxChars 字符的内容
// 返回拼接的文本内容和实际结束行号
func takeChunk(lines []string, position, maxChars int) (string, int) {
	var sb strings.Builder
	endLine := position
	for i := position; i < len(lines); i++ {
		line := lines[i]
		if sb.Len()+len(line)+1 > maxChars && i > position {
			break
		}
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(line)
		endLine = i + 1
	}
	return sb.String(), endLine
}

// callLLM 调用 LLM 并解析 JSON 响应
func (e *LLMMetadataExtractor) callLLM(ctx context.Context, userContent string) (*segmentResult, error) {
	segmentCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	messages := []llm.ChatMessage{
		{Role: "system", Content: "你是一个运维文档分析助手。请严格按照要求的 JSON 格式输出，不要包含任何其他内容。"},
		{Role: "user", Content: userContent},
	}

	resp, err := e.provider.ChatCompletion(segmentCtx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 清理 JSON 响应
	cleaned := strings.TrimSpace(resp)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var result segmentResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return &result, nil
}

// buildFirstSegmentPrompt 构建第一段的 prompt（提取所有字段）
func buildFirstSegmentPrompt(content string, segmentNum, totalSegments int) string {
	return fmt.Sprintf("你正在处理一篇运维文档的第 %d 段（共 %d 段）。\n"+
		"请从内容中提取：\n"+
		"- service: 所属服务/系统名\n"+
		"- module: 所属模块\n"+
		"- keywords: 关键词列表\n"+
		"- components: 涉及的技术组件\n"+
		"- phenomena: 问题现象概述\n\n"+
		"另外，请检查文档末尾是否存在不完整的内容，包括但不限于：\n"+
		"- 句子写到一半被截断\n"+
		"- 代码块未闭合（缺少 ```）\n"+
		"- Markdown 表格行不完整\n"+
		"- 排查步骤/操作流程描述到一半\n"+
		"- 任何语义上未完结的段落\n\n"+
		"如果存在上述情况，请通过 resend_from_line 字段返回最后一个完整段落的起始行号。\n"+
		"如果末尾内容完整，返回 null。\n\n"+
		"输出纯JSON：\n"+
		"{\"service\":\"\",\"module\":\"\",\"keywords\":[],\"components\":[],\"phenomena\":\"\",\"resend_from_line\":null}\n\n"+
		"文档内容：\n%s", segmentNum, totalSegments, content)
}

// buildSubsequentSegmentPrompt 构建后续段的 prompt（只提取 keywords/components）
func buildSubsequentSegmentPrompt(content string, segmentNum, totalSegments int, existingKeywords, existingComponents []string) string {
	return fmt.Sprintf("你正在处理同一篇文档的第 %d 段（共 %d 段）。\n"+
		"前面已提取的关键词：%s，组件：%s。\n\n"+
		"请继续从这段内容中补充新的关键词和技术组件。\n"+
		"如果没有新信息，返回空列表即可。\n\n"+
		"同样检查末尾是否有不完整内容（句子截断、代码块未闭合、步骤描述到一半等），\n"+
		"如有请通过 resend_from_line 返回最后一个完整段落的行号。\n\n"+
		"输出纯JSON：\n"+
		"{\"keywords\":[],\"components\":[],\"resend_from_line\":null}\n\n"+
		"文档内容：\n%s",
		segmentNum, totalSegments,
		strings.Join(existingKeywords, ", "),
		strings.Join(existingComponents, ", "),
		content)
}

// keysOf 返回 map 的所有 key 组成的切片
func keysOf(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

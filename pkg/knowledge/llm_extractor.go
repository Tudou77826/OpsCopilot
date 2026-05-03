package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

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
	provider   llm.Provider
	moduleList []ModuleConfigEntry // 可选：约束 LLM 输出的 module 范围
}

// NewLLMMetadataExtractor 创建 LLM 元数据提取器
func NewLLMMetadataExtractor(provider llm.Provider, moduleList []ModuleConfigEntry) *LLMMetadataExtractor {
	return &LLMMetadataExtractor{provider: provider, moduleList: moduleList}
}

const chunkSize = 70000 // ~70K 字符一段（留余量给 LLM 输出）

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
	totalSegments := calcTotalSegments(lines, chunkSize)

	for position < totalLines {
		segmentIdx++
		// 从当前行位置计算字符偏移，取 ~chunkSize 字符
		chunk, endLine := takeChunk(lines, position, chunkSize)

		var prompt string
		if segmentIdx == 1 {
			prompt = e.buildFirstSegmentPrompt(chunk, segmentIdx, totalSegments)
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
		// LLM 返回 1-based 行号，需转换为 0-based 索引
		if result.ResendFromLine != nil {
			resendIdx := *result.ResendFromLine - 1 // 1-based → 0-based
			if resendIdx >= position && resendIdx < endLine {
				position = resendIdx
			} else {
				position = endLine
			}
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
	segmentCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	defer cancel()

	messages := []llm.ChatMessage{
		{Role: "system", Content: "你是一个运维文档分析助手。请严格按照要求的 JSON 格式输出，不要包含任何其他内容。"},
		{Role: "user", Content: userContent},
	}

	resp, err := e.provider.ChatCompletion(segmentCtx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	var result segmentResult
	if err := json.Unmarshal([]byte(cleanJSONResponse(resp)), &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return &result, nil
}

// buildFirstSegmentPrompt 构建第一段的 prompt（提取所有字段）
func (e *LLMMetadataExtractor) buildFirstSegmentPrompt(content string, segmentNum, totalSegments int) string {
	moduleHint := FormatModuleList(e.moduleList)
	prompt := fmt.Sprintf("你正在处理一篇运维文档的第 %d 段（共 %d 段）。\n"+
		"请从内容中提取：\n"+
		"- service: 所属服务/系统名\n"+
		"- module: 所属模块\n"+
		"- keywords: 关键词列表\n"+
		"- components: 涉及的技术组件\n"+
		"- phenomena: 问题现象概述\n\n", segmentNum, totalSegments)

	if moduleHint != "" {
		prompt += moduleHint + "\n\n"
	}

	prompt += "另外，请检查文档末尾是否存在不完整的内容，包括但不限于：\n" +
		"- 句子写到一半被截断\n" +
		"- 代码块未闭合（缺少 ```）\n" +
		"- Markdown 表格行不完整\n" +
		"- 排查步骤/操作流程描述到一半\n" +
		"- 任何语义上未完结的段落\n\n" +
		"如果存在上述情况，请通过 resend_from_line 字段返回最后一个完整段落的起始行号。\n" +
		"如果末尾内容完整，返回 null。\n\n" +
		"输出纯JSON（不要用 markdown 代码块包裹，不要添加任何说明文字）：\n" +
		"{\"service\":\"\",\"module\":\"\",\"keywords\":[],\"components\":[],\"phenomena\":\"\",\"resend_from_line\":null}\n\n" +
		"文档内容：\n" + content

	return prompt
}

// buildSubsequentSegmentPrompt 构建后续段的 prompt（只提取 keywords/components）
func buildSubsequentSegmentPrompt(content string, segmentNum, totalSegments int, existingKeywords, existingComponents []string) string {
	return fmt.Sprintf("你正在处理同一篇文档的第 %d 段（共 %d 段）。\n"+
		"前面已提取的关键词：%s，组件：%s。\n\n"+
		"请继续从这段内容中补充新的关键词和技术组件。\n"+
		"如果没有新信息，返回空列表即可。\n\n"+
		"同样检查末尾是否有不完整内容（句子截断、代码块未闭合、步骤描述到一半等），\n"+
		"如有请通过 resend_from_line 返回最后一个完整段落的行号。\n\n"+
		"输出纯JSON（不要用 markdown 代码块包裹，不要添加任何说明文字）：\n"+
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

// reJSONBlock 从 LLM 响应中提取 JSON 对象的正则
// 匹配第一个 { 到最后一个 }，贪婪匹配以处理嵌套 JSON
var reJSONBlock = regexp.MustCompile(`(?s)\{.*\}`)

// cleanJSONResponse 清理 LLM 返回的 JSON 响应
// 先尝试从 markdown 代码块中提取，再用正则匹配第一个 JSON 对象
func cleanJSONResponse(resp string) string {
	cleaned := strings.TrimSpace(resp)

	// 如果被 ```json ... ``` 或 ``` ... ``` 包裹，提取代码块内容
	if strings.HasPrefix(cleaned, "```") {
		afterFirst := cleaned
		if idx := strings.Index(afterFirst, "\n"); idx >= 0 {
			afterFirst = afterFirst[idx+1:]
		}
		if idx := strings.LastIndex(afterFirst, "```"); idx >= 0 {
			afterFirst = afterFirst[:idx]
		}
		cleaned = strings.TrimSpace(afterFirst)
	}

	// 如果整个字符串已经是合法 JSON，直接返回
	if json.Valid([]byte(cleaned)) {
		return cleaned
	}

	// 兜底：用正则提取第一个 JSON 对象
	if match := reJSONBlock.FindString(cleaned); match != "" {
		return match
	}

	return cleaned
}

// RecordGroup 按模块分组的记录
type RecordGroup struct {
	Module  string
	Content string // 该模块的原始内容片段
}

// recordModule LLM 返回的记录-模块映射
type recordModule struct {
	Title  string `json:"title"`  // 记录标题
	Module string `json:"module"` // 归属模块
}

// analyzeModulesResult LLM 分析模块的结果
type analyzeModulesResult struct {
	Records []recordModule `json:"records"`
}

// analyzeModulesMaxContent AnalyzeModules 传给 LLM 的最大内容长度
// 模块分析只需看标题和每段开头，不需要全文
const analyzeModulesMaxContent = 40000

// AnalyzeModules 分析文档中各记录所属的模块
// 用于多模块混合文档的拆分
// 有 module_list 时约束 LLM 从列表选择，无则让 LLM 自行推断模块名
func (e *LLMMetadataExtractor) AnalyzeModules(ctx context.Context, content string) ([]*RecordGroup, error) {
	if e.provider == nil {
		return nil, fmt.Errorf("LLM provider not configured")
	}

	// 提取文档中所有 ## 标题作为分段依据
	recordTitles := extractRecordTitles(content)
	if len(recordTitles) <= 1 {
		return nil, nil // 单记录或无记录，无需拆分
	}

	// 对超长文档截断，避免超出 LLM 上下文窗口
	// 保留标题 + 每段开头的若干内容
	analysisContent := content
	if len(analysisContent) > analyzeModulesMaxContent {
		analysisContent = truncateForModuleAnalysis(content, analyzeModulesMaxContent)
	}

	// 构建分析 prompt
	moduleHint := FormatModuleList(e.moduleList)
	prompt := "以下文档包含多个章节（## 标题）。请根据内容判断每个章节所属的模块。\n\n"
	if moduleHint != "" {
		prompt += moduleHint + "\n\n"
	} else {
		prompt += "请根据文档内容自行推断每个章节所属的模块名（如\"核心支付模块\"、\"订单模块\"等）。\n\n"
	}

	prompt += "章节标题列表：\n"
	for i, title := range recordTitles {
		prompt += fmt.Sprintf("%d. %s\n", i+1, title)
	}

	prompt += "\n如果所有章节都属于同一模块，返回空数组 []。\n" +
		"如果可以按模块分组，为每个章节标注所属模块名。\n\n" +
		"输出纯JSON（不要用 markdown 代码块包裹，不要添加任何说明文字）：\n" +
		"{\"records\":[{\"title\":\"章节标题\",\"module\":\"模块名\"}, ...]}\n\n" +
		"文档内容：\n" + analysisContent

	segmentCtx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	defer cancel()

	messages := []llm.ChatMessage{
		{Role: "system", Content: "你是一个运维文档分析助手。请严格按照要求的 JSON 格式输出，不要包含任何其他内容。"},
		{Role: "user", Content: prompt},
	}

	resp, err := e.provider.ChatCompletion(segmentCtx, messages)
	if err != nil {
		log.Printf("[AnalyzeModules] LLM 调用失败: %v", err)
		return nil, fmt.Errorf("LLM call for module analysis failed: %w", err)
	}

	var result analyzeModulesResult
	if err := json.Unmarshal([]byte(cleanJSONResponse(resp)), &result); err != nil {
		log.Printf("[AnalyzeModules] JSON 解析失败, raw=%.200s", resp)
		return nil, fmt.Errorf("failed to parse module analysis result: %w", err)
	}

	// 按模块分组原始内容
	groups := groupContentByModule(content, recordTitles, result.Records)
	moduleNames := make([]string, 0, len(groups))
	for _, g := range groups {
		moduleNames = append(moduleNames, g.Module)
	}
	log.Printf("[AnalyzeModules] 结果: %d 条记录 → %d 个模块 (%v)", len(result.Records), len(groups), moduleNames)
	return groups, nil
}

// truncateForModuleAnalysis 对超长文档做智能截断
// 保留每个 ## 段落的标题 + 前若干行内容，确保 LLM 能判断模块归属
// 前言（第一个 ## 之前的内容）最多保留 preambleMaxChars 字符
// 确保所有 ## 标题都出现在截断结果中，即使正文被省略
const preambleMaxChars = 500
const sectionPreviewLines = 10

func truncateForModuleAnalysis(content string, maxChars int) string {
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	foundFirstH2 := false
	preambleWritten := 0
	preambleTruncated := false

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		isSectionHeader := strings.HasPrefix(trimmed, "## ")

		if !foundFirstH2 && !isSectionHeader {
			// 前言区域：限制总长度
			if preambleWritten+len(lines[i])+1 > preambleMaxChars {
				if !preambleTruncated {
					sb.WriteString("\n...(前言已截断)\n")
					preambleTruncated = true
				}
				continue
			}
			sb.WriteString(lines[i])
			sb.WriteByte('\n')
			preambleWritten += len(lines[i]) + 1
			continue
		}

		if isSectionHeader {
			foundFirstH2 = true
			// 即使超出上限，也至少保留标题行
			if sb.Len()+len(lines[i])+1 > maxChars {
				sb.WriteString(lines[i])
				sb.WriteString("\n...(内容已省略)\n")
				continue
			}
			sb.WriteString(lines[i])
			sb.WriteByte('\n')

			// 保留标题后的前若干行内容
			for j := 1; j <= sectionPreviewLines && i+j < len(lines); j++ {
				nextTrimmed := strings.TrimSpace(lines[i+j])
				if strings.HasPrefix(nextTrimmed, "## ") {
					break
				}
				if sb.Len()+len(lines[i+j])+1 > maxChars {
					sb.WriteString("\n...(内容已省略)\n")
					break
				}
				sb.WriteString(lines[i+j])
				sb.WriteByte('\n')
			}
			// 跳过已写入的行
			for k := i + 1; k < len(lines) && k <= i+sectionPreviewLines; k++ {
				if strings.HasPrefix(strings.TrimSpace(lines[k]), "## ") {
					break
				}
				i++
			}
		}
	}

	return sb.String()
}

// extractRecordTitles 从文档中提取所有 ## 标题
func extractRecordTitles(content string) []string {
	var titles []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			title := strings.TrimPrefix(trimmed, "## ")
			title = strings.TrimSpace(title)
			if title != "" {
				titles = append(titles, title)
			}
		}
	}
	return titles
}

// groupContentByModule 根据 LLM 返回的记录-模块映射，按模块分组文档内容
// 同一模块的多条记录合并为一个 group；第一条 ## 之前的内容（前言）追加到每个 group
func groupContentByModule(content string, titles []string, records []recordModule) []*RecordGroup {
	// 建立 title → module 映射
	titleToModule := make(map[string]string)
	for _, r := range records {
		titleToModule[r.Title] = r.Module
	}

	// 按 ## 标题拆分内容，同时收集前言
	lines := strings.Split(content, "\n")
	var preamble strings.Builder // 第一个 ## 之前的内容
	// sectionContent 收集每个 module 的内容片段
	sectionContent := make(map[string][]string) // module → []content片段
	// moduleOrder 保持模块出现顺序
	var moduleOrder []string
	moduleSeen := make(map[string]bool)
	var currentTitle string
	var currentSection strings.Builder

	flushSection := func() {
		if currentTitle == "" || currentSection.Len() == 0 {
			return
		}
		module := titleToModule[currentTitle]
		if module == "" {
			module = "默认模块"
		}
		sectionContent[module] = append(sectionContent[module], currentSection.String())
		if !moduleSeen[module] {
			moduleSeen[module] = true
			moduleOrder = append(moduleOrder, module)
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			flushSection()
			currentTitle = strings.TrimPrefix(trimmed, "## ")
			currentSection.Reset()
		}
		if currentTitle == "" {
			// 前言内容（第一个 ## 之前）
			if preamble.Len() > 0 {
				preamble.WriteByte('\n')
			}
			preamble.WriteString(line)
		} else {
			if currentSection.Len() > 0 {
				currentSection.WriteByte('\n')
			}
			currentSection.WriteString(line)
		}
	}
	flushSection()

	preambleStr := strings.TrimSpace(preamble.String())

	var groups []*RecordGroup
	for _, module := range moduleOrder {
		parts := sectionContent[module]
		var combined strings.Builder
		if preambleStr != "" {
			combined.WriteString(preambleStr)
			combined.WriteByte('\n')
		}
		for i, part := range parts {
			if i > 0 {
				combined.WriteByte('\n')
			}
			combined.WriteString(part)
		}
		groups = append(groups, &RecordGroup{
			Module:  module,
			Content: combined.String(),
		})
	}

	return groups
}

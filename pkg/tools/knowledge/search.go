// Package knowledge 提供知识库相关工具的实现
//
// 搜索工具（SearchTool）支持智能关键词提取和多语言搜索，是定位Agent的核心工具。
// 设计要点：
//   - 通过 TermExtractor 接口注入关键词提取能力，支持 LLM 增强的搜索
//   - 通过 TermCache 接口支持跨调用的关键词缓存，避免重复提取
//   - 支持 chooseSearchKey 策略，自动选择最优搜索关键词（中文优先）
//   - 支持中英文混合搜索，自动将英文关键词补充到中文搜索中
//
// 优化方向：
//   - 可考虑添加语义搜索（向量检索）作为补充
//   - 可考虑添加搜索结果缓存，避免重复搜索相同内容
//   - 可考虑支持模糊匹配和同义词扩展
package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/tools"
	"sort"
	"strings"
	"time"
	"unicode"
)

// TermExtractor 关键词提取器接口
// 用于从搜索查询中提取加权关键词，提升搜索质量
type TermExtractor interface {
	ExtractWeightedTerms(ctx context.Context, text string) ([]knowledge.WeightedTerm, error)
}

// TermExtractorWithRetry 支持重试次数的关键词提取器接口
// 优先使用此接口，支持自定义重试次数
type TermExtractorWithRetry interface {
	ExtractWeightedTermsWithRetry(ctx context.Context, text string, maxAttempts int) ([]knowledge.WeightedTerm, error)
}

// TermCache 词项缓存接口
// 用于缓存已提取的关键词，避免重复调用 LLM
type TermCache interface {
	Get(key string) []knowledge.WeightedTerm
	Set(key string, terms []knowledge.WeightedTerm)
}

// SearchTool 知识库搜索工具
// 核心功能：搜索知识库文档，返回匹配结果（路径、分数、摘要）
type SearchTool struct {
	knowledgeDir  string         // 知识库目录
	termExtractor TermExtractor  // 关键词提取器（可选）
	originalQuery string         // 用户原始问题（用于 chooseSearchKey）
	termCache     TermCache      // 关键词缓存（可选）
	retryMax      int            // 关键词提取的最大重试次数
}

// SearchToolOption 搜索工具选项（函数选项模式）
type SearchToolOption func(*SearchTool)

// WithOriginalQuery 设置原始查询
// 用于 chooseSearchKey 策略，当模型生成的查询是英文而原始问题是中文时，优先使用中文
func WithOriginalQuery(query string) SearchToolOption {
	return func(t *SearchTool) {
		t.originalQuery = query
	}
}

// WithTermCache 设置词项缓存
// 缓存已提取的关键词，同一搜索词在多次调用时避免重复 LLM 请求
func WithTermCache(cache TermCache) SearchToolOption {
	return func(t *SearchTool) {
		t.termCache = cache
	}
}

// WithRetryMax 设置关键词提取的重试次数
func WithRetryMax(max int) SearchToolOption {
	return func(t *SearchTool) {
		t.retryMax = max
	}
}

// NewSearchTool 创建知识库搜索工具
// 参数:
//   - knowledgeDir: 知识库目录路径
//   - extractor: 关键词提取器（可为 nil，此时使用基础搜索）
//   - opts: 可选配置（原始查询、缓存、重试次数）
func NewSearchTool(knowledgeDir string, extractor TermExtractor, opts ...SearchToolOption) *SearchTool {
	t := &SearchTool{
		knowledgeDir:  knowledgeDir,
		termExtractor: extractor,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Name 返回工具名称
func (t *SearchTool) Name() string {
	return "search_knowledge"
}

// Description 返回工具描述
func (t *SearchTool) Description() string {
	return "Search within all documentation content and return top matches with snippets. Use this first to find relevant docs by content."
}

// Parameters 返回参数定义
func (t *SearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Search query. Prefer short phrases or keywords."},
			"top_k": {"type": "integer", "description": "Number of results to return (1-20).", "minimum": 1, "maximum": 20}
		},
		"required": ["query"],
		"additionalProperties": false
	}`)
}

// Execute 执行搜索
func (t *SearchTool) Execute(ctx context.Context, args map[string]interface{}, emitStatus tools.StatusEmitter) (string, error) {
	query, _ := args["query"].(string)
	topK := 5 // 默认值
	if v, ok := args["top_k"]; ok {
		switch val := v.(type) {
		case int:
			topK = val
		case float64:
			topK = int(val)
		case int64:
			topK = int(val)
		}
	}
	if topK <= 0 {
		topK = 5
	}

	modelKey := strings.TrimSpace(query)
	originalKey := strings.TrimSpace(t.originalQuery)
	key := chooseSearchKey(originalKey, modelKey)

	if emitStatus != nil {
		if modelKey != "" && key != modelKey && modelKey != originalKey {
			emitStatus("searching", fmt.Sprintf("正在生成检索关键词（KEY: %s，ModelKey: %s）...", shortText(key, 90), shortText(modelKey, 60)))
		} else {
			emitStatus("searching", fmt.Sprintf("正在生成检索关键词（KEY: %s）...", shortText(key, 120)))
		}
	}

	// 获取或提取关键词
	var terms []knowledge.WeightedTerm
	if t.termCache != nil {
		terms = t.termCache.Get(key)
	}
	if len(terms) == 0 && t.termExtractor != nil {
		var extracted []knowledge.WeightedTerm
		var err error

		// 优先使用支持重试的接口
		if extractorWithRetry, ok := t.termExtractor.(TermExtractorWithRetry); ok && t.retryMax > 0 {
			extracted, err = extractorWithRetry.ExtractWeightedTermsWithRetry(ctx, key, t.retryMax)
		} else {
			extracted, err = t.termExtractor.ExtractWeightedTerms(ctx, key)
		}

		if err == nil && len(extracted) > 0 {
			terms = extracted
			if t.termCache != nil {
				t.termCache.Set(key, terms)
			}
		}
	}

	// 如果modelKey是英文且与key不同，添加到terms
	if modelKey != "" && modelKey != key && !containsHan(modelKey) {
		terms = append(terms, knowledge.WeightedTerm{Term: strings.ToLower(modelKey), Weight: 2})
	}

	// 格式化terms用于状态显示
	termsText := ""
	if len(terms) > 0 {
		termsText = formatWeightedTerms(terms, 6, 120)
	}

	if emitStatus != nil {
		if termsText != "" {
			emitStatus("searching", fmt.Sprintf("正在检索相关内容（KEY: %s，Terms: %s，TopK: %d）...", shortText(key, 80), termsText, topK))
		} else {
			emitStatus("searching", fmt.Sprintf("正在检索相关内容（KEY: %s，TopK: %d）...", shortText(key, 120), topK))
		}
	}

	// 执行搜索
	toolAt := time.Now()
	var hits []knowledge.SearchHit
	var err error
	if len(terms) > 0 {
		hits, err = knowledge.SearchWithTerms(t.knowledgeDir, key, terms, topK)
	} else {
		hits, err = knowledge.Search(t.knowledgeDir, key, topK)
	}
	toolCost := time.Since(toolAt)

	if err != nil {
		log.Printf("[SearchTool] cost=%s err=%v", toolCost, err)
		return "", fmt.Errorf("搜索失败: %w", err)
	}

	log.Printf("[SearchTool] cost=%s hits=%d", toolCost, len(hits))
	result, _ := json.Marshal(hits)
	return string(result), nil
}

// ===== 辅助函数 ===

// shortText 截断文本并清理换行符
// 用于状态消息显示，避免过长文本干扰UI
func shortText(s string, max int) string {
	t := strings.TrimSpace(s)
	t = strings.ReplaceAll(t, "\r", " ")
	t = strings.ReplaceAll(t, "\n", " ")
	if max <= 0 || len(t) <= max {
		return t
	}
	return t[:max] + "..."
}

// chooseSearchKey 选择最优搜索关键词
// 策略说明:
//   - 如果两个都为空，返回空字符串
//   - 如果其中一个为空，返回非空的那个
//   - 如果原始问题是中文，模型生成的是英文，优先使用中文（中文文档用中文搜更准确）
//   - 如果原始问题是英文，模型生成的是中文，使用中文（用户可能输入了中文）
//   - 如果两个都是中文或都是英文，使用较短的（模型提炼过的通常更精准）
func chooseSearchKey(original string, model string) string {
	o := strings.TrimSpace(original)
	m := strings.TrimSpace(model)
	if o == "" && m == "" {
		return ""
	}
	if m == "" {
		return o
	}
	if o == "" {
		return m
	}
	if containsHan(o) && !containsHan(m) {
		return o
	}
	if containsHan(m) && !containsHan(o) {
		return m
	}
	if len([]rune(m)) > 0 && len([]rune(m)) < len([]rune(o)) {
		return m
	}
	return o
}

// containsHan 检查字符串是否包含中文字符
func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// formatWeightedTerms 格式化关键词列表用于显示
// 参数:
//   - terms: 关键词列表
//   - maxItems: 最大显示数量
//   - maxChars: 最大字符数
func formatWeightedTerms(terms []knowledge.WeightedTerm, maxItems int, maxChars int) string {
	if len(terms) == 0 {
		return ""
	}
	// 复制并按权重排序
	cp := append([]knowledge.WeightedTerm(nil), terms...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].Weight == cp[j].Weight {
			return cp[i].Term < cp[j].Term
		}
		return cp[i].Weight > cp[j].Weight
	})
	// 限制数量
	if maxItems > 0 && len(cp) > maxItems {
		cp = cp[:maxItems]
	}
	// 格式化为 "term(weight), term(weight)..." 形式
	parts := make([]string, 0, len(cp))
	for _, t := range cp {
		term := strings.TrimSpace(t.Term)
		if term == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s(%.1f)", term, t.Weight))
	}
	out := strings.Join(parts, ", ")
	return shortText(out, maxChars)
}

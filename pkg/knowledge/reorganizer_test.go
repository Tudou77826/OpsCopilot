package knowledge

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/llm"
)

// Mock LLM 返回的 SOP 格式重组文档
const mockSOPResponse = `---
service: Payment Service
module: 核心支付模块
type: sop
---

# 支付系统排查手册

## 场景：API 接口超时

- **现象**: 请求超时
- **关键词**: timeout, 504
- **涉及组件**: Nginx, MySQL

<!-- META: {"resend_from_line": null} -->`

// Mock LLM 返回的 Archive 格式重组文档
const mockArchiveResponse = `---
service: Payment Service
module: 核心支付模块
type: archive
---

# 支付接口超时

## 问题现象

前端调用支付接口返回504

## 关键词

504, timeout

## 根本原因

MySQL慢查询

<!-- META: {"resend_from_line": null} -->`

func TestReorganize_SOPDocument(t *testing.T) {
	input := `# 支付相关

有时候API会超时，大概504错误。
涉及nginx和mysql。

排查方法：看日志`

	mock := &llm.MockProvider{Response: mockSOPResponse}
	reorganizer := NewLLMContentReorganizer(mock, nil)

	docs, err := reorganizer.Reorganize(context.Background(), input)
	if err != nil {
		t.Fatalf("Reorganize error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d, want 1", len(docs))
	}
	doc := docs[0]

	if doc.DocType != "sop" {
		t.Errorf("DocType = %q, want 'sop'", doc.DocType)
	}
	if doc.Service != "Payment Service" {
		t.Errorf("Service = %q, want 'Payment Service'", doc.Service)
	}
	if !strings.Contains(doc.Content, "## 场景：") {
		t.Error("Content should contain '## 场景：'")
	}
	if !strings.Contains(doc.Content, "---") {
		t.Error("Content should contain Front Matter delimiters")
	}
}

func TestReorganize_ArchiveDocument(t *testing.T) {
	input := `某次排查记录

前端说支付接口504了。
查了下是MySQL慢查询导致的。
解决方案：优化SQL。`

	mock := &llm.MockProvider{Response: mockArchiveResponse}
	reorganizer := NewLLMContentReorganizer(mock, nil)

	docs, err := reorganizer.Reorganize(context.Background(), input)
	if err != nil {
		t.Fatalf("Reorganize error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d, want 1", len(docs))
	}
	doc := docs[0]

	if doc.DocType != "archive" {
		t.Errorf("DocType = %q, want 'archive'", doc.DocType)
	}
	if !strings.Contains(doc.Content, "## 问题现象") {
		t.Error("Content should contain '## 问题现象'")
	}
}

func TestReorganize_UnstructuredDoc(t *testing.T) {
	input := `一些零散的运维笔记

某个服务有时候会出问题
重启可以暂时解决
但是不知道根本原因`

	mock := &llm.MockProvider{Response: mockSOPResponse}
	reorganizer := NewLLMContentReorganizer(mock, nil)

	docs, err := reorganizer.Reorganize(context.Background(), input)
	if err != nil {
		t.Fatalf("Reorganize error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d, want 1", len(docs))
	}
	doc := docs[0]

	if !strings.Contains(doc.Content, "---") {
		t.Error("Content should contain Front Matter delimiters")
	}
	if doc.Service == "" {
		t.Error("Service should not be empty")
	}
}

func TestReorganize_LLMError(t *testing.T) {
	input := `some content`

	mock := &llm.MockProvider{Err: fmt.Errorf("LLM unavailable")}
	reorganizer := NewLLMContentReorganizer(mock, nil)

	_, err := reorganizer.Reorganize(context.Background(), input)
	if err == nil {
		t.Error("expected error when LLM fails, got nil")
	}
}

func TestReorganize_NilProvider(t *testing.T) {
	reorganizer := NewLLMContentReorganizer(nil, nil)

	_, err := reorganizer.Reorganize(context.Background(), "some content")
	if err == nil {
		t.Error("expected error with nil provider, got nil")
	}
}

func TestReorganize_EmptyResponse(t *testing.T) {
	input := `some content`

	mock := &llm.MockProvider{Response: ""}
	reorganizer := NewLLMContentReorganizer(mock, nil)

	_, err := reorganizer.Reorganize(context.Background(), input)
	if err == nil {
		t.Error("expected error when LLM returns empty, got nil")
	}
}

func TestReorganize_AlreadyFormatted(t *testing.T) {
	input := `---
service: Payment Service
module: 核心支付模块
type: sop
---

# 支付系统排查手册

## 场景：API 接口超时

- **现象**: 请求超时`

	mock := &llm.MockProvider{Response: mockSOPResponse}
	reorganizer := NewLLMContentReorganizer(mock, nil)

	docs, err := reorganizer.Reorganize(context.Background(), input)
	if err != nil {
		t.Fatalf("Reorganize error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d, want 1", len(docs))
	}
	// Even formatted docs go through LLM reorganization
	if docs[0].Content == "" {
		t.Error("Content should not be empty")
	}
}

func TestParseReorganizerOutput(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantNoMeta     bool
		wantResendLine *int
	}{
		{
			name: "with META tag",
			input: `some markdown content

<!-- META: {"resend_from_line": null} -->`,
			wantNoMeta:     false,
			wantResendLine: nil,
		},
		{
			name: "with resend_from_line value",
			input: `some markdown content

<!-- META: {"resend_from_line": 42} -->`,
			wantNoMeta: false,
			wantResendLine: func() *int {
				v := 42
				return &v
			}(),
		},
		{
			name:  "without META tag",
			input: `plain markdown without meta`,
			wantNoMeta: true,
		},
		{
			name: "invalid JSON in META",
			input: `some content

<!-- META: {invalid json} -->`,
			wantNoMeta: true, // parse error falls back to raw output
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markdown, meta := parseReorganizerOutput(tt.input)

			if tt.wantNoMeta {
				// Without valid META, meta.ResendFromLine should be nil
				if meta.ResendFromLine != nil {
					t.Errorf("ResendFromLine should be nil, got %d", *meta.ResendFromLine)
				}
			} else {
				if tt.wantResendLine == nil {
					if meta.ResendFromLine != nil {
						t.Errorf("ResendFromLine should be nil, got %d", *meta.ResendFromLine)
					}
				} else {
					if meta.ResendFromLine == nil {
						t.Error("ResendFromLine should not be nil")
					} else if *meta.ResendFromLine != *tt.wantResendLine {
						t.Errorf("ResendFromLine = %d, want %d", *meta.ResendFromLine, *tt.wantResendLine)
					}
				}
			}

			if markdown == "" {
				t.Error("markdown should not be empty")
			}
		})
	}
}

func TestParseReorganizerOutput_MetaStripped(t *testing.T) {
	input := `---
service: Test
---

# Title

content here

<!-- META: {"resend_from_line": null} -->`

	markdown, meta := parseReorganizerOutput(input)

	if strings.Contains(markdown, "META:") {
		t.Error("markdown should not contain META tag")
	}
	if meta.ResendFromLine != nil {
		t.Errorf("ResendFromLine should be nil, got %d", *meta.ResendFromLine)
	}
	if !strings.Contains(markdown, "# Title") {
		t.Error("markdown should contain the original content")
	}
}

// TestReorganize_ResendFromLineOffByOne 验证 resend_from_line 的 1-based → 0-based 转换
func TestReorganize_ResendFromLineOffByOne(t *testing.T) {
	// 直接测试 parseReorganizerOutput 的 1-based 解析
	input := `markdown
<!-- META: {"resend_from_line": 3} -->`

	_, meta := parseReorganizerOutput(input)
	if meta.ResendFromLine == nil || *meta.ResendFromLine != 3 {
		t.Errorf("ResendFromLine = %v, want 3", meta.ResendFromLine)
	}
}

func TestTakeChunk_ResendFromLineBounds(t *testing.T) {
	// 验证 takeChunk 返回的 position/endLine 是 0-based
	lines := []string{"line1", "line2", "line3", "line4", "line5"}
	chunk, endLine := takeChunk(lines, 0, 100)

	_ = chunk
	// endLine 应该是 5（所有行都被取走，指向下一个位置）
	if endLine != 5 {
		t.Errorf("endLine = %d, want 5", endLine)
	}

	// 模拟 LLM 返回 resend_from_line=3（1-based）
	// 转换为 0-based: 3 - 1 = 2
	resendIdx := 3 - 1
	if resendIdx < 0 || resendIdx >= endLine {
		t.Errorf("resendIdx %d out of bounds [0, %d)", resendIdx, endLine)
	}
	// resendIdx=2 意味着下一次 takeChunk 从 lines[2] 开始（即 "line3"）
	chunk2, endLine2 := takeChunk(lines, resendIdx, 100)
	if !strings.Contains(chunk2, "line3") {
		t.Error("second chunk should contain 'line3' (the resend point)")
	}
	if endLine2 != 5 {
		t.Errorf("endLine2 = %d, want 5", endLine2)
	}
}

func TestTailContent(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		maxChars  int
		wantEmpty bool
		wantContains string
		wantNotContains string
	}{
		{
			name:      "short content returned as-is",
			content:   "short",
			maxChars:  100,
			wantContains: "short",
			wantNotContains: "...(前面内容已省略)",
		},
		{
			name:      "long content truncated with prefix",
			content:   strings.Repeat("abcdefghij", 100), // 1000 chars
			maxChars:  50,
			wantContains: "...(前面内容已省略)",
		},
		{
			name:      "empty content",
			content:   "",
			maxChars:  100,
			wantContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tailContent(tt.content, tt.maxChars)
			if tt.wantContains != "" && !strings.Contains(result, tt.wantContains) {
				t.Errorf("tailContent() should contain %q", tt.wantContains)
			}
			if tt.wantNotContains != "" && strings.Contains(result, tt.wantNotContains) {
				t.Errorf("tailContent() should NOT contain %q", tt.wantNotContains)
			}
		})
	}
}

func TestBuildReorganizeSubsequentPrompt_DoesNotExplode(t *testing.T) {
	// 模拟一个非常长的已重组文档
	longExisting := strings.Repeat("已重组内容行\n", 10000) // ~100K chars
	newChunk := "新的原始片段"

	prompt := buildReorganizeSubsequentPrompt(longExisting, newChunk, 2, 3)

	// prompt 不应该包含完整的 100K 已重组内容
	// 应该被截断到 ~subsequentContextMaxChars
	if len(prompt) > subsequentContextMaxChars+len(newChunk)+500 {
		t.Errorf("subsequent prompt too large: %d chars (max expected ~%d)",
			len(prompt), subsequentContextMaxChars+len(newChunk)+500)
	}
	// 应该包含省略提示
	if !strings.Contains(prompt, "...(前面内容已省略)") {
		t.Error("subsequent prompt should contain truncation hint")
	}
	// 应该包含新片段
	if !strings.Contains(prompt, newChunk) {
		t.Error("subsequent prompt should contain new chunk")
	}
}

func TestCalcTotalSegments(t *testing.T) {
	tests := []struct {
		name      string
		lines     []string
		maxChars  int
		wantSegs  int
	}{
		{
			name:     "empty content",
			lines:    []string{},
			maxChars: 100,
			wantSegs: 0,
		},
		{
			name:     "single short line fits in one chunk",
			lines:    []string{"hello"},
			maxChars: 100,
			wantSegs: 1,
		},
		{
			name:     "lines split into chunks",
			lines:    []string{"aaaa", "bbbb", "cccc", "dddd"},
			maxChars: 10, // "aaaa\nbbbb" = 9 chars fits, then "cccc\ndddd" = 9 chars fits => 2 segments
			wantSegs: 2,
		},
		{
			name:     "long lines need separate chunks",
			lines:    []string{"1234567890", "1234567890", "short"},
			maxChars: 12,
			wantSegs: 3,
		},
		{
			name:     "all lines fit in one chunk",
			lines:    []string{"a", "b", "c"},
			maxChars: 100,
			wantSegs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcTotalSegments(tt.lines, tt.maxChars)
			if got != tt.wantSegs {
				t.Errorf("calcTotalSegments() = %d, want %d", got, tt.wantSegs)
			}
		})
	}
}

func TestLLMCallTimeout_Is5Minutes(t *testing.T) {
	// 验证 LLM 调用超时时间为 5 分钟
	if llmCallTimeout != 5*time.Minute {
		t.Errorf("llmCallTimeout = %v, want 5m0s", llmCallTimeout)
	}
}

func TestReorganize_SingleRecordNoSplit(t *testing.T) {
	// 单记录文档即使有 moduleList 也不拆分
	input := `某次排查记录

前端说支付接口504了。
查了下是MySQL慢查询导致的。`

	mock := &llm.MockProvider{Response: mockArchiveResponse}
	moduleList := []ModuleConfigEntry{
		{Name: "核心支付模块", Description: "支付接口、退款"},
	}
	reorganizer := NewLLMContentReorganizer(mock, moduleList)

	docs, err := reorganizer.Reorganize(context.Background(), input)
	if err != nil {
		t.Fatalf("Reorganize error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d, want 1 (single record, no split)", len(docs))
	}
}

func TestReorganize_NoModuleList_StillTriesSplit(t *testing.T) {
	// 多记录文档即使没有 moduleList 也尝试拆分
	// 使用 sequenceMock 让 AnalyzeModules 返回"同一模块"，触发回退到 reorganizeSingle
	input := `## 故障记录 1
支付接口504

## 故障记录 2
订单超时`

	// 第1次调用 AnalyzeModules → 返回空数组（同一模块）→ 回退 reorganizeSingle
	// 第2次调用 reorganizeSingle → 返回有效的重组 Markdown
	mock := &sequenceMockProvider{
		responses: []string{
			`{"records":[]}`,      // AnalyzeModules: 同一模块，不拆分
			mockArchiveResponse,    // reorganizeSingle: 有效的重组输出
		},
	}
	reorganizer := NewLLMContentReorganizer(mock, nil)

	docs, err := reorganizer.Reorganize(context.Background(), input)
	if err != nil {
		t.Fatalf("Reorganize error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d, want 1 (fallback to single doc)", len(docs))
	}
	if docs[0].Content == "" {
		t.Error("Content should not be empty")
	}
}

// sequenceMockProvider 按顺序返回预设的响应
type sequenceMockProvider struct {
	responses []string
	index     int
}

func (m *sequenceMockProvider) ChatCompletion(ctx context.Context, messages []llm.ChatMessage) (string, error) {
	if m.index >= len(m.responses) {
		return "", fmt.Errorf("no more responses")
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func (m *sequenceMockProvider) ChatWithTools(ctx context.Context, messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
	content, err := m.ChatCompletion(ctx, messages)
	if err != nil {
		return nil, err
	}
	return &llm.ChatResponse{Content: content}, nil
}

func TestExtractRecordTitles(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantCount int
	}{
		{
			name:      "no headers",
			content:   "just some text\nno headers",
			wantCount: 0,
		},
		{
			name:      "one h2 header",
			content:   "# Title\n## 故障记录 1\nsome content",
			wantCount: 1,
		},
		{
			name:      "multiple h2 headers",
			content:   "# Title\n## 故障记录 1\ncontent1\n## 故障记录 2\ncontent2\n## 故障记录 3\ncontent3",
			wantCount: 3,
		},
		{
			name:      "h1 not counted",
			content:   "# Title\n## Only This\ncontent",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			titles := extractRecordTitles(tt.content)
			if len(titles) != tt.wantCount {
				t.Errorf("extractRecordTitles() = %d titles, want %d", len(titles), tt.wantCount)
			}
		})
	}
}

func TestGroupContentByModule_SameModuleMerged(t *testing.T) {
	// L1 fix: 同一模块的多条记录应合并为一个 group
	content := `# 排查手册

## 故障记录 1
支付接口504

## 故障记录 2
支付超时

## 故障记录 3
订单超时`

	titles := extractRecordTitles(content)
	records := []recordModule{
		{Title: "故障记录 1", Module: "核心支付模块"},
		{Title: "故障记录 2", Module: "核心支付模块"},
		{Title: "故障记录 3", Module: "订单模块"},
	}

	groups := groupContentByModule(content, titles, records)

	// 应该只有 2 个 group（核心支付模块 + 订单模块）
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}

	// 核心支付模块应包含两条记录的内容
	if groups[0].Module != "核心支付模块" {
		t.Errorf("groups[0].Module = %q, want '核心支付模块'", groups[0].Module)
	}
	if !strings.Contains(groups[0].Content, "故障记录 1") {
		t.Error("核心支付模块 group should contain '故障记录 1'")
	}
	if !strings.Contains(groups[0].Content, "故障记录 2") {
		t.Error("核心支付模块 group should contain '故障记录 2'")
	}

	// 订单模块
	if groups[1].Module != "订单模块" {
		t.Errorf("groups[1].Module = %q, want '订单模块'", groups[1].Module)
	}
	if !strings.Contains(groups[1].Content, "故障记录 3") {
		t.Error("订单模块 group should contain '故障记录 3'")
	}
}

func TestGroupContentByModule_PreamblePreserved(t *testing.T) {
	// L2 fix: ## 之前的 preamble 应保留在每个 group 中
	content := `---
service: Payment Service
---

# 支付系统排查手册

## 故障记录 1
支付接口504

## 故障记录 2
订单超时`

	titles := extractRecordTitles(content)
	records := []recordModule{
		{Title: "故障记录 1", Module: "核心支付模块"},
		{Title: "故障记录 2", Module: "订单模块"},
	}

	groups := groupContentByModule(content, titles, records)

	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}

	// 每个 group 都应包含 preamble（Front Matter 和 # 标题）
	for i, g := range groups {
		if !strings.Contains(g.Content, "service: Payment Service") {
			t.Errorf("groups[%d] should contain Front Matter preamble", i)
		}
		if !strings.Contains(g.Content, "# 支付系统排查手册") {
			t.Errorf("groups[%d] should contain h1 title preamble", i)
		}
	}
}

func TestGroupContentByModule_UnassignedTitle(t *testing.T) {
	// L4 fix: LLM 未分配模块的标题应归入"默认模块"
	content := `## 记录 A
内容A

## 记录 B
内容B

## 记录 C
内容C`

	titles := extractRecordTitles(content)
	records := []recordModule{
		{Title: "记录 A", Module: "模块1"},
		// 记录 B 和 C 未被 LLM 分配（截断导致）
	}

	groups := groupContentByModule(content, titles, records)

	// 应该有 2 个 group：模块1 + 默认模块
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}

	// 找到默认模块的 group
	var defaultGroup *RecordGroup
	for _, g := range groups {
		if g.Module == "默认模块" {
			defaultGroup = g
			break
		}
	}
	if defaultGroup == nil {
		t.Fatal("should have a '默认模块' group")
	}
	if !strings.Contains(defaultGroup.Content, "记录 B") {
		t.Error("默认模块 group should contain '记录 B'")
	}
	if !strings.Contains(defaultGroup.Content, "记录 C") {
		t.Error("默认模块 group should contain '记录 C'")
	}
}

func TestTruncateForModuleAnalysis_AllTitlesPreserved(t *testing.T) {
	// L4 fix: 截断后所有 ## 标题仍应可见
	// 生成一个长文档，包含多个 ## 段落
	var lines []string
	lines = append(lines, "# 排查手册")
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("## 故障记录 %d", i))
		// 每段添加大量正文内容
		for j := 0; j < 200; j++ {
			lines = append(lines, fmt.Sprintf("这是第 %d 段的内容行 %d，包含一些有意义的运维排查信息。", i, j))
		}
	}
	content := strings.Join(lines, "\n")

	truncated := truncateForModuleAnalysis(content, 5000)

	// 所有 20 个标题都应出现
	for i := 1; i <= 20; i++ {
		title := fmt.Sprintf("## 故障记录 %d", i)
		if !strings.Contains(truncated, title) {
			t.Errorf("truncated content should contain %q", title)
		}
	}
}

func TestTruncateForModuleAnalysis_PreambleLimited(t *testing.T) {
	// L5 fix: 前言内容应被限制
	var preamble []string
	preamble = append(preamble, "# 标题")
	for i := 0; i < 100; i++ {
		preamble = append(preamble, fmt.Sprintf("前言内容行 %d，这是一段很长的介绍文字。", i))
	}
	preamble = append(preamble, "## 第一节")
	preamble = append(preamble, "内容1")
	content := strings.Join(preamble, "\n")

	truncated := truncateForModuleAnalysis(content, 10000)

	// 应包含标题和 ## 第一节
	if !strings.Contains(truncated, "# 标题") {
		t.Error("truncated should contain h1 title")
	}
	if !strings.Contains(truncated, "## 第一节") {
		t.Error("truncated should contain '## 第一节'")
	}
	// 前言应被截断
	if strings.Contains(truncated, "前言内容行 80") {
		t.Error("preamble should be truncated, should not contain line 80")
	}
}

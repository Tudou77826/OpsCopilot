package knowledge

import (
	"context"
	"fmt"
	"strings"
	"testing"

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
	reorganizer := NewLLMContentReorganizer(mock)

	doc, err := reorganizer.Reorganize(context.Background(), input)
	if err != nil {
		t.Fatalf("Reorganize error: %v", err)
	}

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
	reorganizer := NewLLMContentReorganizer(mock)

	doc, err := reorganizer.Reorganize(context.Background(), input)
	if err != nil {
		t.Fatalf("Reorganize error: %v", err)
	}

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
	reorganizer := NewLLMContentReorganizer(mock)

	doc, err := reorganizer.Reorganize(context.Background(), input)
	if err != nil {
		t.Fatalf("Reorganize error: %v", err)
	}

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
	reorganizer := NewLLMContentReorganizer(mock)

	_, err := reorganizer.Reorganize(context.Background(), input)
	if err == nil {
		t.Error("expected error when LLM fails, got nil")
	}
}

func TestReorganize_NilProvider(t *testing.T) {
	reorganizer := NewLLMContentReorganizer(nil)

	_, err := reorganizer.Reorganize(context.Background(), "some content")
	if err == nil {
		t.Error("expected error with nil provider, got nil")
	}
}

func TestReorganize_EmptyResponse(t *testing.T) {
	input := `some content`

	mock := &llm.MockProvider{Response: ""}
	reorganizer := NewLLMContentReorganizer(mock)

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
	reorganizer := NewLLMContentReorganizer(mock)

	doc, err := reorganizer.Reorganize(context.Background(), input)
	if err != nil {
		t.Fatalf("Reorganize error: %v", err)
	}
	// Even formatted docs go through LLM reorganization
	if doc.Content == "" {
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

package ai

import (
	"strings"
	"testing"
)

func TestNormalizeAgentResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "正常 Markdown 直接返回",
			input: "## 问题分析\n\n这是分析内容。",
			want:  "## 问题分析\n\n这是分析内容。",
		},
		{
			name:  "JSON 包装 summary",
			input: `{"summary": "## 问题分析\n\n这是内容。"}`,
			want:  "## 问题分析\n\n这是内容。",
		},
		{
			name:  "JSON 包装 content",
			input: `{"content": "直接内容"}`,
			want:  "直接内容",
		},
		{
			name:  "孤立的代码块标记-奇数",
			input: "```\ncode\n```\n\n额外内容\n```",
			want:  "```\ncode\n```\n\n额外内容",
		},
		{
			name:  "偶数代码块标记不处理",
			input: "```\ncode\n```",
			want:  "```\ncode\n```",
		},
		{
			name:  "前后空白被清理",
			input: "  \n  内容  \n  ",
			want:  "内容",
		},
		{
			name:  "空输入",
			input: "",
			want:  "",
		},
		{
			name:  "JSON 但不是 summary/content",
			input: `{"error": "something"}`,
			want:  `{"error": "something"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeAgentResponse(tt.input)
			if strings.TrimSpace(got) != strings.TrimSpace(tt.want) {
				t.Errorf("normalizeAgentResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

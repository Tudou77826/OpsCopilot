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
			name:  "空输入",
			input: "",
			want:  `{"steps":[],"commands":[]}`,
		},
		{
			name:  "纯文本被包装成JSON",
			input: "知识库中暂无相关排障文档。建议检查基本网络连通性。",
			want:  `{"summary":"知识库中暂无相关排障文档。建议检查基本网络连通性。","steps":[],"commands":[]}`,
		},
		{
			name:  "纯Markdown被包装成JSON",
			input: "## 问题分析\n\n这是分析内容。",
			want:  `{"summary":"## 问题分析\n\n这是分析内容。","steps":[],"commands":[]}`,
		},
		{
			name:  "JSON包装summary提取",
			input: `{"summary": "## 问题分析\n\n这是内容。"}`,
			want:  `{"summary":"## 问题分析\n\n这是内容。","steps":[],"commands":[]}`,
		},
		{
			name:  "JSON包装content提取",
			input: `{"content": "直接内容"}`,
			want:  `{"summary":"直接内容","steps":[],"commands":[]}`,
		},
		{
			name:  "有效troubleshoot JSON保持不变",
			input: `{"steps":[{"step":1,"title":"检查服务状态"}],"commands":[{"command":"systemctl status nginx","description":"检查Nginx状态","risk":"Low"}]}`,
			want:  `{"steps":[{"step":1,"title":"检查服务状态"}],"commands":[{"command":"systemctl status nginx","description":"检查Nginx状态","risk":"Low"}]}`,
		},
		{
			name:  "JSON被markdown代码块包裹",
			input: "下面是分析：\n```json\n{\"steps\":[],\"commands\":[]}\n```\n",
			want:  `{"steps":[],"commands":[]}`,
		},
		{
			name:  "JSON前后有额外文字",
			input: "Based on analysis:\n{\"steps\":[{\"step\":1,\"title\":\"检查\"}],\"commands\":[]}\nHope this helps!",
			want:  `{"steps":[{"step":1,"title":"检查"}],"commands":[]}`,
		},
		{
			name:  "JSON但不含steps/commands/summary被包装",
			input: `{"error": "something"}`,
			want:  `{"summary":"{\"error\": \"something\"}","steps":[],"commands":[]}`,
		},
		{
			name:  "偶数代码块标记-非JSON内容被包装",
			input: "```\ncode\n```",
			want:  "{\"summary\":\"```\\ncode\\n```\",\"steps\":[],\"commands\":[]}",
		},
		{
			name:  "前后空白被清理后包装",
			input: "  \n  内容  \n  ",
			want:  `{"summary":"内容","steps":[],"commands":[]}`,
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

package knowledge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCleanJSONResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantKey string // 期望 JSON 中包含的 key 或 value 片段
	}{
		{
			name:    "plain JSON object",
			input:   `{"service":"Payment","module":"核心支付模块"}`,
			wantKey: `"service"`,
		},
		{
			name:    "JSON wrapped in ```json block",
			input:   "```json\n{\"service\":\"Payment\"}\n```",
			wantKey: `"service"`,
		},
		{
			name:    "JSON wrapped in ``` block (no lang tag)",
			input:   "```\n{\"service\":\"Payment\"}\n```",
			wantKey: `"service"`,
		},
		{
			name:    "JSON with leading/trailing whitespace",
			input:   "  \n  {\"service\":\"Payment\"}  \n  ",
			wantKey: `"service"`,
		},
		{
			name:    "JSON with explanation text before",
			input:   "根据分析，结果如下：\n\n{\"service\":\"Payment\"}",
			wantKey: `"service"`,
		},
		{
			name:    "JSON with explanation text after",
			input:   "{\"service\":\"Payment\"}\n\n以上供参考。",
			wantKey: `"service"`,
		},
		{
			name:    "JSON with explanation before and after",
			input:   "以下是提取结果：\n```json\n{\"service\":\"Payment\"}\n```\n请查收。",
			wantKey: `"service"`,
		},
		{
			name:    "JSON with uppercase JSON in code block",
			input:   "```JSON\n{\"service\":\"Payment\"}\n```",
			wantKey: `"service"`,
		},
		{
			name:    "nested JSON arrays",
			input:   `{"records":[{"title":"故障1","module":"支付模块"}]}`,
			wantKey: `"records"`,
		},
		{
			name:    "empty JSON object",
			input:   `{}`,
			wantKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := cleanJSONResponse(tt.input)

			// 验证清理后的结果是合法 JSON
			if !json.Valid([]byte(cleaned)) {
				t.Errorf("cleanJSONResponse() produced invalid JSON:\n%s", cleaned)
				return
			}

			if tt.wantKey != "" && !strings.Contains(cleaned, tt.wantKey) {
				t.Errorf("cleanJSONResponse() = %q, want to contain %q", cleaned, tt.wantKey)
			}
		})
	}
}

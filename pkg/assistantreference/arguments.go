package assistantreference

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

type Arguments struct {
	Query   string `json:"query,omitempty"`
	Service string `json:"service,omitempty"`
	Module  string `json:"module,omitempty"`
	ID      string `json:"id,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Group   string `json:"group,omitempty"`
	Offset  int    `json:"offset,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

var referenceFields = map[string][]string{
	"knowledge_list":     {"service", "module", "offset", "limit"},
	"knowledge_search":   {"query", "limit"},
	"knowledge_read":     {"id"},
	"quick_command_list": {"kind", "group", "query", "offset", "limit"},
	"quick_command_read": {"kind", "id"},
}

func ToolNames() []string {
	return []string{"knowledge_list", "knowledge_search", "knowledge_read", "quick_command_list", "quick_command_read"}
}

// ParseArguments is shared by the transport and application boundary.
// In particular, no caller-supplied filesystem path or execution flag is accepted.
func ParseArguments(name, argument string) (Arguments, error) {
	var in Arguments
	allowed, ok := referenceFields[name]
	if !ok {
		return in, errors.New("未知的参考资料工具")
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(argument), &raw) != nil || raw == nil {
		return in, errors.New("参数必须是对象")
	}
	for key, value := range raw {
		found := false
		for _, field := range allowed {
			if key == field {
				found = true
				break
			}
		}
		if !found || string(value) == "null" {
			return in, errors.New("不支持的参考资料参数")
		}
	}
	if json.Unmarshal([]byte(argument), &in) != nil {
		return in, errors.New("参考资料参数类型无效")
	}
	for _, value := range []string{in.Query, in.Service, in.Module, in.ID, in.Kind, in.Group} {
		if len(value) > 512 || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") {
			return in, errors.New("参考资料参数过长或包含控制字符")
		}
	}
	if in.Offset < 0 || in.Offset > 1000000 || in.Limit < 0 || in.Limit > 50 {
		return in, errors.New("分页参数无效（limit 为 1–50）")
	}
	if _, provided := raw["limit"]; provided && in.Limit == 0 {
		return in, errors.New("limit 为 1–50")
	}
	if in.Limit == 0 {
		in.Limit = 25
		if name == "knowledge_search" {
			in.Limit = 5
		}
	}
	if in.Kind != "" && in.Kind != "terminal" && in.Kind != "assistant" {
		return in, errors.New("kind 必须是 terminal 或 assistant")
	}
	if name == "knowledge_search" && strings.TrimSpace(in.Query) == "" {
		return in, errors.New("请提供检索词")
	}
	if (name == "knowledge_read" || name == "quick_command_read") && strings.TrimSpace(in.ID) == "" {
		return in, errors.New("请提供列表或搜索返回的 ID")
	}
	if name == "quick_command_read" && in.Kind == "" {
		return in, errors.New("请指定命令类型")
	}
	return in, nil
}

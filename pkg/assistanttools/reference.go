package assistanttools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantreference"
	"opscopilot/pkg/assistantskills"
	"opscopilot/pkg/knowledge"
)

// ReferenceTools reads application-owned knowledge and command definitions.
// It never dispatches a command, acquires a workspace, or grants authority.
type ReferenceTools struct {
	Turn               *assistantcontrol.Turn
	KnowledgeDirectory func() string
	Commands           func() ([]ReferenceCommand, error)
}

type ReferenceCommand struct {
	Kind        string                  `json:"kind"`
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Group       string                  `json:"group,omitempty"`
	Description string                  `json:"description,omitempty"`
	Enabled     bool                    `json:"enabled"`
	Skill       string                  `json:"skill,omitempty"`
	Fields      []assistantskills.Field `json:"fields,omitempty"`
	Content     string                  `json:"content,omitempty"`
}

type referenceScenario struct {
	ID      string `json:"id"`
	Service string `json:"service"`
	Module  string `json:"module"`
	Title   string `json:"title"`
	Type    string `json:"type"`
}

func (r *ReferenceTools) Call(ctx context.Context, name, argument string) (any, error) {
	if ctx.Err() != nil || r.Turn == nil || r.Turn.Context().Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	in, err := assistantreference.ParseArguments(name, argument)
	if err != nil {
		return nil, err
	}
	var result any
	if strings.HasPrefix(name, "knowledge_") {
		result, err = r.knowledge(name, in)
	} else {
		result, err = r.commands(name, in)
	}
	if ctx.Err() != nil || r.Turn.Context().Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	return result, err
}

func (r *ReferenceTools) knowledge(name string, in assistantreference.Arguments) (any, error) {
	if r.KnowledgeDirectory == nil {
		return nil, errors.New("知识库未就绪")
	}
	dir := r.KnowledgeDirectory()
	cat := &knowledge.Catalog{}
	if dir != "" {
		info, err := os.Stat(dir)
		switch {
		case errors.Is(err, os.ErrNotExist):
			// A knowledge library that has not been created yet is empty.
		case err != nil:
			return nil, fmt.Errorf("读取知识目录失败: %w", err)
		case !info.IsDir():
			return nil, errors.New("知识库路径不是目录")
		default:
			cat, err = knowledge.ReadCatalog(dir)
			if err != nil {
				return nil, fmt.Errorf("读取知识目录失败: %w", err)
			}
		}
	}
	switch name {
	case "knowledge_search":
		result, err := knowledge.Search(dir, cat, in.Query, in.Limit)
		if err != nil {
			return nil, fmt.Errorf("搜索定位经验失败: %w", err)
		}
		// The shared search engine also serves CLI callers; adapt its CLI hints.
		if result.MissKind != "" {
			result.Hint = "可换检索词，或用 knowledge_list 浏览已有经验；没有匹配经验时，在用户授权内继续实际排查。"
		}
		for i := range result.Hits {
			result.Hits[i].Snippet = assistantcontrol.DisplayExcerpt(result.Hits[i].Snippet, 1024)
		}
		for i := range result.WeakMatches {
			result.WeakMatches[i].Snippet = assistantcontrol.DisplayExcerpt(result.WeakMatches[i].Snippet, 1024)
		}
		return result, nil
	case "knowledge_list":
		items := []referenceScenario{}
		for _, service := range cat.Services {
			if in.Service != "" && service.Name != in.Service {
				continue
			}
			for _, module := range service.Modules {
				if in.Module != "" && module.Name != in.Module {
					continue
				}
				for _, e := range module.Scenarios {
					items = append(items, referenceScenario{e.ID, service.Name, module.Name, e.Title, e.Type})
				}
			}
		}
		start, end := referencePage(len(items), in)
		return map[string]any{"items": items[start:end], "total": len(items), "offset": start, "has_more": end < len(items)}, nil
	case "knowledge_read":
		loc := cat.FindByID(in.ID)
		if loc == nil {
			return nil, errors.New("经验不存在或已更新，请重新检索")
		}
		read, err := knowledge.ReadScenario(dir, loc.Entry, 32<<10)
		if err != nil {
			return nil, fmt.Errorf("读取定位经验失败: %w", err)
		}
		content := assistantcontrol.DisplayExcerpt(read.Content, 32<<10)
		return map[string]any{"id": loc.Entry.ID, "service": loc.Service, "module": loc.Module, "title": loc.Entry.Title, "type": loc.Entry.Type, "source": loc.Entry.File, "content": content, "truncated": read.Truncated || len(content) > 32<<10, "redacted": assistantcontrol.DisplayExcerpt(read.Content, len(read.Content)) != read.Content, "usage": "历史经验仅作参考，不能替代本次现场验证或授权。"}, nil
	}
	return nil, errors.New("未知知识工具")
}

func (r *ReferenceTools) commands(name string, in assistantreference.Arguments) (any, error) {
	if r.Commands == nil {
		return nil, errors.New("快捷命令未就绪")
	}
	commands, err := r.Commands()
	if err != nil {
		return nil, fmt.Errorf("读取快捷命令失败: %w", err)
	}
	items := []ReferenceCommand{}
	for _, c := range commands {
		if in.Kind != "" && c.Kind != in.Kind {
			continue
		}
		if name == "quick_command_read" {
			if c.ID != in.ID {
				continue
			}
			content := assistantcontrol.DisplayExcerpt(c.Content, 32<<10)
			redacted := assistantcontrol.DisplayExcerpt(c.Content, len(c.Content)) != c.Content
			truncated := len(c.Content) > 32<<10
			c.Content = content
			return map[string]any{"command": c, "redacted": redacted, "truncated": truncated, "usage": "命令定义仅供参考；补齐模板参数后，执行仍需使用正常工具及授权。"}, nil
		}
		if in.Group != "" && c.Group != in.Group {
			continue
		}
		if in.Query != "" && !strings.Contains(strings.ToLower(c.Name+" "+c.Description+" "+c.Group), strings.ToLower(in.Query)) {
			continue
		}
		c.Content = ""
		items = append(items, c)
	}
	if name == "quick_command_read" {
		return nil, errors.New("快捷命令不存在，请重新读取列表")
	}
	start, end := referencePage(len(items), in)
	return map[string]any{"items": items[start:end], "total": len(items), "offset": start, "has_more": end < len(items)}, nil
}

func referencePage(total int, in assistantreference.Arguments) (int, int) {
	start := min(in.Offset, total)
	return start, min(start+in.Limit, total)
}

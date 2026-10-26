package opscli

import (
	"flag"
	"fmt"
	"os"

	"opscopilot/pkg/assistantcontrol"
	kb "opscopilot/pkg/knowledge"
)

const knowledgeReadLimit = 32 * 1024

// knowledge implements the read-only knowledge primitives used by the server CLI.
// It deliberately returns values instead of printing them; the host decides how
// to encode and present the result.
func knowledge(args []string, directory string) (any, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("knowledge 子命令必填: list、search 或 read")
	}
	options, err := parseKnowledgeOptions(args)
	if err != nil {
		return nil, err
	}
	if directory == "" {
		return nil, fmt.Errorf("请配置共享知识目录 OPSCOPILOT_KNOWLEDGE_DIR")
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, fmt.Errorf("加载知识库失败: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("加载知识库失败: 路径不是目录: %s", directory)
	}

	cat, err := kb.ReadCatalog(directory)
	if err != nil {
		return nil, fmt.Errorf("加载知识库失败: %w", err)
	}

	switch args[0] {
	case "list":
		return knowledgeList(options, cat)
	case "search":
		return kb.Search(directory, cat, options.query, options.top)
	case "read":
		return knowledgeRead(options.id, directory, cat)
	default:
		return nil, fmt.Errorf("未知 knowledge 子命令: %s", args[0])
	}
}

type knowledgeOptions struct {
	service, module, query, id string
	top                        int
}

func parseKnowledgeOptions(args []string) (knowledgeOptions, error) {
	var options knowledgeOptions
	err := flags("knowledge "+args[0], args[1:], func(fs *flag.FlagSet) {
		switch args[0] {
		case "list":
			fs.StringVar(&options.service, "service", "", "服务名")
			fs.StringVar(&options.module, "module", "", "模块名")
		case "search":
			fs.StringVar(&options.query, "query", "", "症状/关键词")
			fs.IntVar(&options.top, "top", 5, "返回命中条数上限")
		case "read":
			fs.StringVar(&options.id, "id", "", "场景短 ID")
		}
	})
	if err != nil {
		return options, err
	}
	switch args[0] {
	case "list":
		if (options.service == "") != (options.module == "") {
			return options, fmt.Errorf("--service 与 --module 必须同时提供")
		}
	case "search":
		return options, requireOptions("--query", options.query)
	case "read":
		return options, requireOptions("--id", options.id)
	default:
		return options, fmt.Errorf("未知 knowledge 子命令")
	}
	return options, nil
}

func knowledgeList(options knowledgeOptions, cat *kb.Catalog) (any, error) {
	if options.service == "" {
		type moduleSummary struct {
			Name          string `json:"name"`
			ScenarioCount int    `json:"scenario_count"`
		}
		type serviceSummary struct {
			Name    string          `json:"name"`
			Modules []moduleSummary `json:"modules"`
		}
		services := make([]serviceSummary, 0, len(cat.Services))
		for _, svc := range cat.Services {
			summary := serviceSummary{Name: svc.Name, Modules: make([]moduleSummary, 0, len(svc.Modules))}
			for _, mod := range svc.Modules {
				summary.Modules = append(summary.Modules, moduleSummary{Name: mod.Name, ScenarioCount: len(mod.Scenarios)})
			}
			services = append(services, summary)
		}
		return map[string]any{"services": services}, nil
	}

	for _, svc := range cat.Services {
		if svc.Name != options.service {
			continue
		}
		for _, mod := range svc.Modules {
			if mod.Name != options.module {
				continue
			}
			type scenarioBrief struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			}
			scenarios := make([]scenarioBrief, 0, len(mod.Scenarios))
			for _, entry := range mod.Scenarios {
				scenarios = append(scenarios, scenarioBrief{ID: entry.ID, Title: entry.Title})
			}
			return map[string]any{"service": svc.Name, "module": mod.Name, "scenarios": scenarios}, nil
		}
		return nil, fmt.Errorf("服务 %s 下不存在模块 %s", options.service, options.module)
	}
	return nil, fmt.Errorf("不存在服务 %s", options.service)
}

func knowledgeRead(id, directory string, cat *kb.Catalog) (any, error) {
	loc := cat.FindByID(id)
	if loc == nil {
		return nil, fmt.Errorf("场景 %s 不存在", id)
	}
	result, err := kb.ReadScenario(directory, loc.Entry, knowledgeReadLimit)
	if err != nil {
		return nil, fmt.Errorf("读取场景失败: %w", err)
	}

	type readOutput struct {
		ID        string `json:"id"`
		Service   string `json:"service"`
		Module    string `json:"module"`
		Title     string `json:"title"`
		Type      string `json:"type"`
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
	}
	return readOutput{
		ID: loc.Entry.ID, Service: loc.Service, Module: loc.Module,
		Title: loc.Entry.Title, Type: loc.Entry.Type,
		Content:   assistantcontrol.DisplayExcerpt(result.Content, knowledgeReadLimit),
		Truncated: result.Truncated,
	}, nil
}

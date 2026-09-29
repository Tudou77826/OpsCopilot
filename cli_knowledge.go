package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"opscopilot/pkg/knowledge"
)

// cmdKnowledge: opscopilot knowledge list|search|read
// 知识库原语接口：单发调用、不依赖服务器登记、不依赖 LLM 配置。
// 场景是唯一寻址单元（短 ID），不暴露文件路径与行号。
func cmdKnowledge(args []string) (exitCode int) {
	if !cliHelpRequested(args) && len(args) > 0 && (args[0] == "list" || args[0] == "search" || args[0] == "read") {
		done, _ := beginCLIUsage(loadCLIEnv(), "cli_knowledge_"+args[0])
		defer func() {
			if exitCode == 0 {
				done("success")
			} else {
				done("failure")
			}
		}()
	}
	if len(args) < 1 {
		knowledgeUsage()
		return 1
	}
	switch args[0] {
	case "list":
		return cmdKnowledgeList(args[1:])
	case "search":
		return cmdKnowledgeSearch(args[1:])
	case "read":
		return cmdKnowledgeRead(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: knowledge %s\n", args[0])
		knowledgeUsage()
		return 1
	}
}

func knowledgeUsage() {
	fmt.Fprintln(os.Stderr, `用法:
  opscopilot knowledge list                                  # 服务→模块树（含场景数）
  opscopilot knowledge list --service S --module M           # 该模块下场景列表 [{id,title}]
  opscopilot knowledge search --query "<症状/关键词>" [--top N]
  opscopilot knowledge read --id <场景ID>`)
}

// loadKnowledgeCatalog 构建知识库目录。目录不存在时返回空目录（不报错）：
// 知识库为空是合法状态，接口应返回空集与提示而不是失败。
func loadKnowledgeCatalog(env cliEnv) *knowledge.Catalog {
	if _, err := os.Stat(env.knowledgeDir); err != nil {
		return &knowledge.Catalog{}
	}
	cat, err := knowledge.BuildCatalog(env.knowledgeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构建知识库目录失败: %v\n", err)
		return &knowledge.Catalog{}
	}
	return cat
}

func cmdKnowledgeList(args []string) int {
	fs := flag.NewFlagSet("knowledge list", flag.ExitOnError)
	service := fs.String("service", "", "服务名")
	module := fs.String("module", "", "模块名")
	fs.Parse(args)

	cat := loadKnowledgeCatalog(loadCLIEnv())

	// 无参数：返回服务→模块树
	if *service == "" && *module == "" {
		type moduleSummary struct {
			Name          string `json:"name"`
			ScenarioCount int    `json:"scenario_count"`
		}
		type serviceSummary struct {
			Name    string          `json:"name"`
			Modules []moduleSummary `json:"modules"`
		}
		out := make([]serviceSummary, 0, len(cat.Services))
		for _, svc := range cat.Services {
			s := serviceSummary{Name: svc.Name, Modules: make([]moduleSummary, 0, len(svc.Modules))}
			for _, mod := range svc.Modules {
				s.Modules = append(s.Modules, moduleSummary{Name: mod.Name, ScenarioCount: len(mod.Scenarios)})
			}
			out = append(out, s)
		}
		printJSON(map[string]interface{}{"services": out})
		return 0
	}

	// 带 service+module：返回该模块的场景列表
	if *service == "" || *module == "" {
		fmt.Fprintln(os.Stderr, "错误: --service 与 --module 必须同时提供")
		fs.Usage()
		return 1
	}
	for i := range cat.Services {
		svc := &cat.Services[i]
		if svc.Name != *service {
			continue
		}
		for j := range svc.Modules {
			mod := &svc.Modules[j]
			if mod.Name != *module {
				continue
			}
			type scenarioBrief struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			}
			list := make([]scenarioBrief, 0, len(mod.Scenarios))
			for _, e := range mod.Scenarios {
				list = append(list, scenarioBrief{ID: e.ID, Title: e.Title})
			}
			printJSON(map[string]interface{}{
				"service":   svc.Name,
				"module":    mod.Name,
				"scenarios": list,
			})
			return 0
		}
		// 服务存在但模块不存在：列出该服务的模块帮助纠正
		var mods []string
		for _, m := range svc.Modules {
			mods = append(mods, m.Name)
		}
		fmt.Fprintf(os.Stderr, "错误: 服务 %s 下不存在模块 %s。可用模块: %s\n", *service, *module, strings.Join(mods, ", "))
		return 1
	}
	var services []string
	for _, s := range cat.Services {
		services = append(services, s.Name)
	}
	if len(services) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 知识库为空")
		return 1
	}
	fmt.Fprintf(os.Stderr, "错误: 不存在服务 %s。可用服务: %s\n", *service, strings.Join(services, ", "))
	return 1
}

func cmdKnowledgeSearch(args []string) int {
	fs := flag.NewFlagSet("knowledge search", flag.ExitOnError)
	query := fs.String("query", "", "症状/关键词（必填）")
	top := fs.Int("top", 5, "返回命中条数上限")
	fs.Parse(args)

	if strings.TrimSpace(*query) == "" {
		fmt.Fprintln(os.Stderr, "错误: --query 必填")
		fs.Usage()
		return 1
	}

	env := loadCLIEnv()
	cat := loadKnowledgeCatalog(env)
	res, err := knowledge.Search(env.knowledgeDir, cat, *query, *top)
	if err != nil {
		fmt.Fprintf(os.Stderr, "检索失败: %v\n", err)
		return 1
	}
	printJSON(res)
	return 0
}

func cmdKnowledgeRead(args []string) int {
	fs := flag.NewFlagSet("knowledge read", flag.ExitOnError)
	id := fs.String("id", "", "场景短 ID（必填，来自 list/search 输出）")
	fs.Parse(args)

	if strings.TrimSpace(*id) == "" {
		fmt.Fprintln(os.Stderr, "错误: --id 必填")
		fs.Usage()
		return 1
	}

	env := loadCLIEnv()
	cat := loadKnowledgeCatalog(env)
	loc := cat.FindByID(*id)
	if loc == nil {
		fmt.Fprintf(os.Stderr, "错误: 场景 %s 不存在。可先用 knowledge list 或 search 获取有效 ID\n", *id)
		return 1
	}
	r, err := knowledge.ReadScenario(env.knowledgeDir, loc.Entry, 32*1024)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取失败: %v\n", err)
		return 1
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
	printJSON(readOutput{
		ID:        loc.Entry.ID,
		Service:   loc.Service,
		Module:    loc.Module,
		Title:     loc.Entry.Title,
		Type:      loc.Entry.Type,
		Content:   r.Content,
		Truncated: r.Truncated,
	})
	return 0
}

// printJSON 统一 JSON 输出：stdout、禁用 HTML 转义（中文原样）
func printJSON(v interface{}) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "序列化输出失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(buf.String())
}

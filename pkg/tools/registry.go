package tools

import (
	"opscopilot/pkg/llm"
	"sync"
)

// Registry 工具注册器
// 线程安全的工具集合管理器，支持注册、获取、列出和转换为LLM工具格式
//
// 使用示例:
//
//	registry := NewRegistry()
//	registry.Register(tool1)
//	registry.Register(tool2)
//
//	// 获取单个工具
//	if tool, ok := registry.Get("tool1"); ok {
//	    tool.Execute(ctx, args, emitStatus)
//	}
//
//	// 转换为LLM工具列表
//	llmTools := registry.ToLLMTools()
type Registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewRegistry 创建新的工具注册器
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具
// 如果同名工具已存在，将被覆盖
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

// Get 获取工具
// 返回工具和是否存在的标志
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List 列出所有已注册的工具
// 返回工具列表的副本（避免外部修改影响内部状态）
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// ToLLMTools 将所有注册的工具转换为LLM工具格式
// 返回的列表可直接用于 provider.ChatWithTools 的 tools 参数
func (r *Registry) ToLLMTools() []llm.Tool {
	list := r.List()
	result := make([]llm.Tool, len(list))
	for i, t := range list {
		result[i] = ToLLMTool(t)
	}
	return result
}

// ToLLMTool 将单个工具转换为LLM工具格式
// 用于将 Tool 接口转换为 llm.Tool 结构体
func ToLLMTool(t Tool) llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		},
	}
}

# OpsCopilot Agent 系统设计文档

> 本文档描述 Agent 系统的架构设计和实现细节

## 概述

OpsCopilot 的 Agent 系统基于 **ReAct (Reasoning + Acting)** 模式实现，通过 LLM 和工具的迭代交互来完成复杂的问题定位任务。

## 架构图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Agent 架构                                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                        RunAgent 循环                              │   │
│  │                                                                   │   │
│  │   ┌─────────┐    ┌─────────────┐    ┌─────────────┐             │   │
│  │   │ 思考    │───▶│ 选择工具    │───▶│ 执行工具    │             │   │
│  │   │ (LLM)   │    │ (LLM)       │    │ (Registry)  │             │   │
│  │   └─────────┘    └─────────────┘    └──────┬──────┘             │   │
│  │         ▲                                   │                     │   │
│  │         │         ┌─────────────┐           │                     │   │
│  │         └─────────│ 更新历史    │◀──────────┘                     │   │
│  │                   └─────────────┘                                 │   │
│  │                                                                   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                        工具层 (Registry)                          │   │
│  │                                                                   │   │
│  │   ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │   │
│  │   │ search_knowledge│  │list_knowledge   │  │read_knowledge   │ │   │
│  │   │                 │  │    _files       │  │    _file        │ │   │
│  │   │ - 关键词提取    │  │                 │  │                 │ │   │
│  │   │ - 混合检索      │  │ - 文档列表      │  │ - 文件读取      │ │   │
│  │   │ - 结果排序      │  │                 │  │ - 内容截断      │ │   │
│  │   └─────────────────┘  └─────────────────┘  └─────────────────┘ │   │
│  │                                                                   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## 核心组件

### 1. Tool 接口（pkg/tools/interface.go）

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage
    Execute(ctx context.Context, args map[string]interface{}, emitStatus StatusEmitter) (string, error)
}
```

| 方法 | 用途 |
|-----|------|
| `Name()` | 工具唯一标识符，LLM 调用时使用 |
| `Description()` | 工具描述，帮助 LLM 理解何时使用 |
| `Parameters()` | JSON Schema 格式的参数定义 |
| `Execute()` | 执行工具逻辑，返回结果字符串 |

### 2. Registry（pkg/tools/registry.go）

工具注册器，管理所有可用工具：

```go
registry := tools.NewRegistry()
registry.Register(tool1)
registry.Register(tool2)

// 获取工具
if tool, ok := registry.Get("search_knowledge"); ok {
    result, err := tool.Execute(ctx, args, emitStatus)
}

// 转换为 LLM 工具格式
llmTools := registry.ToLLMTools()
```

### 3. Agent 循环（pkg/ai/agent.go）

```go
func (s *AIService) RunAgent(ctx context.Context, opts AgentRunOptions) (string, error) {
    // 1. 创建工具注册器
    registry := tools.NewRegistry()
    registry.Register(knowledge.NewSearchTool(...))
    registry.Register(knowledge.NewListFilesTool(...))
    registry.Register(knowledge.NewReadFileTool(...))

    // 2. 构建 LLM 工具列表
    llmTools := registry.ToLLMTools()

    // 3. ReAct 循环
    for i := 0; i < maxSteps; i++ {
        // 3.1 调用 LLM
        resp, err := provider.ChatWithTools(ctx, messages, llmTools)

        // 3.2 如果没有工具调用，返回结果
        if len(resp.ToolCalls) == 0 {
            return resp.Content, nil
        }

        // 3.3 执行工具调用
        for _, tc := range resp.ToolCalls {
            if tool, ok := registry.Get(tc.Function.Name); ok {
                result, err := tool.Execute(ctx, args, emitStatus)
                // 将结果加入对话历史
                messages = append(messages, llm.ChatMessage{
                    Role:       "tool",
                    ToolCallID: tc.ID,
                    Content:    result,
                })
            }
        }
    }
}
```

## 知识库工具详解

### search_knowledge

最核心的工具，支持智能搜索：

```
┌─────────────────────────────────────────────────────────────────┐
│                    search_knowledge 流程                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  输入: query (搜索词), top_k (返回数量)                          │
│                                                                  │
│  1. 选择搜索关键词 (chooseSearchKey)                             │
│     - 原始问题是中文 → 使用中文                                  │
│     - 模型生成更短 → 使用模型词                                  │
│                                                                  │
│  2. 提取加权关键词 (TermExtractor)                               │
│     - 使用 LLM 提取 5-10 个关键词                               │
│     - 每个关键词有权重 (1-5)                                     │
│     - 结果缓存到 TermCache                                       │
│                                                                  │
│  3. 执行搜索                                                     │
│     - knowledge.SearchWithTerms() 加权搜索                       │
│     - 或 knowledge.Search() 基础搜索                             │
│                                                                  │
│  输出: JSON 数组 [{path, score, snippets}, ...]                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

**chooseSearchKey 策略**：

| 条件 | 选择 |
|-----|------|
| 原始中文 + 模型英文 | 原始中文 |
| 原始英文 + 模型中文 | 模型中文 |
| 两者同语言 | 较短的 |

### list_knowledge_files

列出知识库中的所有文档：

```json
// 输出
["deploy/ssh.md", "database/mysql.md", "network/dns.md"]
```

### read_knowledge_file

读取特定文档内容：

```json
// 输入
{"path": "deploy/ssh.md"}

// 输出（纯文本，自动截断超过 20000 字符）
"# SSH 连接问题排查\n\n## 常见错误..."
```

## 状态发射器

工具执行时可以通过 `StatusEmitter` 向 UI 发送状态更新：

```go
type StatusEmitter func(stage, message string)

// 使用示例
if emitStatus != nil {
    emitStatus("searching", "正在检索相关内容...")
}
```

**常用 stage 值**：

| stage | 用途 |
|-------|------|
| `thinking` | 思考中 |
| `searching` | 搜索中 |
| `reading` | 阅读文档中 |
| `answering` | 生成回答中 |

## 扩展指南

### 添加新工具

1. 创建工具文件 `pkg/tools/mycategory/my_tool.go`：

```go
package mycategory

type MyTool struct {
    // 配置字段
}

func NewMyTool(config string) *MyTool {
    return &MyTool{config: config}
}

func (t *MyTool) Name() string {
    return "my_tool"
}

func (t *MyTool) Description() string {
    return "Tool description for LLM"
}

func (t *MyTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "param1": {"type": "string", "description": "..."}
        },
        "required": ["param1"]
    }`)
}

func (t *MyTool) Execute(ctx context.Context, args map[string]interface{}, emitStatus tools.StatusEmitter) (string, error) {
    // 实现工具逻辑
    return "result", nil
}
```

2. 在 `agent.go` 中注册：

```go
registry.Register(mycategory.NewMyTool(config))
```

## 性能优化

### 关键词缓存

`SearchTool` 支持 `TermCache` 接口，避免重复调用 LLM 提取关键词：

```go
type TermCache interface {
    Get(key string) []knowledge.WeightedTerm
    Set(key string, terms []knowledge.WeightedTerm)
}
```

### 参数类型处理

由于 JSON 解析后数字是 `float64`，工具中需要处理多种类型：

```go
topK := 5
if v, ok := args["top_k"]; ok {
    switch val := v.(type) {
    case int:
        topK = val
    case float64:
        topK = int(val)
    case int64:
        topK = int(val)
    }
}
```

## 未来规划

1. **向量检索** - 添加 embedding 模型，支持语义搜索
2. **混合检索** - 向量 + 关键词融合排序
3. **诊断推理** - 多轮诊断，自动分析命令输出
4. **自动执行** - 安全机制下的命令自动执行

## 参考资料

- [ReAct 论文](https://arxiv.org/abs/2210.03629)
- [OpenAI Function Calling](https://platform.openai.com/docs/guides/function-calling)
- [JSON Schema](https://json-schema.org/)

// Package tools 提供统一的工具接口定义和注册机制
//
// 设计目标:
//   - 定义统一的 Tool 接口，便于扩展新工具
//   - 支持状态发射器（StatusEmitter），让工具能向UI报告进度
//   - 提供注册器（Registry）管理工具集合
//
// 使用示例:
//
//	registry := tools.NewRegistry()
//	registry.Register(myTool)
//	llmTools := registry.ToLLMTools()
//
//	if tool, ok := registry.Get("my_tool"); ok {
//	    result, err := tool.Execute(ctx, args, emitStatus)
//	}
package tools

import (
	"context"
	"encoding/json"
)

// StatusEmitter 状态发射器函数类型
// 用于工具执行时向UI发送状态更新（如"正在搜索..."、"正在阅读..."）
// 参数:
//   - stage: 阶段标识（如 "searching", "reading", "thinking"）
//   - message: 人类可读的状态消息
type StatusEmitter func(stage, message string)

// Tool 统一的工具接口
// 所有知识库工具和未来扩展的工具都需要实现此接口
type Tool interface {
	// Name 返回工具名称（唯一标识符)
	Name() string

	// Description 返回工具描述（用于LLM理解工具用途)
	Description() string

	// Parameters 返回JSON Schema格式的参数定义
	// 参考: https://json-schema.org/understanding-json-schema/
	Parameters() json.RawMessage

	// Execute 执行工具并返回结果
	// 参数:
	//   - ctx: 上下文，支持取消操作
	//   - args: 工具参数，已从JSON解析为map
	//   - emitStatus: 状态发射器(可为nil)
	// 返回:
	//   - string: 工具执行结果(通常是JSON或纯文本)
	//   - error: 执行错误
	Execute(ctx context.Context, args map[string]interface{}, emitStatus StatusEmitter) (string, error)
}

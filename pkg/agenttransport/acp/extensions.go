package acp

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

var managedMethods = []string{"profiles/models/write", "profiles/agents/write", "settings/reload"}

func (r InitializeResult) requireManagedCapabilities() error {
	extensions := r.AgentCapabilities.Meta.Chrys
	if extensions == nil || extensions.APIVersion != 1 {
		return errors.New("iCode 未声明受支持的 ACP 扩展接口（需要 apiVersion 1），请更新运行组件")
	}
	if !r.AgentCapabilities.MCP.HTTP {
		return errors.New("iCode 未开放 HTTP MCP，无法接入 OpsCopilot 工具")
	}
	for _, method := range managedMethods {
		if !slices.Contains(extensions.Methods, method) {
			return fmt.Errorf("iCode 缺少 ACP 接口 %s，请更新运行组件", method)
		}
	}
	return nil
}

// ACP extension names are advertised without a prefix; the stdio wire uses _.
func (c *Client) callExtension(ctx context.Context, method string, params, result any) error {
	return c.Call(ctx, "_"+method, params, result)
}

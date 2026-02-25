package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// Client 定义 MCP 客户端接口
type Client interface {
	// Start 启动 MCP 服务器进程并建立连接
	Start(ctx context.Context, serverPath string) error

	// Stop 停止 MCP 服务器
	Stop(ctx context.Context) error

	// IsReady 检查 MCP 服务器是否就绪
	IsReady() bool

	// ListTools 获取 MCP 服务器提供的工具列表
	ListTools(ctx context.Context) ([]Tool, error)

	// CallTool 调用 MCP 工具并返回结果
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error)
}

// stdioClient 通过标准输入输出与 MCP 服务器通信的客户端实现
type stdioClient struct {
	cmd       *exec.Cmd
	serverCmd string
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	mu        sync.RWMutex
	started   bool
}

func (c *stdioClient) Start(ctx context.Context, serverPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.serverCmd = serverPath
	c.cmd = exec.CommandContext(ctx, serverPath)

	// 设置平台特定的进程属性（隐藏窗口等）
	c.setPlatformCmdAttr()

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return err
	}

	c.stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := c.cmd.Start(); err != nil {
		return err
	}

	c.started = true
	return nil
}

func (c *stdioClient) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	// 关闭管道
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.stdout != nil {
		c.stdout.Close()
	}

	// 终止进程
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}

	c.started = false
	return nil
}

func (c *stdioClient) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.started && c.cmd != nil && c.cmd.Process != nil
}

// ListTools 获取 MCP 服务器提供的工具列表
func (c *stdioClient) ListTools(ctx context.Context) ([]Tool, error) {
	c.mu.RLock()
	if !c.started {
		c.mu.RUnlock()
		return nil, fmt.Errorf("MCP client not started")
	}
	c.mu.RUnlock()

	// 构建 tools/list 请求
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	// 发送请求
	if err := c.sendRequest(req); err != nil {
		return nil, fmt.Errorf("failed to send tools/list request: %w", err)
	}

	// 读取响应
	resp, err := c.readResponse()
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("MCP error: %s", resp.Error.Message)
	}

	// 解析结果
	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	var result ToolsListResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tools list result: %w", err)
	}

	return result.Tools, nil
}

// CallTool 调用 MCP 工具并返回结果
func (c *stdioClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	c.mu.RLock()
	if !c.started {
		c.mu.RUnlock()
		return "", fmt.Errorf("MCP client not started")
	}
	c.mu.RUnlock()

	// 构建 tools/call 请求
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	}

	// 发送请求
	if err := c.sendRequest(req); err != nil {
		return "", fmt.Errorf("failed to send tools/call request: %w", err)
	}

	// 读取响应
	resp, err := c.readResponse()
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("MCP error: %s", resp.Error.Message)
	}

	// 解析结果
	resultJSON, err := json.Marshal(resp.Result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	var result ToolCallResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal tool call result: %w", err)
	}

	// 将 content 转换为字符串
	if len(result.Content) == 0 {
		return "", nil
	}

	// 简化处理：将 content 数组转换为 JSON 字符串
	contentJSON, err := json.Marshal(result.Content)
	if err != nil {
		return "", fmt.Errorf("failed to marshal content: %w", err)
	}

	return string(contentJSON), nil
}

// sendRequest 发送 JSON-RPC 请求到 MCP 服务器
func (c *stdioClient) sendRequest(req JSONRPCRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// 添加换行符（MCP 协议使用行分隔的 JSON）
	data = append(data, '\n')

	_, err = c.stdin.Write(data)
	return err
}

// readResponse 从 MCP 服务器读取 JSON-RPC 响应
func (c *stdioClient) readResponse() (*JSONRPCResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 设置读取超时
	// 注意：这里使用通道超时模式，因为 Read 本身不支持 context
	type result struct {
		data []byte
		err  error
	}

	resultCh := make(chan result, 1)

	go func() {
		// 读取一行（以换行符分隔）
		reader := bufio.NewReader(c.stdout)
		line, err := reader.ReadBytes('\n')
		resultCh <- result{line, err}
	}()

	select {
	case res := <-resultCh:
		if res.err != nil {
			return nil, res.err
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(res.data, &resp); err != nil {
			return nil, err
		}
		return &resp, nil

	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout reading response from MCP server")
	}
}

// NewClient 创建 MCP 客户端实例
func NewClient() Client {
	return &stdioClient{}
}

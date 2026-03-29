package ftipc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Client is the IPC client used by the FTP manager to communicate with the main app.
type Client struct {
	baseURL string
	token   string
}

// NewClientFromTokenFile creates an IPC client by reading the token file.
func NewClientFromTokenFile(dir string) (*Client, error) {
	path := filepath.Join(dir, "ipc.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 IPC 配置失败: %w", err)
	}

	var info IPCInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("解析 IPC 配置失败: %w", err)
	}

	return &Client{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", info.Port),
		token:   info.Token,
	}, nil
}

// NewClient creates an IPC client with direct connection info.
func NewClient(info IPCInfo) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", info.Port),
		token:   info.Token,
	}
}

// GetSessions requests the list of active sessions from the main app.
func (c *Client) GetSessions() (IPCResponse, error) {
	return c.doGet("/api/sessions")
}

// DoAction sends an action request to the main app.
func (c *Client) DoAction(req IPCRequest) (IPCResponse, error) {
	return c.doPost("/api/ft/", req)
}

func (c *Client) doGet(path string) (IPCResponse, error) {
	req, err := http.NewRequest("GET", c.baseURL+path, nil)
	if err != nil {
		return IPCResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return IPCResponse{}, fmt.Errorf("IPC 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return IPCResponse{}, fmt.Errorf("IPC 认证失败")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return IPCResponse{}, fmt.Errorf("读取 IPC 响应失败: %w", err)
	}

	var ipcResp IPCResponse
	if err := json.Unmarshal(body, &ipcResp); err != nil {
		return IPCResponse{}, fmt.Errorf("解析 IPC 响应失败: %w", err)
	}
	return ipcResp, nil
}

func (c *Client) doPost(path string, payload IPCRequest) (IPCResponse, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return IPCResponse{}, err
	}

	req, err := http.NewRequest("POST", c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return IPCResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return IPCResponse{}, fmt.Errorf("IPC 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return IPCResponse{}, fmt.Errorf("IPC 认证失败")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return IPCResponse{}, fmt.Errorf("读取 IPC 响应失败: %w", err)
	}

	var ipcResp IPCResponse
	if err := json.Unmarshal(body, &ipcResp); err != nil {
		return IPCResponse{}, fmt.Errorf("解析 IPC 响应失败: %w", err)
	}
	return ipcResp, nil
}

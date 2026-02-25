package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// MCPServerConfig MCP 服务器配置
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPConfig MCP 配置文件结构
type MCPConfig struct {
	Servers map[string]MCPServerConfig `json:"mcpServers"`
}

// Manager MCP 服务器管理器
type Manager struct {
	configPath string
	config     *MCPConfig
	clients    map[string]Client // 服务器名 -> 客户端
	mu         sync.RWMutex
}

// NewManager 创建 MCP 管理器
func NewManager(configPath string) *Manager {
	return &Manager{
		configPath: configPath,
		clients:    make(map[string]Client),
	}
}

// Load 加载配置文件
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查配置文件是否存在
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		// 配置文件不存在，创建空配置
		m.config = &MCPConfig{
			Servers: make(map[string]MCPServerConfig),
		}
		return nil
	}

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	m.config = &config
	return nil
}

// Save 保存配置文件
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config == nil {
		m.config = &MCPConfig{
			Servers: make(map[string]MCPServerConfig),
		}
	}

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(m.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetServerConfigs 获取所有服务器配置
func (m *Manager) GetServerConfigs() map[string]MCPServerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config == nil {
		return make(map[string]MCPServerConfig)
	}

	result := make(map[string]MCPServerConfig, len(m.config.Servers))
	for k, v := range m.config.Servers {
		result[k] = v
	}
	return result
}

// GetStatus 获取所有 MCP 服务器的状态
func (m *Manager) GetStatus() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]bool)
	for name, client := range m.clients {
		status[name] = client.IsReady()
		fmt.Printf("[MCP.GetStatus] Server %s: ready=%v\n", name, client.IsReady())
	}

	// 添加配置了但未启动的服务器
	if m.config != nil {
		for name := range m.config.Servers {
			if _, exists := status[name]; !exists {
				status[name] = false
				fmt.Printf("[MCP.GetStatus] Configured server %s: not started (added as false)\n", name)
			}
		}
	} else {
		fmt.Println("[MCP.GetStatus] Config is nil")
	}

	fmt.Printf("[MCP.GetStatus] Final status: %+v\n", status)
	return status
}

// StartAll 启动所有配置的 MCP 服务器
func (m *Manager) StartAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.config == nil || len(m.config.Servers) == 0 {
		return nil
	}

	for name, serverConfig := range m.config.Servers {
		if _, exists := m.clients[name]; exists {
			continue // 已启动
		}

		client := NewClient()
		ctx := context.Background()
		if err := client.Start(ctx, serverConfig.Command, serverConfig.Args...); err != nil {
			// 记录错误但继续启动其他服务器
			fmt.Printf("[MCP] Failed to start server %s: %v\n", name, err)
			continue
		}

		m.clients[name] = client
		fmt.Printf("[MCP] Server %s started successfully\n", name)
	}

	return nil
}

// StopAll 停止所有 MCP 服务器
func (m *Manager) StopAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	ctx := context.Background()
	for name, client := range m.clients {
		if err := client.Stop(ctx); err != nil {
			fmt.Printf("[MCP] Error stopping server %s: %v\n", name, err)
			lastErr = err
		}
		delete(m.clients, name)
	}

	return lastErr
}

// GetClient 获取指定服务器的客户端
func (m *Manager) GetClient(name string) (Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, exists := m.clients[name]
	if !exists {
		return nil, false
	}
	return client, true
}

// GetAllClients 获取所有已启动的客户端
func (m *Manager) GetAllClients() map[string]Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]Client)
	for name, client := range m.clients {
		result[name] = client
	}
	return result
}

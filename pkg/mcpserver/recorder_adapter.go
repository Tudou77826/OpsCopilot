package mcpserver

import (
	"fmt"
	"sync"
	"time"

	"opscopilot/pkg/recorder"
)

// MCPRecordedCommand MCP 记录的命令（包含服务器信息）
type MCPRecordedCommand struct {
	Index     int    `json:"index"`
	Command   string `json:"command"`
	Server    string `json:"server"`
	Output    string `json:"output"`
	ExitCode  int    `json:"exit_code"`
	Duration  int64  `json:"duration_ms"`
	Timestamp int64  `json:"timestamp"` // 相对开始时间的毫秒数
	Note      string `json:"note,omitempty"`
}

// MCPSessionInfo MCP 会话信息
type MCPSessionInfo struct {
	ID          string              `json:"id"`
	Problem     string              `json:"problem"`
	StartTime   time.Time           `json:"start_time"`
	EndTime     *time.Time          `json:"end_time,omitempty"`
	RootCause   string              `json:"root_cause,omitempty"`
	Conclusion  string              `json:"conclusion,omitempty"`
	Commands    []MCPRecordedCommand `json:"commands"`
	Servers     map[string]bool     `json:"servers"`
	Findings    []string            `json:"findings"`
	Suggestions []string            `json:"suggestions"`
}

// MCPRecorderAdapter MCP 录制器适配器
// 复用主程序 recorder，添加 MCP 特有功能（服务器追踪、exitCode、findings）
type MCPRecorderAdapter struct {
	recorder   *recorder.Recorder
	current    *MCPSessionInfo
	mu         sync.RWMutex
}

// NewMCPRecorderAdapter 创建适配器
func NewMCPRecorderAdapter(r *recorder.Recorder) *MCPRecorderAdapter {
	return &MCPRecorderAdapter{
		recorder: r,
	}
}

// StartSession 开始故障排查会话
func (a *MCPRecorderAdapter) StartSession(problem string) (*MCPSessionInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 检查是否已有活动会话
	if a.current != nil {
		return nil, fmt.Errorf("已有活动的排查会话，请先结束当前会话")
	}

	// 使用主程序 recorder 的 StartSession（兼容旧 API）
	session := a.recorder.StartSession(problem, nil)
	if session == nil {
		return nil, fmt.Errorf("failed to start session")
	}

	// 创建 MCP 会话信息
	a.current = &MCPSessionInfo{
		ID:          session.ID,
		Problem:     problem,
		StartTime:   session.StartTime,
		Commands:    make([]MCPRecordedCommand, 0),
		Servers:     make(map[string]bool),
		Findings:    make([]string, 0),
		Suggestions: make([]string, 0),
	}

	return a.current, nil
}

// RecordCommand 记录命令执行
func (a *MCPRecorderAdapter) RecordCommand(server, command, output string, exitCode int, duration time.Duration, note string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.current == nil {
		return fmt.Errorf("no active recording session")
	}

	// 截断输出
	const maxOutputLen = 10000
	if len(output) > maxOutputLen {
		output = output[:maxOutputLen] + "...[truncated]"
	}

	// 计算相对时间戳
	timestamp := time.Since(a.current.StartTime).Milliseconds()

	// 添加到 MCP 命令列表
	a.current.Commands = append(a.current.Commands, MCPRecordedCommand{
		Index:     len(a.current.Commands),
		Command:   command,
		Server:    server,
		Output:    output,
		ExitCode:  exitCode,
		Duration:  duration.Milliseconds(),
		Timestamp: timestamp,
		Note:      note,
	})

	// 记录服务器
	a.current.Servers[server] = true

	// 同时记录到主程序 recorder（作为时间线事件）
	a.recorder.AddEvent("mcp_command", command, map[string]interface{}{
		"server":    server,
		"exit_code": exitCode,
		"duration":  duration.Milliseconds(),
		"note":      note,
	})

	return nil
}

// EndSession 结束录制会话
func (a *MCPRecorderAdapter) EndSession(rootCause, conclusion string, findings []string) (*MCPSessionInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.current == nil {
		return nil, fmt.Errorf("no active recording session")
	}

	// 设置结束信息
	now := time.Now()
	a.current.EndTime = &now
	a.current.RootCause = rootCause
	a.current.Conclusion = conclusion
	a.current.Findings = append(a.current.Findings, findings...)

	// 使用主程序 recorder 结束会话
	if err := a.recorder.StopSession(rootCause, conclusion); err != nil {
		return nil, err
	}

	session := a.current
	a.current = nil

	return session, nil
}

// GetCurrentSession 获取当前会话
func (a *MCPRecorderAdapter) GetCurrentSession() *MCPSessionInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.current
}

// GetSessionStatus 获取会话状态
func (a *MCPRecorderAdapter) GetSessionStatus() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.current == nil {
		return map[string]interface{}{
			"active": false,
		}
	}

	servers := make([]string, 0)
	for s := range a.current.Servers {
		servers = append(servers, s)
	}

	return map[string]interface{}{
		"active":            true,
		"session_id":        a.current.ID,
		"problem":           a.current.Problem,
		"started_at":        a.current.StartTime.Format(time.RFC3339),
		"duration_seconds":  int(time.Since(a.current.StartTime).Seconds()),
		"connected_servers": servers,
		"executed_commands": len(a.current.Commands),
		"findings":          a.current.Findings,
	}
}

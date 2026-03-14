package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	recorder     *recorder.Recorder
	current      *MCPSessionInfo
	knowledgeDir string // 知识库目录
	mu           sync.RWMutex
}

// NewMCPRecorderAdapter 创建适配器
func NewMCPRecorderAdapter(r *recorder.Recorder, knowledgeDir string) *MCPRecorderAdapter {
	return &MCPRecorderAdapter{
		recorder:     r,
		knowledgeDir: knowledgeDir,
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

	// 归档到知识库
	if a.knowledgeDir != "" {
		if err := a.archiveToKnowledge(); err != nil {
			// 归档失败不影响会话结束，只记录错误
			fmt.Printf("[MCP] Warning: failed to archive to knowledge: %v\n", err)
		}
	}

	session := a.current
	a.current = nil

	return session, nil
}

// archiveToKnowledge 将会话归档到知识库
func (a *MCPRecorderAdapter) archiveToKnowledge() error {
	if a.current == nil {
		return nil
	}

	// 创建知识库子目录
	troubleshootDir := filepath.Join(a.knowledgeDir, "troubleshooting")
	if err := os.MkdirAll(troubleshootDir, 0755); err != nil {
		return fmt.Errorf("failed to create knowledge directory: %w", err)
	}

	// 生成 Markdown 内容
	content := a.generateKnowledgeMarkdown()

	// 生成文件名（使用日期+问题摘要）
	dateStr := a.current.StartTime.Format("2006-01-02")
	problemSlug := slugify(a.current.Problem)
	if len(problemSlug) > 50 {
		problemSlug = problemSlug[:50]
	}
	filename := fmt.Sprintf("%s_%s_%s.md", dateStr, problemSlug, a.current.ID[:8])
	filePath := filepath.Join(troubleshootDir, filename)

	// 写入文件
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write knowledge file: %w", err)
	}

	fmt.Printf("[MCP] Archived to knowledge: %s\n", filePath)
	return nil
}

// generateKnowledgeMarkdown 生成知识库 Markdown 内容
func (a *MCPRecorderAdapter) generateKnowledgeMarkdown() string {
	var sb strings.Builder

	// 标题
	sb.WriteString(fmt.Sprintf("# %s\n\n", a.current.Problem))

	// 元信息
	sb.WriteString("## 概述\n\n")
	sb.WriteString(fmt.Sprintf("- **开始时间**: %s\n", a.current.StartTime.Format("2006-01-02 15:04:05")))
	if a.current.EndTime != nil {
		sb.WriteString(fmt.Sprintf("- **结束时间**: %s\n", a.current.EndTime.Format("2006-01-02 15:04:05")))
		sb.WriteString(fmt.Sprintf("- **持续时间**: %d 秒\n", int(a.current.EndTime.Sub(a.current.StartTime).Seconds())))
	}
	sb.WriteString(fmt.Sprintf("- **涉及服务器**: %d 台\n", len(a.current.Servers)))

	// 服务器列表
	if len(a.current.Servers) > 0 {
		sb.WriteString("\n### 服务器\n\n")
		for server := range a.current.Servers {
			sb.WriteString(fmt.Sprintf("- %s\n", server))
		}
	}

	// 根因
	if a.current.RootCause != "" {
		sb.WriteString("\n## 根本原因\n\n")
		sb.WriteString(a.current.RootCause + "\n")
	}

	// 解决方案
	if a.current.Conclusion != "" {
		sb.WriteString("\n## 解决方案\n\n")
		sb.WriteString(a.current.Conclusion + "\n")
	}

	// 关键发现
	if len(a.current.Findings) > 0 {
		sb.WriteString("\n## 关键发现\n\n")
		for _, f := range a.current.Findings {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}

	// 执行的命令
	if len(a.current.Commands) > 0 {
		sb.WriteString("\n## 执行的命令\n\n")
		for _, cmd := range a.current.Commands {
			sb.WriteString(fmt.Sprintf("### 命令 %d: %s\n\n", cmd.Index+1, cmd.Command))
			sb.WriteString(fmt.Sprintf("- **服务器**: %s\n", cmd.Server))
			sb.WriteString(fmt.Sprintf("- **执行时间**: %d ms\n", cmd.Duration))
			sb.WriteString(fmt.Sprintf("- **退出码**: %d\n", cmd.ExitCode))
			if cmd.Note != "" {
				sb.WriteString(fmt.Sprintf("- **备注**: %s\n", cmd.Note))
			}
			sb.WriteString("\n**输出**:\n\n```\n")
			sb.WriteString(cmd.Output)
			sb.WriteString("\n```\n\n")
		}
	}

	// 元数据
	sb.WriteString("\n---\n\n")
	sb.WriteString(fmt.Sprintf("*会话ID: %s*\n", a.current.ID))
	sb.WriteString(fmt.Sprintf("*归档时间: %s*\n", time.Now().Format("2006-01-02 15:04:05")))

	return sb.String()
}

// slugify 将字符串转换为文件名友好的格式
func slugify(s string) string {
	// 替换空格和特殊字符
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, "?", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "<", "")
	s = strings.ReplaceAll(s, ">", "")
	s = strings.ReplaceAll(s, "|", "")
	s = strings.ReplaceAll(s, "\"", "")
	s = strings.ReplaceAll(s, "'", "")

	// 限制长度（中文字符占3字节，需要保留足够空间）
	if len(s) > 50 {
		// 尝试在字符边界截断
		runes := []rune(s)
		if len(runes) > 20 {
			s = string(runes[:20])
		}
	}

	return s
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

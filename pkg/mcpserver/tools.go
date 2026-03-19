package mcpserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
)

// encodeBase64 编码字符串为 base64（用于安全传递密码）
func encodeBase64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// toolServerList 列出服务器
func (s *Server) toolServerList() (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connected := make([]map[string]interface{}, 0)
	available := make([]map[string]interface{}, 0)

	// 已连接的服务器
	for name, conn := range s.connections {
		lastActive := time.Unix(0, conn.LastActive.Load())
		connected = append(connected, map[string]interface{}{
			"name":          name,
			"connected_at":  conn.ConnectedAt,
			"idle_seconds":  int(time.Since(lastActive).Seconds()),
		})
	}

	// 可连接的服务器（从 sessions.json 读取）
	// 递归获取所有 session
	var flattenSessions func(nodes []*sessionmanager.Session)
	flattenSessions = func(nodes []*sessionmanager.Session) {
		for _, node := range nodes {
			if node.Type == sessionmanager.TypeSession && node.Config != nil {
				if _, isConnected := s.connections[node.Name]; !isConnected {
					available = append(available, map[string]interface{}{
						"name":  node.Name,
						"host":  node.Config.Host,
						"port":  node.Config.Port,
						"user":  node.Config.User,
						"group": node.Config.Group,
					})
				}
			}
			if node.Type == sessionmanager.TypeFolder {
				flattenSessions(node.Children)
			}
		}
	}
	flattenSessions(s.sessionMgr.GetSessions())

	// 收集所有分组
	groups := make(map[string]bool)
	var collectGroups func(nodes []*sessionmanager.Session)
	collectGroups = func(nodes []*sessionmanager.Session) {
		for _, node := range nodes {
			if node.Type == sessionmanager.TypeSession && node.Config != nil && node.Config.Group != "" {
				groups[node.Config.Group] = true
			}
			if node.Type == sessionmanager.TypeFolder {
				groups[node.Name] = true
				collectGroups(node.Children)
			}
		}
	}
	collectGroups(s.sessionMgr.GetSessions())

	groupList := make([]string, 0)
	for g := range groups {
		groupList = append(groupList, g)
	}

	return map[string]interface{}{
		"connected": connected,
		"available": available,
		"groups":    groupList,
		"note":      "available 服务器已保存凭证，但连接可能失败（如网络不通、密码已变更）",
	}, nil
}

// toolServerConnect 连接服务器
func (s *Server) toolServerConnect(args map[string]interface{}) (interface{}, error) {
	serverName, ok := args["server"].(string)
	if !ok || serverName == "" {
		return nil, fmt.Errorf("缺少 server 参数")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已连接
	if conn, exists := s.connections[serverName]; exists {
		conn.LastActive.Store(time.Now().UnixNano())
		return map[string]interface{}{
			"success": true,
			"server":  serverName,
			"message": "已连接",
		}, nil
	}

	// 从 sessions.json 查找服务器配置
	var serverConfig *sshclient.ConnectConfig
	var findConfig func(nodes []*sessionmanager.Session) *sshclient.ConnectConfig
	findConfig = func(nodes []*sessionmanager.Session) *sshclient.ConnectConfig {
		for _, node := range nodes {
			if node.Type == sessionmanager.TypeSession && node.Name == serverName && node.Config != nil {
				return node.Config
			}
			if node.Type == sessionmanager.TypeFolder {
				if found := findConfig(node.Children); found != nil {
					return found
				}
			}
		}
		return nil
	}
	serverConfig = findConfig(s.sessionMgr.GetSessions())

	if serverConfig == nil {
		return nil, fmt.Errorf("服务器 '%s' 未找到", serverName)
	}

	// 从 secretstore 获取密码
	password, err := s.secretStore.Get("opscopilot", serverConfig.Host+"_"+serverConfig.User)
	if err != nil {
		// 如果 keyring 中没有密码，尝试从 config 中获取（兼容旧格式）
		// sessions.json 中可能存储了明文密码（不推荐，但需要兼容）
		password = ""
	}

	// 如果密码为空，尝试使用 config 中的密码字段
	if password == "" && serverConfig.Password != "" {
		password = serverConfig.Password
	}

	if password == "" {
		return nil, fmt.Errorf("服务器 '%s' 的密码未找到，请先在 OpsCopilot 中连接一次", serverName)
	}

	// 创建 SSH 配置
	sshConfig := &sshclient.ConnectConfig{
		Name:     serverConfig.Name,
		Host:     serverConfig.Host,
		Port:     serverConfig.Port,
		User:     serverConfig.User,
		Password: password,
		Group:    serverConfig.Group,
	}

	// 获取 root 密码（尝试多个来源）
	rootPassword := ""

	// 1. 优先从 sessions.json 中的配置获取
	if serverConfig.RootPassword != "" {
		rootPassword = serverConfig.RootPassword
	}

	// 2. 尝试从 keyring 获取（使用 "root" 作为用户标识）
	if rootPassword == "" {
		if rp, err := s.secretStore.Get("OpsCopilot-SSH", serverConfig.Host+":root"); err == nil {
			rootPassword = rp
		}
	}

	// 3. 尝试使用普通用户密码作为 root 密码（某些情况下相同）
	// 注意：这不是最佳实践，但有时用户会使用相同的密码

	// 处理跳板机
	if serverConfig.Bastion != nil {
		// 从 secretstore 获取跳板机密码
		bastionPassword, err := s.secretStore.Get("opscopilot", serverConfig.Bastion.Host+"_"+serverConfig.Bastion.User)
		if err != nil {
			bastionPassword = ""
		}
		if bastionPassword == "" && serverConfig.Bastion.Password != "" {
			bastionPassword = serverConfig.Bastion.Password
		}

		if bastionPassword == "" {
			return nil, fmt.Errorf("跳板机 '%s' 的密码未找到", serverConfig.Bastion.Host)
		}

		sshConfig.Bastion = &sshclient.ConnectConfig{
			Name:     serverConfig.Bastion.Name,
			Host:     serverConfig.Bastion.Host,
			Port:     serverConfig.Bastion.Port,
			User:     serverConfig.Bastion.User,
			Password: bastionPassword,
		}
	}

	// 创建 SSH 客户端
	client, err := sshclient.NewClient(sshConfig)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}

	// 保存连接（包括 root 密码用于 sudo）
	conn := &Connection{
		Name:         serverName,
		Host:         serverConfig.Host, // 保存 IP 用于白名单匹配
		Client:       client,
		RootPassword: rootPassword,
		ConnectedAt:  time.Now(),
	}
	conn.LastActive.Store(time.Now().UnixNano())
	s.connections[serverName] = conn

	return map[string]interface{}{
		"success":       true,
		"server":        serverName,
		"message":       "连接成功",
		"has_root_auth": rootPassword != "",
	}, nil
}

// toolServerDisconnect 断开服务器连接
func (s *Server) toolServerDisconnect(args map[string]interface{}) (interface{}, error) {
	serverName, ok := args["server"].(string)
	if !ok || serverName == "" {
		return nil, fmt.Errorf("缺少 server 参数")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	conn, exists := s.connections[serverName]
	if !exists {
		return nil, fmt.Errorf("服务器 '%s' 未连接", serverName)
	}

	if conn.Client != nil {
		conn.Client.Close()
	}

	delete(s.connections, serverName)

	return map[string]interface{}{
		"success": true,
		"server":  serverName,
		"message": "已断开连接",
	}, nil
}

// toolSSHExec 执行 SSH 命令
func (s *Server) toolSSHExec(args map[string]interface{}) (interface{}, error) {
	serverName, ok := args["server"].(string)
	if !ok || serverName == "" {
		return nil, fmt.Errorf("缺少 server 参数")
	}

	command, ok := args["command"].(string)
	if !ok || command == "" {
		return nil, fmt.Errorf("缺少 command 参数")
	}

	note, _ := args["note"].(string)
	maxLineLength := 500
	if v, ok := args["max_line_length"].(float64); ok {
		maxLineLength = int(v)
	}

	// 获取连接（先获取以确定服务器 IP）
	s.mu.RLock()
	conn, exists := s.connections[serverName]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("服务器 '%s' 未连接，请先使用 server_connect 连接", serverName)
	}

	// 检查命令是否允许（使用白名单管理器）
	var checkResult CheckResult
	if s.whitelistManager != nil {
		// 重新加载配置，确保使用最新的白名单（如 UI 刚修改过）
		_ = s.whitelistManager.Reload()
		checkResult = s.whitelistManager.Check(command, conn.Host)
	} else {
		// 回退到简单检查器
		simpleResult := s.checker.Check(command)
		checkResult = CheckResult{
			Allowed: simpleResult.Allowed,
			Reason:  simpleResult.Reason,
		}
	}

	if !checkResult.Allowed {
		return nil, fmt.Errorf("%s", checkResult.Reason)
	}

	if !exists {
		return nil, fmt.Errorf("服务器 '%s' 未连接，请先使用 server_connect 连接", serverName)
	}

	// 执行命令
	startTime := time.Now()

	var output string
	var err error

	// 如果配置了 root 密码，使用 su 执行命令
	if conn.RootPassword != "" {
		// 使用 su -c 执行命令，密码通过 stdin 传递
		// 格式: echo 'password' | su -c 'command' -
		escapedCmd := strings.ReplaceAll(command, "'", "'\\''")
		fullCmd := fmt.Sprintf("echo '%s' | su -c '%s' -", conn.RootPassword, escapedCmd)
		output, err = conn.Client.Run(fullCmd)
		// 如果 su 命令失败，回退到普通执行
		if err != nil {
			output, err = conn.Client.Run(command)
		}
	} else {
		output, err = conn.Client.Run(command)
	}

	duration := time.Since(startTime)
	exitCode := 0
	if err != nil {
		exitCode = 1
		output = err.Error()
	}

	// 更新最后活动时间（使用原子操作，并发安全）
	conn.LastActive.Store(time.Now().UnixNano())

	// 处理输出
	controller := NewOutputController(s.config.MaxTotalBytes, maxLineLength, s.config.HeadLines)
	result := controller.Process(output)

	// 记录到录制会话（使用 MCP 适配器）
	if s.mcpRecorder.GetCurrentSession() != nil {
		s.mcpRecorder.RecordCommand(serverName, command, result.Output, exitCode, duration, note)
	}

	return map[string]interface{}{
		"success": err == nil,
		"output":  result.Output,
		"meta": map[string]interface{}{
			"total_bytes":          result.Meta.TotalBytes,
			"returned_bytes":       result.Meta.ReturnedBytes,
			"total_lines":          result.Meta.TotalLines,
			"returned_lines":       result.Meta.ReturnedLines,
			"truncated_lines":      result.Meta.TruncatedLines,
			"long_lines_truncated": result.Meta.LongLinesTruncated,
			"command":              command,
			"server":               serverName,
			"duration_ms":          duration.Milliseconds(),
			"exit_code":            exitCode,
		},
	}, nil
}

// toolSessionStart 开始会话
func (s *Server) toolSessionStart(args map[string]interface{}) (interface{}, error) {
	problem, ok := args["problem"].(string)
	if !ok || problem == "" {
		return nil, fmt.Errorf("缺少 problem 参数")
	}

	// 使用 MCP 适配器开始会话
	session, err := s.mcpRecorder.StartSession(problem)
	if err != nil {
		return nil, err
	}

	// 如果指定了服务器列表，预先连接
	if servers, ok := args["servers"].([]interface{}); ok {
		for _, sv := range servers {
			if serverName, ok := sv.(string); ok {
				s.toolServerConnect(map[string]interface{}{"server": serverName})
			}
		}
	}

	return map[string]interface{}{
		"session_id": session.ID,
		"status":     "active",
		"message":    "会话已开始，所有操作将被记录",
	}, nil
}

// toolSessionStatus 查看会话状态
func (s *Server) toolSessionStatus() (interface{}, error) {
	status := s.mcpRecorder.GetSessionStatus()
	return status, nil
}

// toolSessionEnd 结束会话
func (s *Server) toolSessionEnd(args map[string]interface{}) (interface{}, error) {
	summary, ok := args["summary"].(string)
	if !ok || summary == "" {
		return nil, fmt.Errorf("缺少 summary 参数")
	}

	rootCause, _ := args["root_cause"].(string)

	findings := make([]string, 0)
	if f, ok := args["findings"].([]interface{}); ok {
		for _, v := range f {
			if s, ok := v.(string); ok {
				findings = append(findings, s)
			}
		}
	}

	// 使用 MCP 适配器结束会话
	session, err := s.mcpRecorder.EndSession(rootCause, summary, findings)
	if err != nil {
		return nil, err
	}

	// 断开所有连接
	s.mu.Lock()
	for name, conn := range s.connections {
		if conn.Client != nil {
			conn.Client.Close()
		}
		delete(s.connections, name)
	}
	s.mu.Unlock()

	// 收集服务器列表
	servers := make([]string, 0)
	for server := range session.Servers {
		servers = append(servers, server)
	}

	return map[string]interface{}{
		"session_id":          session.ID,
		"status":              "completed",
		"duration_seconds":    int(session.EndTime.Sub(session.StartTime).Seconds()),
		"servers_accessed":    len(servers),
		"commands_executed":   len(session.Commands),
		"root_cause":          session.RootCause,
		"conclusion":          session.Conclusion,
	}, nil
}

// toolGetHints 获取排查提示
func (s *Server) toolGetHints(args map[string]interface{}) (interface{}, error) {
	problem, ok := args["problem"].(string)
	if !ok || problem == "" {
		return nil, fmt.Errorf("缺少 problem 参数")
	}

	problemContext, _ := args["context"].(string)

	// 检查 AIService 是否可用
	if s.aiService == nil {
		return nil, fmt.Errorf("AI 服务未初始化，请检查 config.json 中的 LLM 配置（需要配置 llm.APIKey）")
	}

	// 组合问题和上下文
	fullProblem := problem
	if problemContext != "" {
		fullProblem = problem + "\n\n上下文：" + problemContext
	}

	// 调用 AskTroubleshoot（enableMCP=false，不使用 MCP 工具）
	result, err := s.aiService.AskTroubleshoot(context.Background(), fullProblem, s.config.KnowledgeDir, false)
	if err != nil {
		return nil, fmt.Errorf("定位指导生成失败: %w", err)
	}

	// 返回 Markdown 格式的定位指导
	return map[string]interface{}{
		"content": result,  // Markdown 文本
		"format":  "markdown",
	}, nil
}

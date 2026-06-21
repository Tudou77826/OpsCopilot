package ops

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
)

// buildSudoCommand 构造以 root 身份执行命令的 shell 语句
// 格式: echo 'password' | su -c 'command' -
// 命令中的单引号被转义以安全嵌入，密码通过 stdin 传递给 su
func buildSudoCommand(command, rootPassword string) string {
	escapedCmd := strings.ReplaceAll(command, "'", "'\\''")
	return fmt.Sprintf("echo '%s' | su -c '%s' -", rootPassword, escapedCmd)
}

// ConnectResult 连接结果
type ConnectResult struct {
	Success    bool   `json:"success"`
	Server     string `json:"server"`
	Message    string `json:"message"`
	HasRootAuth bool  `json:"has_root_auth"`
}

// Connect 连接服务器
// 从 sessions.json 读取配置，从 secretstore 读取凭据，建立 SSH 连接。
// 已连接同名服务器时幂等返回（刷新活跃时间，不重建）。
func (m *Manager) Connect(serverName string) (*ConnectResult, error) {
	if serverName == "" {
		return nil, fmt.Errorf("缺少 server 参数")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已连接（幂等）
	if conn, exists := m.connections[serverName]; exists {
		conn.LastActive.Store(time.Now().UnixNano())
		return &ConnectResult{
			Success: true,
			Server:  serverName,
			Message: "已连接",
		}, nil
	}

	// 从 sessions.json 查找服务器配置
	serverConfig := findSessionConfig(m.sessionMgr.GetSessions(), serverName)
	if serverConfig == nil {
		return nil, fmt.Errorf("服务器 '%s' 未找到", serverName)
	}

	// 从 secretstore 获取密码
	password, err := m.secretStore.Get("opscopilot", serverConfig.Host+"_"+serverConfig.User)
	if err != nil {
		password = ""
	}
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
	if serverConfig.RootPassword != "" {
		rootPassword = serverConfig.RootPassword
	}
	if rootPassword == "" {
		if rp, err := m.secretStore.Get("OpsCopilot-SSH", serverConfig.Host+":root"); err == nil {
			rootPassword = rp
		}
	}

	// 处理跳板机
	if serverConfig.Bastion != nil {
		bastionPassword, err := m.secretStore.Get("opscopilot", serverConfig.Bastion.Host+"_"+serverConfig.Bastion.User)
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

	conn := &Connection{
		Name:         serverName,
		Host:         serverConfig.Host,
		Client:       client,
		RootPassword: rootPassword,
		ConnectedAt:  time.Now(),
	}
	conn.LastActive.Store(time.Now().UnixNano())
	m.connections[serverName] = conn

	return &ConnectResult{
		Success:     true,
		Server:      serverName,
		Message:     "连接成功",
		HasRootAuth: rootPassword != "",
	}, nil
}

// Disconnect 断开服务器连接
func (m *Manager) Disconnect(serverName string) error {
	if serverName == "" {
		return fmt.Errorf("缺少 server 参数")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[serverName]
	if !exists {
		return fmt.Errorf("服务器 '%s' 未连接", serverName)
	}

	if conn.Client != nil {
		conn.Client.Close()
	}
	delete(m.connections, serverName)
	return nil
}

// ExecOptions 命令执行选项
type ExecOptions struct {
	MaxLineLength int           // 单行最大长度，默认 500
	Timeout       time.Duration // 单条命令超时，默认 120s；超时后向远端发 SIGKILL 并返回错误
}

// ExecResult 命令执行结果
type ExecResult struct {
	Success  bool              `json:"success"`
	Output   string            `json:"output"`
	Meta     map[string]any    `json:"meta"`
}

// Exec 在已连接的服务器上执行命令
// 安全闸门内置且优先：先校验白名单（仅用 sessions 中的 host 即可，不触发 SSH），
// 通过后再按需连接并执行。调用方无法绕过此校验。
// 若服务器尚未连接，会自动连接（惰性连接，避免白名单拒绝时白白发起 SSH）。
// 命令级超时：默认 120s，超时后杀掉远端进程，避免慢命令拖死共享连接。
// 连接失效时自动重建一次，保证单条慢命令不会让后续命令全部不可用。
func (m *Manager) Exec(ctx context.Context, serverName, command string, opts ExecOptions) (*ExecResult, error) {
	if serverName == "" {
		return nil, fmt.Errorf("缺少 server 参数")
	}
	if command == "" {
		return nil, fmt.Errorf("缺少 command 参数")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	maxLineLength := 500
	if opts.MaxLineLength > 0 {
		maxLineLength = opts.MaxLineLength
	}
	timeout := 120 * time.Second
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}

	// 1. 先查服务器 host（仅读 sessions.json，不触发 SSH），用于白名单策略匹配
	m.mu.RLock()
	conn, exists := m.connections[serverName]
	m.mu.RUnlock()

	var host string
	if exists {
		host = conn.Host
	} else {
		cfg := findSessionConfig(m.sessionMgr.GetSessions(), serverName)
		if cfg == nil {
			return nil, fmt.Errorf("服务器 '%s' 未找到", serverName)
		}
		host = cfg.Host
	}

	// 2. === 安全闸门：命令必须通过白名单校验（不可绕过，且先于连接）===
	checkResult := m.whitelistManager.Check(command, host)
	if !checkResult.Allowed {
		return nil, fmt.Errorf("%s", checkResult.Reason)
	}

	// 3. 若尚未连接，则自动连接（惰性）
	if !exists {
		if _, err := m.Connect(serverName); err != nil {
			return nil, err
		}
		m.mu.RLock()
		conn, exists = m.connections[serverName]
		m.mu.RUnlock()
		if !exists {
			return nil, fmt.Errorf("服务器 '%s' 连接后仍不可用", serverName)
		}
	}

	// 执行命令（带超时；连接失效时重建一次重试）
	startTime := time.Now()

	runCmd := func(c *Connection) (string, error) {
		// 每条命令独立的超时 context（也受外部 ctx 约束）
		cmdCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// 如果配置了 root 密码，使用 su 执行命令
		if c.RootPassword != "" {
			fullCmd := buildSudoCommand(command, c.RootPassword)
			out, e := c.Client.RunWithContext(cmdCtx, fullCmd)
			if e != nil {
				// su 失败回退到普通执行（注意：超时错误不回退）
				if isTimeoutErr(e) {
					return out, e
				}
				return c.Client.RunWithContext(cmdCtx, command)
			}
			return out, nil
		}
		return c.Client.RunWithContext(cmdCtx, command)
	}

	output, err := runCmd(conn)
	activeConn := conn

	// 若执行失败且疑似连接失效（NewSession 报错），重建连接后重试一次
	// 这样单条慢命令把连接拖死后，下一条能自愈，不必重开整个会话
	if err != nil && !isTimeoutErr(err) && isConnectionDead(conn) {
		slog.Warn("connection appears dead, attempting reconnect", "server", serverName, "error", err)
		if reconn, rErr := m.reconnect(serverName); rErr == nil {
			activeConn = reconn
			output, err = runCmd(reconn)
		}
	}

	duration := time.Since(startTime)
	exitCode := 0
	if err != nil {
		exitCode = 1
		output = err.Error()
	}

	// 更新最后活动时间（使用原子操作，并发安全）
	activeConn.LastActive.Store(time.Now().UnixNano())

	// 处理输出
	controller := NewOutputController(m.config.MaxTotalBytes, maxLineLength, m.config.HeadLines)
	result := controller.Process(output)

	return &ExecResult{
		Success: err == nil,
		Output:  result.Output,
		Meta: map[string]any{
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

// findSessionConfig 递归从会话树中按 Host(IP) 查找服务器配置。
// 以 IP 为唯一主键：用户在 OpsCopilot 中登记服务器时 Host 字段即 IP，
// 且 UpdateSession 保证同 Host 唯一，因此 IP → session 是 1:1 映射，
// 避免了 Name 可被改成别名导致 CLI 无法定位的问题。
func findSessionConfig(nodes []*sessionmanager.Session, host string) *sshclient.ConnectConfig {
	for _, node := range nodes {
		if node.Type == sessionmanager.TypeSession && node.Config != nil && node.Config.Host == host {
			return node.Config
		}
		if node.Type == sessionmanager.TypeFolder {
			if found := findSessionConfig(node.Children, host); found != nil {
				return found
			}
		}
	}
	return nil
}

// isTimeoutErr 判断错误是否为 context 超时/取消
// 超时错误不应触发连接重建（命令慢不代表连接坏），也回退到 su 之外的普通执行
func isTimeoutErr(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// isConnectionDead 探测 SSH 连接是否已失效
// 通过尝试创建新 session 判断：若失败，说明底层 client 不可用，需要重建
func isConnectionDead(conn *Connection) bool {
	if conn == nil || conn.Client == nil {
		return true
	}
	s, err := conn.Client.NewSession()
	if err != nil {
		return true
	}
	s.Close()
	return false
}

// reconnect 强制断开旧连接并重建
// 用于慢命令把共享连接拖死后的自愈：下一条命令触发重建，不必重开整个会话
func (m *Manager) reconnect(serverName string) (*Connection, error) {
	m.mu.Lock()
	old, exists := m.connections[serverName]
	if exists {
		closeSFTP(old)
		if old.Client != nil {
			old.Client.Close()
		}
		delete(m.connections, serverName)
	}
	m.mu.Unlock()

	// 重新连接（Connect 内部从 sessions + secretstore 重新建连）
	if _, err := m.Connect(serverName); err != nil {
		return nil, err
	}
	m.mu.RLock()
	conn := m.connections[serverName]
	m.mu.RUnlock()
	if conn == nil {
		return nil, fmt.Errorf("重连后连接仍不可用: %s", serverName)
	}
	return conn, nil
}

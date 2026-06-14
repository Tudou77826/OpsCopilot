package mcpserver

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// WhitelistManager 白名单管理器
type WhitelistManager struct {
	config      *WhitelistConfig
	configPath  string
	lastModTime time.Time // 上次成功加载时的文件 mtime,用于跳过无变化的 Reload
	mu          sync.RWMutex
}

// NewWhitelistManager 创建白名单管理器
func NewWhitelistManager(configPath string) (*WhitelistManager, error) {
	mgr := &WhitelistManager{
		configPath: configPath,
	}

	// 尝试加载配置文件
	if err := mgr.load(); err != nil {
		// 如果加载失败，使用默认配置
		mgr.config = DefaultWhitelistConfig()
		// 尝试保存默认配置
		_ = mgr.Save()
	} else if info, statErr := os.Stat(configPath); statErr == nil {
		// 记录初始 mtime,避免首次 ReloadIfChanged 又读一遍
		mgr.lastModTime = info.ModTime()
	}

	return mgr, nil
}

// load 从文件加载配置(无锁版本,调用方需自行加锁)
func (m *WhitelistManager) load() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("配置文件不存在")
		}
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config WhitelistConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	m.config = &config
	return nil
}

// Save 保存配置到文件（原子写:写临时文件 + rename）
func (m *WhitelistManager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeAtomicLocked()
}

// writeAtomicLocked 在已持有读锁或写锁时执行原子写
// 流程:写 .tmp → rename 到正式文件 → 刷新 mtime 缓存
func (m *WhitelistManager) writeAtomicLocked() error {
	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	tmpPath := m.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, m.configPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("重命名配置文件失败: %w", err)
	}

	// 刷新 mtime 缓存,避免本进程下次 ReloadIfChanged 又读一遍
	if info, statErr := os.Stat(m.configPath); statErr == nil {
		m.lastModTime = info.ModTime()
	}
	return nil
}

// GetConfig 获取当前配置
func (m *WhitelistManager) GetConfig() *WhitelistConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// Reload 强制重新从文件加载配置
func (m *WhitelistManager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reloadLocked()
}

// reloadLocked 在已持有写锁时强制重载
func (m *WhitelistManager) reloadLocked() error {
	if err := m.load(); err != nil {
		return err
	}
	if info, statErr := os.Stat(m.configPath); statErr == nil {
		m.lastModTime = info.ModTime()
	}
	return nil
}

// ReloadIfChanged 仅当文件 mtime 变化时才重新加载
// 用于热路径(ssh_exec)避免每次都做磁盘 IO
func (m *WhitelistManager) ReloadIfChanged() error {
	info, err := os.Stat(m.configPath)
	if err != nil {
		return fmt.Errorf("stat 配置文件失败: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if info.ModTime().Equal(m.lastModTime) {
		return nil
	}
	return m.reloadLocked()
}

// UpdateConfig 更新配置(原子写)
func (m *WhitelistManager) UpdateConfig(config *WhitelistConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = config
	return m.writeAtomicLocked()
}

// Check 检查命令是否允许执行
// serverIP 是服务器的 IP 地址，用于匹配策略
func (m *WhitelistManager) Check(command string, serverIP string) CheckResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	command = strings.TrimSpace(command)

	// 空命令
	if command == "" {
		return CheckResult{
			Allowed: false,
			Reason:  "命令不能为空",
		}
	}

	// 收集匹配该 IP 的所有策略
	var matchedPolicies []Policy
	for _, policy := range m.config.Policies {
		if matchesIPRange(serverIP, policy.IPRanges) {
			matchedPolicies = append(matchedPolicies, policy)
		}
	}

	// 在匹配的策略中检查命令
	for _, policy := range matchedPolicies {
		// 遍历策略中的命令
		for _, cmd := range policy.Commands {
			if !cmd.Enabled {
				continue
			}

			// 编译并匹配正则表达式
			matched, err := regexp.MatchString(cmd.Pattern, command)
			if err != nil {
				// 正则表达式无效，跳过
				continue
			}

			if matched {
				return CheckResult{
					Allowed:    true,
					Category:   cmd.Category,
					PolicyName: policy.Name,
				}
			}
		}
	}

	// 命令不在白名单中，返回详细的错误信息
	return CheckResult{
		Allowed: false,
		Reason:  formatDeniedMessage(command, serverIP, matchedPolicies),
	}
}

// matchesIPRange 检查 IP 是否匹配任意一个范围
func matchesIPRange(ip string, ranges []string) bool {
	for _, r := range ranges {
		if r == "*" {
			return true
		}

		// 尝试 CIDR 格式
		if strings.Contains(r, "/") {
			_, cidr, err := net.ParseCIDR(r)
			if err != nil {
				continue
			}
			parsedIP := net.ParseIP(ip)
			if parsedIP != nil && cidr.Contains(parsedIP) {
				return true
			}
		} else {
			// 单 IP 匹配
			if ip == r {
				return true
			}
		}
	}
	return false
}

// formatDeniedMessage 格式化拒绝消息
// 根据 serverIP 和匹配的策略生成详细的错误信息
func formatDeniedMessage(command string, serverIP string, matchedPolicies []Policy) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("命令 '%s' 不在服务器 IP 的白名单策略中。\n\n", command))
	sb.WriteString(fmt.Sprintf("当前服务器 IP: %s\n\n", serverIP))

	if len(matchedPolicies) == 0 {
		sb.WriteString("该 IP 未匹配任何白名单策略。\n")
		sb.WriteString("请在 OpsCopilot 配置中添加针对该 IP 段的策略。\n")
	} else {
		sb.WriteString("该 IP 匹配的策略:\n")
		for _, policy := range matchedPolicies {
			sb.WriteString(fmt.Sprintf("- %s (IP段: %s)\n", policy.Name, strings.Join(policy.IPRanges, ", ")))
			if len(policy.Commands) > 0 {
				sb.WriteString("  允许的命令:\n")
				for _, cmd := range policy.Commands {
					if cmd.Enabled {
						sb.WriteString(fmt.Sprintf("  - %s\n", cmd.Description))
					}
				}
			}
		}
		sb.WriteString("\n白名单策略是基于 IP 段配置的，不同服务器可能有不同的命令权限。\n")
		sb.WriteString("如需执行此命令，请联系管理员修改对应 IP 段的策略配置。\n")
	}

	return sb.String()
}

// GetPoliciesForIP 获取适用于指定 IP 的所有策略
func (m *WhitelistManager) GetPoliciesForIP(ip string) []Policy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Policy
	for _, policy := range m.config.Policies {
		if matchesIPRange(ip, policy.IPRanges) {
			result = append(result, policy)
		}
	}
	return result
}

// AddPolicy 添加新策略
func (m *WhitelistManager) AddPolicy(policy Policy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查 ID 是否重复
	for _, p := range m.config.Policies {
		if p.ID == policy.ID {
			return fmt.Errorf("策略 ID '%s' 已存在", policy.ID)
		}
	}

	m.config.Policies = append(m.config.Policies, policy)
	return nil
}

// UpdatePolicy 更新策略
func (m *WhitelistManager) UpdatePolicy(policy Policy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, p := range m.config.Policies {
		if p.ID == policy.ID {
			m.config.Policies[i] = policy
			return nil
		}
	}

	return fmt.Errorf("策略 '%s' 不存在", policy.ID)
}

// DeletePolicy 删除策略
func (m *WhitelistManager) DeletePolicy(policyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, p := range m.config.Policies {
		if p.ID == policyID {
			m.config.Policies = append(m.config.Policies[:i], m.config.Policies[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("策略 '%s' 不存在", policyID)
}

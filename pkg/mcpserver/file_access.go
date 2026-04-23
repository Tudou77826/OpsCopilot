package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// FileAccessPolicy 文件访问策略
type FileAccessPolicy struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	IPRanges         []string `json:"ip_ranges"`
	ReadPaths        []string `json:"read_paths"`
	WritePaths       []string `json:"write_paths"`
	DeniedPaths      []string `json:"denied_paths"`
	AllowedLocalDirs []string `json:"allowed_local_dirs"`
	MaxReadBytes     int      `json:"max_read_bytes"`
	MaxWriteBytes    int      `json:"max_write_bytes"`
}

// FileAccessConfig 文件访问控制配置
type FileAccessConfig struct {
	Version  string            `json:"version"`
	Policies []FileAccessPolicy `json:"policies"`
}

// FileAccessCheckResult 文件访问检查结果
type FileAccessCheckResult struct {
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason"`
	PolicyName string `json:"policy_name,omitempty"`
}

// FileAccessChecker 文件访问检查器
type FileAccessChecker struct {
	config     *FileAccessConfig
	configPath string
	mu         sync.RWMutex
}

// NewFileAccessChecker 创建文件访问检查器
func NewFileAccessChecker(configPath string) (*FileAccessChecker, error) {
	checker := &FileAccessChecker{
		configPath: configPath,
	}

	if err := checker.load(); err != nil {
		checker.config = DefaultFileAccessConfig()
		_ = checker.Save()
	}

	return checker, nil
}

// load 从文件加载配置
func (c *FileAccessChecker) load() error {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("配置文件不存在")
		}
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config FileAccessConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	c.config = &config
	return nil
}

// Save 保存配置到文件
func (c *FileAccessChecker) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.saveLocked()
}

func (c *FileAccessChecker) saveLocked() error {
	data, err := json.MarshalIndent(c.config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(c.configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// GetConfig 获取当前配置
func (c *FileAccessChecker) GetConfig() *FileAccessConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// UpdateConfig 更新配置
func (c *FileAccessChecker) UpdateConfig(config *FileAccessConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config = config
	return c.saveLocked()
}

// Reload 重新从文件加载配置
func (c *FileAccessChecker) Reload() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.load()
}

// CheckRead 检查远程文件读取权限
func (c *FileAccessChecker) CheckRead(remotePath string, localPath string, serverIP string, fileSize int64) FileAccessCheckResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.checkAccess(remotePath, localPath, serverIP, fileSize, "read")
}

// CheckWrite 检查远程文件写入权限
func (c *FileAccessChecker) CheckWrite(remotePath string, localPath string, serverIP string, fileSize int64) FileAccessCheckResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.checkAccess(remotePath, localPath, serverIP, fileSize, "write")
}

// checkAccess 统一的访问检查逻辑
func (c *FileAccessChecker) checkAccess(remotePath string, localPath string, serverIP string, fileSize int64, mode string) FileAccessCheckResult {
	// 1. 查找匹配的策略
	var matchedPolicy *FileAccessPolicy
	for i := range c.config.Policies {
		policy := &c.config.Policies[i]
		if matchesIPRange(serverIP, policy.IPRanges) {
			matchedPolicy = policy
			break
		}
	}

	if matchedPolicy == nil {
		return FileAccessCheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("服务器 IP %s 未匹配任何文件访问策略", serverIP),
		}
	}

	// 2. 检查本地路径安全性
	if localPath != "" {
		if !isPathAllowed(localPath, matchedPolicy.AllowedLocalDirs) {
			return FileAccessCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("本地路径 %s 不在允许的目录中（允许: %s）", localPath, strings.Join(matchedPolicy.AllowedLocalDirs, ", ")),
			}
		}
	}

	// 3. 检查拒绝路径（优先级最高）
	for _, denied := range matchedPolicy.DeniedPaths {
		if pathMatches(remotePath, denied) {
			return FileAccessCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("远程路径 %s 在拒绝列表中（拒绝规则: %s）", remotePath, denied),
			}
		}
	}

	// 4. 检查远程路径前缀
	if mode == "read" {
		if !isPathAllowed(remotePath, matchedPolicy.ReadPaths) {
			return FileAccessCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("远程路径 %s 不在允许读取的路径中", remotePath),
			}
		}
		// 5. 检查文件大小
		if matchedPolicy.MaxReadBytes > 0 && fileSize > int64(matchedPolicy.MaxReadBytes) {
			return FileAccessCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("文件大小 %d 字节超过下载上限 %d 字节", fileSize, matchedPolicy.MaxReadBytes),
			}
		}
	} else {
		if !isPathAllowed(remotePath, matchedPolicy.WritePaths) {
			return FileAccessCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("远程路径 %s 不在允许写入的路径中（写入路径需要管理员显式配置）", remotePath),
			}
		}
		// 5. 检查文件大小
		if matchedPolicy.MaxWriteBytes > 0 && fileSize > int64(matchedPolicy.MaxWriteBytes) {
			return FileAccessCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("文件大小 %d 字节超过上传上限 %d 字节", fileSize, matchedPolicy.MaxWriteBytes),
			}
		}
	}

	return FileAccessCheckResult{
		Allowed:    true,
		PolicyName: matchedPolicy.Name,
	}
}

// isPathAllowed 检查路径是否在允许的前缀列表中
// 使用 Unix 风格的路径分隔符（远程路径和测试路径都是 Unix 格式）
func isPathAllowed(path string, allowedPrefixes []string) bool {
	if len(allowedPrefixes) == 0 {
		return false
	}

	cleanPath := unixPathClean(path)
	for _, prefix := range allowedPrefixes {
		cleanPrefix := unixPathClean(prefix)
		if cleanPath == cleanPrefix || strings.HasPrefix(cleanPath, cleanPrefix+"/") {
			return true
		}
	}
	return false
}

// unixPathClean 清理 Unix 风格路径（不依赖 filepath.Clean，避免 Windows 路径问题）
func unixPathClean(path string) string {
	// 统一分隔符
	path = strings.ReplaceAll(path, "\\", "/")

	// 解析路径组件（处理 . 和 ..）
	parts := strings.Split(path, "/")
	var stack []string
	for _, part := range parts {
		switch part {
		case "", ".":
			// 跳过
		case "..":
			if len(stack) > 0 && stack[len(stack)-1] != "" {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, part)
		}
	}

	result := strings.Join(stack, "/")
	if !strings.HasPrefix(result, "/") && strings.HasPrefix(path, "/") {
		result = "/" + result
	}
	return result
}

// pathMatches 检查路径是否匹配模式（支持简单的通配符）
func pathMatches(path string, pattern string) bool {
	cleanPath := unixPathClean(path)
	cleanPattern := unixPathClean(pattern)

	// 精确匹配
	if cleanPath == cleanPattern {
		return true
	}

	// 前缀匹配（pattern 是目录）
	if strings.HasPrefix(cleanPath, cleanPattern+"/") {
		return true
	}

	// 简单通配符：/home/*/.ssh/id_* 形式
	if strings.Contains(cleanPattern, "*") {
		return globMatch(cleanPath, cleanPattern)
	}

	return false
}

// globMatch 简单的路径通配符匹配
func globMatch(path string, pattern string) bool {
	pathParts := strings.Split(path, "/")
	patternParts := strings.Split(pattern, "/")

	return globMatchParts(pathParts, patternParts, 0, 0)
}

func globMatchParts(pathParts []string, patternParts []string, pi int, patI int) bool {
	for patI < len(patternParts) && pi < len(pathParts) {
		pp := patternParts[patI]

		if pp == "*" {
			// 通配符：匹配任意单个路径段
			patI++
			pi++
			continue
		}

		if strings.Contains(pp, "*") {
			// 简单通配符匹配（如 id_*）
			if !simpleGlob(pathParts[pi], pp) {
				return false
			}
			patI++
			pi++
			continue
		}

		// 精确匹配
		if pathParts[pi] != pp {
			return false
		}
		patI++
		pi++
	}

	// 模式剩余部分
	for patI < len(patternParts) {
		if patternParts[patI] != "*" {
			return false
		}
		patI++
	}

	return pi == len(pathParts) && patI == len(patternParts)
}

// simpleGlob 简单通配符匹配（仅支持尾部 *）
func simpleGlob(s string, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return s == pattern
	}

	parts := strings.SplitN(pattern, "*", 2)
	prefix := parts[0]
	suffix := parts[1]

	if len(s) < len(prefix)+len(suffix) {
		return false
	}

	if prefix != "" && !strings.HasPrefix(s, prefix) {
		return false
	}

	if suffix != "" && !strings.HasSuffix(s, suffix) {
		return false
	}

	return true
}

// CheckLocalPath 检查本地路径安全性（不依赖具体策略）
func (c *FileAccessChecker) CheckLocalPath(localPath string, serverIP string) FileAccessCheckResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := range c.config.Policies {
		policy := &c.config.Policies[i]
		if matchesIPRange(serverIP, policy.IPRanges) {
			if isPathAllowed(localPath, policy.AllowedLocalDirs) {
				return FileAccessCheckResult{Allowed: true, PolicyName: policy.Name}
			}
			return FileAccessCheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("本地路径 %s 不在允许的目录中", localPath),
			}
		}
	}

	return FileAccessCheckResult{
		Allowed: false,
		Reason:  fmt.Sprintf("服务器 IP %s 未匹配任何文件访问策略", serverIP),
	}
}

// DefaultFileAccessConfig 返回默认文件访问配置
func DefaultFileAccessConfig() *FileAccessConfig {
	return &FileAccessConfig{
		Version: "1.0",
		Policies: []FileAccessPolicy{
			{
				ID:       "default",
				Name:     "Default File Access",
				IPRanges: []string{"*"},
				ReadPaths: []string{
					"/var/log/",
					"/etc/",
					"/tmp/",
					"/home/",
					"/opt/",
					"/srv/",
				},
				WritePaths:       []string{},
				DeniedPaths:      []string{"/etc/shadow", "/etc/ssh/", "/root/.ssh/", "/home/*/.ssh/id_*"},
				AllowedLocalDirs: []string{"/tmp/opscopilot-mcp/"},
				MaxReadBytes:     10 * 1024 * 1024,  // 10MB
				MaxWriteBytes:    5 * 1024 * 1024,   // 5MB
			},
		},
	}
}

// EnsureLocalStagingDir 确保本地暂存目录存在
func EnsureLocalStagingDir(dir string) error {
	// 在 Windows 上 /tmp 路径需要特殊处理
	if dir == "" {
		dir = "/tmp/opscopilot-mcp/"
	}
	return os.MkdirAll(dir, 0755)
}


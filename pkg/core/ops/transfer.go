package ops

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"opscopilot/pkg/core/security"
)

// ensureSFTP 确保连接有可用的 SFTP 客户端
// 首次调用时尝试创建，成功则缓存，失败则标记不可用
func (m *Manager) ensureSFTP(conn *Connection) (*sftp.Client, error) {
	conn.sftpMu.Lock()
	defer conn.sftpMu.Unlock()

	if conn.sftpClient != nil {
		return conn.sftpClient, nil
	}
	if conn.sftpTested && !conn.sftpAvailable {
		return nil, fmt.Errorf("该服务器不支持 SFTP 文件传输")
	}

	sshClient := conn.Client.SSHClient()
	if sshClient == nil {
		conn.sftpTested = true
		conn.sftpAvailable = false
		return nil, fmt.Errorf("SSH 连接不可用")
	}

	client, err := sftp.NewClient(sshClient)
	if err != nil {
		conn.sftpTested = true
		conn.sftpAvailable = false
		slog.Warn("sftp not available", "name", conn.Name, "error", err)
		return nil, fmt.Errorf("该服务器不支持 SFTP 文件传输: %w", err)
	}

	conn.sftpClient = client
	conn.sftpTested = true
	conn.sftpAvailable = true
	slog.Debug("SFTP client established", "name", conn.Name)
	return client, nil
}

// closeSFTP 关闭连接的 SFTP 客户端（线程安全）
func closeSFTP(conn *Connection) {
	conn.sftpMu.Lock()
	defer conn.sftpMu.Unlock()

	if conn.sftpClient != nil {
		conn.sftpClient.Close()
		conn.sftpClient = nil
	}
}

// DownloadOptions 下载选项
type DownloadOptions struct {
	LocalPath string // 本地落地路径
	MaxBytes  int    // 最大下载字节数，默认 10MB
}

// DownloadResult 下载结果
type DownloadResult struct {
	Success bool           `json:"success"`
	Meta    map[string]any `json:"meta"`
}

// Download 从远程服务器下载文件到本地
// 安全闸门内置：文件路径和大小必须通过 file_access 校验。
func (m *Manager) Download(serverName, remotePath string, opts DownloadOptions) (*DownloadResult, error) {
	if serverName == "" {
		return nil, fmt.Errorf("缺少 server 参数")
	}
	if remotePath == "" {
		return nil, fmt.Errorf("缺少 remote_path 参数")
	}
	localPath := opts.LocalPath
	if localPath == "" {
		return nil, fmt.Errorf("缺少 local_path 参数")
	}
	maxBytes := 10 * 1024 * 1024 // 默认 10MB
	if opts.MaxBytes > 0 {
		maxBytes = opts.MaxBytes
	}

	m.mu.RLock()
	conn, exists := m.connections[serverName]
	m.mu.RUnlock()

	// 若尚未连接，则自动连接（惰性）
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

	// === 安全闸门：文件访问校验（不可绕过）===
	if err := m.fileChecker.Reload(); err != nil {
		return nil, fmt.Errorf("加载文件访问配置失败: %w", err)
	}
	checkResult := m.fileChecker.CheckRead(remotePath, localPath, conn.Host, 0)
	if !checkResult.Allowed {
		return nil, fmt.Errorf("%s", checkResult.Reason)
	}

	sftpClient, err := m.ensureSFTP(conn)
	if err != nil {
		return nil, err
	}

	stat, err := sftpClient.Stat(remotePath)
	if err != nil {
		return nil, fmt.Errorf("无法获取远程文件信息: %w", err)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("远程路径 %s 是目录，不支持下载目录", remotePath)
	}

	fileSize := stat.Size()

	checkResult = m.fileChecker.CheckRead(remotePath, localPath, conn.Host, fileSize)
	if !checkResult.Allowed {
		return nil, fmt.Errorf("%s", checkResult.Reason)
	}

	// 覆盖 maxBytes（如果配置中更小）
	cfg := m.fileChecker.GetConfig()
	for _, policy := range cfg.Policies {
		if security.MatchesIPRange(conn.Host, policy.IPRanges) {
			if policy.MaxReadBytes > 0 && policy.MaxReadBytes < maxBytes {
				maxBytes = policy.MaxReadBytes
			}
			break
		}
	}

	if fileSize > int64(maxBytes) {
		return nil, fmt.Errorf("文件大小 %d 字节超过限制 %d 字节", fileSize, maxBytes)
	}

	// 创建本地目录
	localDir := filepath.Dir(localPath)
	if localDir != "." && localDir != "" {
		if err := os.MkdirAll(localDir, 0755); err != nil {
			return nil, fmt.Errorf("创建本地目录失败: %w", err)
		}
	}

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("打开远程文件失败: %w", err)
	}
	defer remoteFile.Close()

	localFile, err := os.Create(localPath)
	if err != nil {
		return nil, fmt.Errorf("创建本地文件失败: %w", err)
	}
	defer localFile.Close()

	written, err := io.Copy(localFile, io.LimitReader(remoteFile, int64(maxBytes)))
	if err != nil {
		os.Remove(localPath)
		return nil, fmt.Errorf("下载文件失败: %w", err)
	}

	conn.LastActive.Store(time.Now().UnixNano())

	slog.Info("file downloaded", "server", serverName, "remote", remotePath, "local", localPath, "bytes", written)

	return &DownloadResult{
		Success: true,
		Meta: map[string]any{
			"server":          serverName,
			"remote_path":     remotePath,
			"local_path":      localPath,
			"size":            written,
			"remote_mode":     fmt.Sprintf("%04o", stat.Mode().Perm()),
			"remote_mod_time": stat.ModTime().Format(time.RFC3339),
		},
	}, nil
}

// UploadOptions 上传选项
type UploadOptions struct {
	LocalPath string // 本地源文件路径
	Backup    bool   // 覆盖前自动备份远程文件，默认 true
	Mkdir     bool   // 自动创建远程目标目录，默认 false
}

// UploadResult 上传结果
type UploadResult struct {
	Success bool           `json:"success"`
	Meta    map[string]any `json:"meta"`
}

// Upload 从本地上传文件到远程服务器
// 安全闸门内置：文件路径和大小必须通过 file_access 校验。
func (m *Manager) Upload(serverName, remotePath string, opts UploadOptions) (*UploadResult, error) {
	if serverName == "" {
		return nil, fmt.Errorf("缺少 server 参数")
	}
	localPath := opts.LocalPath
	if localPath == "" {
		return nil, fmt.Errorf("缺少 local_path 参数")
	}
	if remotePath == "" {
		return nil, fmt.Errorf("缺少 remote_path 参数")
	}
	// 备份开关：由调用方显式控制（CLI flag 默认 true）
	backup := opts.Backup

	m.mu.RLock()
	conn, exists := m.connections[serverName]
	m.mu.RUnlock()

	// 若尚未连接，则自动连接（惰性）
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

	// === 安全闸门：文件访问校验（不可绕过）===
	if err := m.fileChecker.Reload(); err != nil {
		return nil, fmt.Errorf("加载文件访问配置失败: %w", err)
	}
	checkResult := m.fileChecker.CheckWrite(remotePath, localPath, conn.Host, 0)
	if !checkResult.Allowed {
		return nil, fmt.Errorf("%s", checkResult.Reason)
	}

	localInfo, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("本地文件不存在: %w", err)
	}
	if localInfo.IsDir() {
		return nil, fmt.Errorf("本地路径 %s 是目录，不支持上传目录", localPath)
	}
	fileSize := localInfo.Size()

	checkResult = m.fileChecker.CheckWrite(remotePath, localPath, conn.Host, fileSize)
	if !checkResult.Allowed {
		return nil, fmt.Errorf("%s", checkResult.Reason)
	}

	sftpClient, err := m.ensureSFTP(conn)
	if err != nil {
		return nil, err
	}

	// 备份远程文件（如果存在且 backup=true）
	backupPath := ""
	backupCreated := false
	if backup {
		if remoteStat, err := sftpClient.Stat(remotePath); err == nil && !remoteStat.IsDir() {
			backupPath = remotePath + ".bak." + time.Now().Format("20060102-150405")
			if err := sftpBackupFile(sftpClient, remotePath, backupPath); err != nil {
				return nil, fmt.Errorf("备份远程文件失败: %w", err)
			}
			backupCreated = true
			slog.Debug("remote file backed up", "src", remotePath, "dst", backupPath)
		}
	}

	// 创建远程目录
	if opts.Mkdir {
		remoteDir := remotePath
		if idx := strings.LastIndex(remotePath, "/"); idx > 0 {
			remoteDir = remotePath[:idx]
		}
		if err := sftpClient.MkdirAll(remoteDir); err != nil {
			return nil, fmt.Errorf("创建远程目录失败: %w", err)
		}
	}

	localFile, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer localFile.Close()

	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return nil, fmt.Errorf("创建远程文件失败: %w", err)
	}
	defer remoteFile.Close()

	written, err := io.Copy(remoteFile, localFile)
	if err != nil {
		return nil, fmt.Errorf("上传文件失败: %w", err)
	}

	conn.LastActive.Store(time.Now().UnixNano())

	slog.Info("file uploaded", "local", localPath, "server", serverName, "remote", remotePath, "bytes", written)

	return &UploadResult{
		Success: true,
		Meta: map[string]any{
			"server":         serverName,
			"local_path":     localPath,
			"remote_path":    remotePath,
			"bytes_written":  written,
			"backup_path":    backupPath,
			"backup_created": backupCreated,
		},
	}, nil
}

// sftpBackupFile 通过 SFTP 备份远程文件
func sftpBackupFile(client *sftp.Client, src string, dst string) error {
	srcFile, err := client.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := client.Create(dst)
	if err != nil {
		return fmt.Errorf("创建备份文件失败: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

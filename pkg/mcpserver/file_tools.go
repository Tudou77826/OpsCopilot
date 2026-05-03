package mcpserver

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

// ensureSFTP 确保连接有可用的 SFTP 客户端
// 首次调用时尝试创建，成功则缓存，失败则标记不可用
// 调用方持有的 conn.sftpMu 锁会在本函数返回后继续持有
func (s *Server) ensureSFTP(conn *Connection) (*sftp.Client, error) {
	conn.sftpMu.Lock()
	defer conn.sftpMu.Unlock()

	if conn.sftpClient != nil {
		return conn.sftpClient, nil
	}
	if conn.sftpTested && !conn.sftpAvailable {
		return nil, fmt.Errorf("该服务器不支持 SFTP 文件传输")
	}

	// 首次尝试创建 SFTP 客户端
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
		log.Printf("[MCP] SFTP not available for %s: %v", conn.Name, err)
		return nil, fmt.Errorf("该服务器不支持 SFTP 文件传输: %w", err)
	}

	conn.sftpClient = client
	conn.sftpTested = true
	conn.sftpAvailable = true
	log.Printf("[MCP] SFTP client established for %s", conn.Name)
	return client, nil
}

// toolFileTransfer 文件传输入口（根据 action 分发）
func (s *Server) toolFileTransfer(args map[string]interface{}) (interface{}, error) {
	action, _ := args["action"].(string)
	switch action {
	case "download":
		return s.toolFileDownload(args)
	case "upload":
		return s.toolFileUpload(args)
	default:
		return nil, fmt.Errorf("未知 action: %s，可选: download, upload", action)
	}
}

// toolFileDownload 从远程服务器下载文件到本地
func (s *Server) toolFileDownload(args map[string]interface{}) (interface{}, error) {
	serverName, _ := args["server"].(string)
	if serverName == "" {
		return nil, fmt.Errorf("缺少 server 参数")
	}
	remotePath, _ := args["remote_path"].(string)
	if remotePath == "" {
		return nil, fmt.Errorf("缺少 remote_path 参数")
	}
	localPath, _ := args["local_path"].(string)
	if localPath == "" {
		return nil, fmt.Errorf("缺少 local_path 参数")
	}
	maxBytes := 10 * 1024 * 1024 // 默认 10MB
	if v, ok := args["max_bytes"].(float64); ok && int(v) > 0 {
		maxBytes = int(v)
	}

	// 获取连接
	s.mu.RLock()
	conn, exists := s.connections[serverName]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("服务器 '%s' 未连接，请先使用 server_connect 连接", serverName)
	}

	// 获取 SFTP 客户端
	sftpClient, err := s.ensureSFTP(conn)
	if err != nil {
		return nil, err
	}

	// Stat 远程文件
	stat, err := sftpClient.Stat(remotePath)
	if err != nil {
		return nil, fmt.Errorf("无法获取远程文件信息: %w", err)
	}

	if stat.IsDir() {
		return nil, fmt.Errorf("远程路径 %s 是目录，不支持下载目录", remotePath)
	}

	fileSize := stat.Size()

	// 完整的权限检查（包含大小）
	if s.fileChecker != nil {
		checkResult := s.fileChecker.CheckRead(remotePath, localPath, conn.Host, fileSize)
		if !checkResult.Allowed {
			return nil, fmt.Errorf("%s", checkResult.Reason)
		}
	}

	// 覆盖 maxBytes（如果配置中更小）
	if s.fileChecker != nil {
		cfg := s.fileChecker.GetConfig()
		for _, policy := range cfg.Policies {
			if matchesIPRange(conn.Host, policy.IPRanges) {
				if policy.MaxReadBytes > 0 && policy.MaxReadBytes < maxBytes {
					maxBytes = policy.MaxReadBytes
				}
				break
			}
		}
	}

	if fileSize > int64(maxBytes) {
		return nil, fmt.Errorf("文件大小 %d 字节超过限制 %d 字节", fileSize, maxBytes)
	}

	// 创建本地目录
	localDir := localPath
	if idx := strings.LastIndex(localPath, "/"); idx > 0 {
		localDir = localPath[:idx]
	} else if idx := strings.LastIndex(localPath, "\\"); idx > 0 {
		localDir = localPath[:idx]
	}
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return nil, fmt.Errorf("创建本地目录失败: %w", err)
	}

	// 下载文件
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
		// 清理不完整的文件
		os.Remove(localPath)
		return nil, fmt.Errorf("下载文件失败: %w", err)
	}

	// 更新连接活跃时间
	conn.LastActive.Store(time.Now().UnixNano())

	log.Printf("[MCP] File downloaded: %s:%s -> %s (%d bytes)", serverName, remotePath, localPath, written)

	return map[string]interface{}{
		"success": true,
		"meta": map[string]interface{}{
			"server":           serverName,
			"remote_path":      remotePath,
			"local_path":       localPath,
			"size":             written,
			"remote_mode":      fmt.Sprintf("%04o", stat.Mode().Perm()),
			"remote_mod_time":  stat.ModTime().Format(time.RFC3339),
		},
	}, nil
}

// toolFileUpload 从本地上传文件到远程服务器
func (s *Server) toolFileUpload(args map[string]interface{}) (interface{}, error) {
	serverName, _ := args["server"].(string)
	if serverName == "" {
		return nil, fmt.Errorf("缺少 server 参数")
	}
	localPath, _ := args["local_path"].(string)
	if localPath == "" {
		return nil, fmt.Errorf("缺少 local_path 参数")
	}
	remotePath, _ := args["remote_path"].(string)
	if remotePath == "" {
		return nil, fmt.Errorf("缺少 remote_path 参数")
	}
	backup := true
	if v, ok := args["backup"].(bool); ok {
		backup = v
	}
	makeDir := false
	if v, ok := args["mkdir"].(bool); ok {
		makeDir = v
	}

	// 获取连接
	s.mu.RLock()
	conn, exists := s.connections[serverName]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("服务器 '%s' 未连接，请先使用 server_connect 连接", serverName)
	}

	// 检查本地文件存在
	localInfo, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("本地文件不存在: %w", err)
	}
	if localInfo.IsDir() {
		return nil, fmt.Errorf("本地路径 %s 是目录，不支持上传目录", localPath)
	}
	fileSize := localInfo.Size()

	// 权限检查
	if s.fileChecker != nil {
		_ = s.fileChecker.Reload()
		checkResult := s.fileChecker.CheckWrite(remotePath, localPath, conn.Host, fileSize)
		if !checkResult.Allowed {
			return nil, fmt.Errorf("%s", checkResult.Reason)
		}
	}

	// 获取 SFTP 客户端
	sftpClient, err := s.ensureSFTP(conn)
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
			log.Printf("[MCP] Remote file backed up: %s -> %s", remotePath, backupPath)
		}
	}

	// 创建远程目录
	if makeDir {
		remoteDir := remotePath
		if idx := strings.LastIndex(remotePath, "/"); idx > 0 {
			remoteDir = remotePath[:idx]
		}
		if err := sftpClient.MkdirAll(remoteDir); err != nil {
			return nil, fmt.Errorf("创建远程目录失败: %w", err)
		}
	}

	// 打开本地文件
	localFile, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer localFile.Close()

	// 创建远程文件
	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return nil, fmt.Errorf("创建远程文件失败: %w", err)
	}
	defer remoteFile.Close()

	written, err := io.Copy(remoteFile, localFile)
	if err != nil {
		return nil, fmt.Errorf("上传文件失败: %w", err)
	}

	// 更新连接活跃时间
	conn.LastActive.Store(time.Now().UnixNano())

	log.Printf("[MCP] File uploaded: %s -> %s:%s (%d bytes)", localPath, serverName, remotePath, written)

	return map[string]interface{}{
		"success": true,
		"meta": map[string]interface{}{
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

// closeSFTP 关闭连接的 SFTP 客户端（线程安全）
func closeSFTP(conn *Connection) {
	conn.sftpMu.Lock()
	defer conn.sftpMu.Unlock()

	if conn.sftpClient != nil {
		conn.sftpClient.Close()
		conn.sftpClient = nil
	}
}

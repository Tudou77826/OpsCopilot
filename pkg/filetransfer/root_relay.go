package filetransfer

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

const defaultRelayBaseDir = "/tmp/opscopilot"

const (
	rootRelayOK   = "__RELAY_OK__"
	rootRelayFail = "__RELAY_FAIL__"
)

// rootShellSession holds a persistent su session on an SSH channel.
// Commands are serialized via cmdMu so only one runs at a time.
type rootShellSession struct {
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	cmdMu   sync.Mutex // serializes command execution
}

// RootRelayTransport implements file operations via su + /tmp/opscopilot/ relay directory.
// When root SSH direct login is disabled (PermitRootLogin=no), this transport uses
// the existing SSH connection with su escalation to access root-owned files.
//
// The su session is created once and reused across multiple commands to avoid
// the overhead of repeated SSH session + PTY + su setup (especially through bastion hosts).
type RootRelayTransport struct {
	sshClient    *ssh.Client
	rootPassword string
	loginUser    string // login username, used to chown relay directory for SCP access

	mu    sync.Mutex
	shell *rootShellSession // cached su session, nil if not yet created
}

func NewRootRelayTransport(client *ssh.Client, rootPassword string, loginUser string) *RootRelayTransport {
	return &RootRelayTransport{
		sshClient:    client,
		rootPassword: rootPassword,
		loginUser:    loginUser,
	}
}

// Close releases any cached root shell session.
func (t *RootRelayTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.shell != nil {
		_, _ = io.WriteString(t.shell.stdin, "exit\n")
		t.shell.session.Close()
		t.shell = nil
	}
}

// getSession returns the cached root shell session, creating one if needed.
func (t *RootRelayTransport) getSession(ctx context.Context) (*rootShellSession, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.shell != nil {
		return t.shell, nil
	}

	log.Printf("[RootRelay] 创建新 su 会话 (loginUser=%s)", t.loginUser)

	session, err := t.sshClient.NewSession()
	if err != nil {
		return nil, &TransferError{Code: ErrorCodeNetwork, Message: fmt.Sprintf("创建 SSH 会话失败: %s", err)}
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm", 50, 80, modes); err != nil {
		session.Close()
		return nil, &TransferError{Code: ErrorCodeNetwork, Message: fmt.Sprintf("请求 PTY 失败: %s", err)}
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, &TransferError{Code: ErrorCodeNetwork, Message: fmt.Sprintf("获取 stdin 失败: %s", err)}
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, &TransferError{Code: ErrorCodeNetwork, Message: fmt.Sprintf("获取 stdout 失败: %s", err)}
	}

	if err := session.Shell(); err != nil {
		session.Close()
		return nil, &TransferError{Code: ErrorCodeNetwork, Message: fmt.Sprintf("启动 shell 失败: %s", err)}
	}

	// Read initial prompt
	buf := make([]byte, 4096)
	_, _ = stdout.Read(buf)

	// If loginUser is empty, we are already the target user (e.g., connected as root).
	// Skip su entirely and use the shell directly.
	if t.loginUser == "" {
		log.Printf("[RootRelay] loginUser 为空，跳过 su 提权，直接使用当前 shell")
	} else {
		// Send "su -"
		if _, err := io.WriteString(stdin, "su -\n"); err != nil {
			session.Close()
			return nil, &TransferError{Code: ErrorCodeAuthFailed, Message: fmt.Sprintf("发送 su 命令失败: %s", err)}
		}

		// Wait for password prompt
		if err := waitForOutput(stdout, []string{"Password:", "密码："}, 10*time.Second); err != nil {
			session.Close()
			return nil, &TransferError{Code: ErrorCodeAuthFailed, Message: fmt.Sprintf("等待密码提示超时: %s", err)}
		}

		// Send root password
		if _, err := io.WriteString(stdin, t.rootPassword+"\n"); err != nil {
			session.Close()
			return nil, &TransferError{Code: ErrorCodeAuthFailed, Message: fmt.Sprintf("发送密码失败: %s", err)}
		}

		// Wait for root shell prompt (# or $)
		if err := waitForOutput(stdout, []string{"# ", "$ "}, 10*time.Second); err != nil {
			log.Printf("[RootRelay] su 认证失败: %v", err)
			session.Close()
			return nil, &TransferError{Code: ErrorCodeAuthFailed, Message: "su 认证失败或超时"}
		}
		log.Printf("[RootRelay] su 认证成功")
	}

	shell := &rootShellSession{
		session: session,
		stdin:   stdin,
		stdout:  stdout,
	}
	t.shell = shell
	log.Printf("[RootRelay] 会话创建成功")
	return shell, nil
}

// invalidateSession closes and discards the current root shell session.
func (t *RootRelayTransport) invalidateSession() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.shell != nil {
		log.Printf("[RootRelay] 会话失效，关闭并重建")
		t.shell.session.Close()
		t.shell = nil
	}
}

// runAsRoot executes a single command as root using a persistent su session.
// If the session is dead, it is invalidated and a fresh one is created on the next call.
func (t *RootRelayTransport) runAsRoot(ctx context.Context, cmd string) (string, error) {
	log.Printf("[RootRelay] 执行 root 命令: %s", cmd)
	shell, err := t.getSession(ctx)
	if err != nil {
		return "", err
	}

	shell.cmdMu.Lock()
	defer shell.cmdMu.Unlock()

	// Send the actual command with marker
	fullCmd := cmd + " && echo " + rootRelayOK + " || echo " + rootRelayFail
	if _, err := io.WriteString(shell.stdin, fullCmd+"\n"); err != nil {
		t.invalidateSession()
		return "", &TransferError{Code: ErrorCodeNetwork, Message: fmt.Sprintf("发送命令失败: %s", err)}
	}

	// Read output until we see the marker, with context cancellation support
	output, err := readUntilMarker(ctx, shell.stdout, rootRelayOK, rootRelayFail, 60*time.Second)
	if err != nil {
		t.invalidateSession()
		return "", err
	}

	if output.failed {
		log.Printf("[RootRelay] 命令执行失败: %s", output.raw)
		return "", &TransferError{Code: ErrorCodePermissionDenied, Message: fmt.Sprintf("root 命令执行失败: %s", output.raw)}
	}

	log.Printf("[RootRelay] 命令执行成功，输出长度=%d", len(output.raw))
	return output.raw, nil
}

type relayOutput struct {
	raw    string
	failed bool
}

// prepareRelayDir creates a unique relay directory under /tmp/opscopilot/.
// Returns the relay directory path and a cleanup function.
func (t *RootRelayTransport) prepareRelayDir(ctx context.Context) (relayDir string, cleanup func(), err error) {
	id := uuid.New().String()[:8]
	relayDir = defaultRelayBaseDir + "/" + id + "/"
	log.Printf("[RootRelay] 创建中转目录: %s", relayDir)

	// Ensure base directory exists
	if _, err := t.runAsRoot(ctx, "mkdir -p "+defaultRelayBaseDir); err != nil {
		return "", nil, &TransferError{Code: ErrorCodeRelayFailed, Message: fmt.Sprintf("创建中转目录失败: %s", err)}
	}

	// Create relay directory
	if _, err := t.runAsRoot(ctx, "mkdir -p "+relayDir); err != nil {
		return "", nil, &TransferError{Code: ErrorCodeRelayFailed, Message: fmt.Sprintf("创建中转目录失败: %s", err)}
	}

	// Chown relay directory to login user so SCP (running as login user) can write to it
	if t.loginUser != "" {
		if _, err := t.runAsRoot(ctx, "chown "+shellSingleQuote(t.loginUser)+" "+shellSingleQuote(relayDir)); err != nil {
			return "", nil, &TransferError{Code: ErrorCodeRelayFailed, Message: fmt.Sprintf("chown 中转目录失败: %s", err)}
		}
	}

	cleanup = func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = t.runAsRoot(cleanupCtx, "rm -rf "+shellSingleQuote(relayDir))
	}

	return relayDir, cleanup, nil
}

// checkTmpSpace checks if /tmp has enough space for the transfer.
func (t *RootRelayTransport) checkTmpSpace(ctx context.Context, requiredBytes int64) error {
	cmd := "df -B1 /tmp | tail -1 | awk '{print $4}'"
	output, err := t.runAsRoot(ctx, cmd)
	if err != nil {
		return nil // If we can't check, proceed anyway
	}
	avail, parseErr := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	if parseErr != nil {
		return nil
	}
	if avail < requiredBytes {
		return &TransferError{
			Code:    ErrorCodeRelayNoSpace,
			Message: fmt.Sprintf("/tmp 空间不足: 需要 %d 字节，可用 %d 字节", requiredBytes, avail),
		}
	}
	return nil
}

// emitStep sends a step notification through the progress callback.
func emitStep(progress func(Progress), step string) {
	if progress != nil {
		progress(Progress{BytesTotal: -1, Step: step})
	}
}

// Upload uploads a local file to a remote path via root relay.
func (t *RootRelayTransport) Upload(ctx context.Context, localPath, remotePath string, progress func(Progress)) (TransferResult, error) {
	log.Printf("[RootRelay] 上传开始: %s -> %s", localPath, remotePath)
	emitStep(progress, "正在检查本地文件...")

	// Get local file size for space check
	lp := filepath.Clean(localPath)
	st, err := os.Stat(lp)
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	fileSize := st.Size()

	// Check space
	emitStep(progress, "正在检查中转空间...")
	if err := t.checkTmpSpace(ctx, fileSize); err != nil {
		return TransferResult{}, err
	}

	log.Printf("[RootRelay] 文件大小=%d 字节", fileSize)

	// Prepare relay directory
	emitStep(progress, "正在创建中转目录...")
	relayDir, cleanup, err := t.prepareRelayDir(ctx)
	if err != nil {
		return TransferResult{}, err
	}
	defer cleanup()

	log.Printf("[RootRelay] 使用中转目录: %s", relayDir)

	// Upload to relay directory via SFTP (as normal user), fallback to SCP
	fileName := path.Base(normalizeRemotePath(remotePath))
	relayPath := relayDir + fileName

	// Try SFTP → SCP → base64 direct upload
	// Use a short timeout for SFTP/SCP attempts so we can fallback to base64
	// if they hang (SCP through bastion can hang indefinitely on ack).
	relayCtx, relayCancel := context.WithTimeout(ctx, 15*time.Second)
	defer relayCancel()

	emitStep(progress, "正在上传到中转目录...")
	sftpTr := NewSFTPTransport(t.sshClient)
	res, err := sftpTr.Upload(relayCtx, localPath, relayPath, progress)
	if err != nil {
		log.Printf("[RootRelay] SFTP 上传到中转目录失败: %v", err)
		// SFTP failed, fallback to SCP
		emitStep(progress, "SFTP 不可用，切换 SCP 上传...")
		scpTr := NewSCPTransport(t.sshClient)
		res, err = scpTr.Upload(relayCtx, localPath, relayPath, progress)
		if err != nil {
			// Both SFTP and SCP failed, fallback to base64 direct upload via su
			emitStep(progress, "SFTP/SCP 均不可用，使用 base64 直传...")
			log.Printf("[RootRelay] SFTP/SCP 均失败，降级为 base64 直传模式")
			cleanup() // clean relay dir, we don't need it
			return t.uploadViaBase64(ctx, lp, normalizeRemotePath(remotePath), fileSize, progress)
		}
	}

	// Copy from relay to target as root.
	emitStep(progress, "正在提权复制到目标路径...")
	// chown is best-effort (||true) but cp must succeed.
	parentDir := path.Dir(normalizeRemotePath(remotePath))
	cmd := fmt.Sprintf(
		"cp %s %s && (chown --reference=%s %s 2>/dev/null || true)",
		shellSingleQuote(relayPath),
		shellSingleQuote(normalizeRemotePath(remotePath)),
		shellSingleQuote(parentDir),
		shellSingleQuote(normalizeRemotePath(remotePath)),
	)
	if _, err := t.runAsRoot(ctx, cmd); err != nil {
		return TransferResult{}, err
	}

	log.Printf("[RootRelay] 上传完成: %s -> %s", localPath, remotePath)
	return res, nil
}

// uploadViaBase64 uploads a file by base64-encoding its content and writing
// it directly through the su session. This is the last-resort fallback when
// neither SFTP nor SCP is available.
func (t *RootRelayTransport) uploadViaBase64(ctx context.Context, lp string, remotePath string, fileSize int64, progress func(Progress)) (TransferResult, error) {
	const chunkSize = 4000 // base64 chunk size per command (safe for shell command length)

	data, err := os.ReadFile(lp)
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}

	total := int64(len(data))
	emitStep(progress, fmt.Sprintf("正在通过 base64 直传 (%d 字节)...", total))

	// For small files, send in one shot
	if total <= chunkSize {
		encoded := base64.StdEncoding.EncodeToString(data)
		cmd := fmt.Sprintf("echo '%s' | base64 -d > %s", encoded, shellSingleQuote(remotePath))
		if _, err := t.runAsRoot(ctx, cmd); err != nil {
			return TransferResult{}, err
		}
		if progress != nil {
			progress(Progress{BytesDone: total, BytesTotal: total})
		}
		log.Printf("[RootRelay] base64 直传完成: %s -> %s (%d 字节)", lp, remotePath, total)
		return TransferResult{Bytes: total}, nil
	}

	// For larger files, write in chunks using >> append
	// First chunk: truncate (>)
	// Subsequent chunks: append (>>)
	first := true
	offset := 0
	for offset < len(data) {
		select {
		case <-ctx.Done():
			return TransferResult{}, &TransferError{Code: ErrorCodeNetwork, Message: "操作已取消"}
		default:
		}

		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]
		encoded := base64.StdEncoding.EncodeToString(chunk)

		var cmd string
		if first {
			cmd = fmt.Sprintf("echo '%s' | base64 -d > %s", encoded, shellSingleQuote(remotePath))
			first = false
		} else {
			cmd = fmt.Sprintf("echo '%s' | base64 -d >> %s", encoded, shellSingleQuote(remotePath))
		}

		if _, err := t.runAsRoot(ctx, cmd); err != nil {
			return TransferResult{}, err
		}

		offset = end
		if progress != nil {
			progress(Progress{BytesDone: int64(offset), BytesTotal: total})
		}
	}

	log.Printf("[RootRelay] base64 分块直传完成: %s -> %s (%d 字节)", lp, remotePath, total)
	return TransferResult{Bytes: total}, nil
}

// downloadViaBase64 downloads a remote file by reading its base64-encoded content
// through the su session. This is the last-resort fallback when neither SFTP nor SCP is available.
func (t *RootRelayTransport) downloadViaBase64(ctx context.Context, remotePath string, localPath string, fileSize int64, progress func(Progress)) (TransferResult, error) {
	emitStep(progress, "正在通过 base64 直传下载...")
	rp := normalizeRemotePath(remotePath)

	// Use base64 encoding to transfer file content
	// For files we know the size of, limit base64 output accordingly
	maxBase64Len := ""
	if fileSize > 0 {
		maxBase64Len = fmt.Sprintf(" | head -c %d", fileSize*2+100) // base64 is ~1.37x larger
	}
	cmd := fmt.Sprintf("base64 %s%s", shellSingleQuote(rp), maxBase64Len)
	output, err := t.runAsRoot(ctx, cmd)
	if err != nil {
		return TransferResult{}, err
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(output))
	if err != nil {
		return TransferResult{}, &TransferError{Code: ErrorCodeUnknown, Message: fmt.Sprintf("base64 解码失败: %s", err)}
	}

	// Write to local file
	lp := filepath.Clean(localPath)
	if err := os.MkdirAll(filepath.Dir(lp), 0755); err != nil {
		return TransferResult{}, toTransferError(err)
	}
	if err := os.WriteFile(lp, decoded, 0644); err != nil {
		return TransferResult{}, toTransferError(err)
	}

	if progress != nil {
		progress(Progress{BytesDone: int64(len(decoded)), BytesTotal: int64(len(decoded))})
	}

	log.Printf("[RootRelay] base64 直传下载完成: %s -> %s (%d 字节)", remotePath, localPath, len(decoded))
	return TransferResult{Bytes: int64(len(decoded))}, nil
}

// Download downloads a remote file to a local path via root relay.
func (t *RootRelayTransport) Download(ctx context.Context, remotePath, localPath string, progress func(Progress)) (TransferResult, error) {
	log.Printf("[RootRelay] 下载开始: %s -> %s", remotePath, localPath)
	emitStep(progress, "正在获取远程文件信息...")

	rp := normalizeRemotePath(remotePath)

	// Get remote file size
	statOutput, err := t.runAsRoot(ctx, fmt.Sprintf("stat -c '%%s' %s", shellSingleQuote(rp)))
	if err != nil {
		return TransferResult{}, err
	}
	fileSize, _ := strconv.ParseInt(strings.TrimSpace(statOutput), 10, 64)

	// Check space
	if fileSize > 0 {
		emitStep(progress, "正在检查中转空间...")
		if err := t.checkTmpSpace(ctx, fileSize); err != nil {
			return TransferResult{}, err
		}
	}

	// Prepare relay directory
	emitStep(progress, "正在创建中转目录...")
	relayDir, cleanup, err := t.prepareRelayDir(ctx)
	if err != nil {
		return TransferResult{}, err
	}
	defer cleanup()

	// Copy to relay as root, preserving filename
	fileName := path.Base(rp)
	relayPath := relayDir + fileName

	emitStep(progress, "正在提权复制到中转目录...")
	cmd := fmt.Sprintf("cp %s %s && chmod 644 %s",
		shellSingleQuote(rp),
		shellSingleQuote(relayPath),
		shellSingleQuote(relayPath),
	)
	if _, err := t.runAsRoot(ctx, cmd); err != nil {
		return TransferResult{}, err
	}

	// Download from relay via SFTP (as normal user), fallback to SCP, then base64
	// Use a short timeout for SFTP/SCP attempts so we can fallback to base64
	// if they hang (SCP through bastion can hang indefinitely on ack).
	relayCtx, relayCancel := context.WithTimeout(ctx, 15*time.Second)
	defer relayCancel()

	emitStep(progress, "正在下载到本地...")
	sftpTr := NewSFTPTransport(t.sshClient)
	res, err := sftpTr.Download(relayCtx, relayPath, localPath, progress)
	if err != nil {
		// SFTP failed, fallback to SCP
		emitStep(progress, "SFTP 不可用，切换 SCP 下载...")
		scpTr := NewSCPTransport(t.sshClient)
		res, err = scpTr.Download(relayCtx, relayPath, localPath, progress)
		if err != nil {
			// Both SFTP and SCP failed, fallback to base64 direct download
			emitStep(progress, "SFTP/SCP 均不可用，使用 base64 直传下载...")
			log.Printf("[RootRelay] SFTP/SCP 下载均失败，降级为 base64 直传模式")
			cleanup() // clean relay dir
			return t.downloadViaBase64(ctx, normalizeRemotePath(remotePath), localPath, fileSize, progress)
		}
	}
	log.Printf("[RootRelay] 下载完成: %s -> %s", remotePath, localPath)
	return res, nil
}

// List lists directory contents via su.
func (t *RootRelayTransport) List(ctx context.Context, remotePath string) ([]Entry, error) {
	p := normalizeRemotePath(remotePath)
	log.Printf("[RootRelay] 列出目录: %s", p)

	// Try find -printf first (more reliable), fall back to ls
	cmd := fmt.Sprintf(
		`find %s -maxdepth 1 -printf '%%F\t%%s\t%%Y\t%%f\n' 2>/dev/null`,
		shellSingleQuote(p),
	)
	output, err := t.runAsRoot(ctx, cmd)
	if err != nil {
		return nil, err
	}

	entries := parseFindOutput(output)
	if entries != nil {
		// Filter out empty name (the directory itself from find)
		filtered := make([]Entry, 0, len(entries))
		for _, e := range entries {
			if e.Name != "" {
				filtered = append(filtered, e)
			}
		}
		return filtered, nil
	}

	// Fall back to ls parsing
	lsCmd := fmt.Sprintf(`ls -la --time-style=long-iso %s`, shellSingleQuote(p))
	lsOutput, err := t.runAsRoot(ctx, lsCmd)
	if err != nil {
		return nil, err
	}
	return parseLsOutput(lsOutput, p)
}

// Stat gets file metadata via su.
func (t *RootRelayTransport) Stat(ctx context.Context, remotePath string) (Entry, error) {
	p := normalizeRemotePath(remotePath)

	cmd := fmt.Sprintf("stat -c '%%F\t%%s\t%%Y' %s", shellSingleQuote(p))
	output, err := t.runAsRoot(ctx, cmd)
	if err != nil {
		return Entry{}, err
	}

	return parseStatOutput(output, p)
}

// Mkdir creates a directory via su.
func (t *RootRelayTransport) Mkdir(ctx context.Context, remotePath string) error {
	p := normalizeRemotePath(remotePath)
	_, err := t.runAsRoot(ctx, "mkdir -p "+shellSingleQuote(p))
	return err
}

// Remove removes a file or directory via su.
func (t *RootRelayTransport) Remove(ctx context.Context, remotePath string, recursive bool) error {
	p := normalizeRemotePath(remotePath)
	cmd := "rm "
	if recursive {
		cmd += "-rf "
	} else {
		cmd += "-f "
	}
	cmd += shellSingleQuote(p)
	_, err := t.runAsRoot(ctx, cmd)
	return err
}

// Rename renames a file or directory via su.
func (t *RootRelayTransport) Rename(ctx context.Context, oldPath, newPath string) error {
	cmd := fmt.Sprintf("mv %s %s",
		shellSingleQuote(normalizeRemotePath(oldPath)),
		shellSingleQuote(normalizeRemotePath(newPath)),
	)
	_, err := t.runAsRoot(ctx, cmd)
	return err
}

// ReadFile reads a remote file's content via base64 encoding through su.
func (t *RootRelayTransport) ReadFile(ctx context.Context, remotePath string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}
	p := normalizeRemotePath(remotePath)
	log.Printf("[RootRelay] 读取文件: %s (maxBytes=%d)", p, maxBytes)

	// Use base64 encoding to safely transfer content
	cmd := fmt.Sprintf("base64 %s | head -c %d", shellSingleQuote(p), maxBytes*2)
	output, err := t.runAsRoot(ctx, cmd)
	if err != nil {
		return nil, err
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(output))
	if err != nil {
		return nil, &TransferError{Code: ErrorCodeUnknown, Message: fmt.Sprintf("base64 解码失败: %s", err)}
	}

	if int64(len(decoded)) > maxBytes {
		return nil, &TransferError{Code: ErrorCodeNotSupported, Message: "文件过大，暂不支持直接编辑"}
	}

	return decoded, nil
}

// WriteFile writes content to a remote file via base64 decoding through su.
func (t *RootRelayTransport) WriteFile(ctx context.Context, remotePath string, content []byte) error {
	p := normalizeRemotePath(remotePath)
	log.Printf("[RootRelay] 写入文件: %s (大小=%d 字节)", p, len(content))
	encoded := base64.StdEncoding.EncodeToString(content)

	// Write base64 content and decode on remote
	cmd := fmt.Sprintf("echo '%s' | base64 -d > %s", encoded, shellSingleQuote(p))
	_, err := t.runAsRoot(ctx, cmd)
	return err
}

// --- Internal helper functions ---

// waitForOutput reads from stdout until one of the expected strings is found or timeout.
func waitForOutput(stdout io.Reader, expected []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	accumulated := ""

	for time.Now().Before(deadline) {
		n, err := stdout.Read(buf)
		if n > 0 {
			accumulated += string(buf[:n])
			for _, exp := range expected {
				if strings.Contains(accumulated, exp) {
					return nil
				}
			}
		}
		if err != nil && err != io.EOF {
			continue
		}
	}

	return fmt.Errorf("timeout waiting for %v, got: %q", expected, accumulated)
}

// readUntilMarker reads output until RELAY_OK or RELAY_FAIL marker is found.
// Supports context cancellation — if ctx is done, returns ctx.Err() immediately.
func readUntilMarker(ctx context.Context, stdout io.Reader, okMarker, failMarker string, timeout time.Duration) (*relayOutput, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 8192)
	accumulated := ""

	type readResult struct {
		n   int
		err error
	}

	for time.Now().Before(deadline) {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, &TransferError{Code: ErrorCodeNetwork, Message: "操作已取消"}
		default:
		}

		// Read in a goroutine so we can select on ctx.Done()
		readCh := make(chan readResult, 1)
		go func() {
			n, err := stdout.Read(buf)
			readCh <- readResult{n, err}
		}()

		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		select {
		case <-ctx.Done():
			return nil, &TransferError{Code: ErrorCodeNetwork, Message: "操作已取消"}
		case result := <-readCh:
			if result.n > 0 {
				accumulated += string(buf[:result.n])
			}

			if strings.Contains(accumulated, okMarker) {
				raw := extractBeforeMarker(accumulated, okMarker)
				return &relayOutput{raw: raw, failed: false}, nil
			}
			if strings.Contains(accumulated, failMarker) {
				raw := extractBeforeMarker(accumulated, failMarker)
				return &relayOutput{raw: raw, failed: true}, nil
			}
		}
	}

	return nil, &TransferError{Code: ErrorCodeNetwork, Message: "等待命令执行结果超时"}
}

// extractBeforeMarker extracts the command output before the marker line.
func extractBeforeMarker(output, marker string) string {
	idx := strings.Index(output, marker)
	if idx < 0 {
		return strings.TrimSpace(output)
	}
	raw := output[:idx]
	// Remove the echoed command line (first line is usually the command echo)
	lines := strings.Split(raw, "\n")
	if len(lines) > 1 {
		return strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	return strings.TrimSpace(raw)
}

// parseFindOutput parses find -printf output: type\tsize\tmodtime\tname
func parseFindOutput(output string) []Entry {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return nil
	}

	entries := make([]Entry, 0, len(lines))
	hasValid := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 {
			// Not find output format, return nil to trigger ls fallback
			return nil
		}

		name := parts[3]
		if name == "" || name == "." || name == ".." {
			continue
		}

		size, _ := strconv.ParseInt(parts[1], 10, 64)
		modTimeSec, _ := strconv.ParseInt(parts[2], 10, 64)

		isDir := parts[0] == "directory"
		var mode uint32
		if isDir {
			mode = 0040755
		} else {
			mode = 0100644
		}

		hasValid = true
		entries = append(entries, Entry{
			Name:    name,
			IsDir:   isDir,
			Size:    size,
			Mode:    mode,
			ModTime: time.Unix(modTimeSec, 0),
		})
	}

	if !hasValid {
		return nil
	}
	return entries
}

// parseLsOutput parses ls -la --time-style=long-iso output.
// Format: perms links owner group size YYYY-MM-DD HH:MM name
func parseLsOutput(output, parentPath string) ([]Entry, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	entries := make([]Entry, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total") {
			continue
		}

		// Parse ls -la output: perms links owner group size date time name
		fields := strings.Fields(line)
		// long-iso format: perms links owner group size YYYY-MM-DD HH:MM name
		// Minimum 8 fields
		if len(fields) < 8 {
			continue
		}

		name := strings.Join(fields[7:], " ")
		if name == "." || name == ".." {
			continue
		}

		perms := fields[0]
		isDir := perms[0] == 'd'

		size, _ := strconv.ParseInt(fields[4], 10, 64)

		// Parse time: fields[5]=YYYY-MM-DD, fields[6]=HH:MM
		timeStr := fields[5] + "T" + fields[6]
		modTime, _ := time.Parse("2006-01-02T15:04", timeStr)

		var mode uint32
		if isDir {
			mode = 0040755
		} else {
			mode = 0100644
		}

		entryPath := joinRemote(parentPath, name)
		entries = append(entries, Entry{
			Path:    entryPath,
			Name:    name,
			IsDir:   isDir,
			Size:    size,
			Mode:    mode,
			ModTime: modTime,
		})
	}

	return entries, nil
}

// parseStatOutput parses stat -c '%F\t%s\t%Y' output.
func parseStatOutput(output, filePath string) (Entry, error) {
	parts := strings.SplitN(strings.TrimSpace(output), "\t", 3)
	if len(parts) != 3 {
		return Entry{}, &TransferError{Code: ErrorCodeNotFound, Message: "stat 解析失败: " + output}
	}

	size, _ := strconv.ParseInt(parts[1], 10, 64)
	modTimeSec, _ := strconv.ParseInt(parts[2], 10, 64)
	isDir := parts[0] == "directory"

	var mode uint32
	if isDir {
		mode = 0040755
	} else {
		mode = 0100644
	}

	return Entry{
		Path:    filePath,
		Name:    path.Base(filePath),
		IsDir:   isDir,
		Size:    size,
		Mode:    mode,
		ModTime: time.Unix(modTimeSec, 0),
	}, nil
}

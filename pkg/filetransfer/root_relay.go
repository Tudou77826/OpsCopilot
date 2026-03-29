package filetransfer

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
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

	mu    sync.Mutex
	shell *rootShellSession // cached su session, nil if not yet created
}

func NewRootRelayTransport(client *ssh.Client, rootPassword string) *RootRelayTransport {
	return &RootRelayTransport{
		sshClient:    client,
		rootPassword: rootPassword,
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
		session.Close()
		return nil, &TransferError{Code: ErrorCodeAuthFailed, Message: "su 认证失败或超时"}
	}

	shell := &rootShellSession{
		session: session,
		stdin:   stdin,
		stdout:  stdout,
	}
	t.shell = shell
	return shell, nil
}

// invalidateSession closes and discards the current root shell session.
func (t *RootRelayTransport) invalidateSession() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.shell != nil {
		t.shell.session.Close()
		t.shell = nil
	}
}

// runAsRoot executes a single command as root using a persistent su session.
// If the session is dead, it is invalidated and a fresh one is created on the next call.
func (t *RootRelayTransport) runAsRoot(ctx context.Context, cmd string) (string, error) {
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
		return "", &TransferError{Code: ErrorCodePermissionDenied, Message: fmt.Sprintf("root 命令执行失败: %s", output.raw)}
	}

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

	// Ensure base directory exists
	if _, err := t.runAsRoot(ctx, "mkdir -p "+defaultRelayBaseDir); err != nil {
		return "", nil, &TransferError{Code: ErrorCodeRelayFailed, Message: fmt.Sprintf("创建中转目录失败: %s", err)}
	}

	// Create relay directory
	if _, err := t.runAsRoot(ctx, "mkdir -p "+relayDir); err != nil {
		return "", nil, &TransferError{Code: ErrorCodeRelayFailed, Message: fmt.Sprintf("创建中转目录失败: %s", err)}
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

// Upload uploads a local file to a remote path via root relay.
func (t *RootRelayTransport) Upload(ctx context.Context, localPath, remotePath string, progress func(Progress)) (TransferResult, error) {
	// Get local file size for space check
	lp := filepath.Clean(localPath)
	st, err := os.Stat(lp)
	if err != nil {
		return TransferResult{}, toTransferError(err)
	}
	fileSize := st.Size()

	// Check space
	if err := t.checkTmpSpace(ctx, fileSize); err != nil {
		return TransferResult{}, err
	}

	// Prepare relay directory
	relayDir, cleanup, err := t.prepareRelayDir(ctx)
	if err != nil {
		return TransferResult{}, err
	}
	defer cleanup()

	// Upload to relay directory via SFTP (as normal user)
	fileName := path.Base(normalizeRemotePath(remotePath))
	relayPath := relayDir + fileName

	sftpTr := NewSFTPTransport(t.sshClient)
	res, err := sftpTr.Upload(ctx, localPath, relayPath, progress)
	if err != nil {
		return TransferResult{}, err
	}

	// Copy from relay to target as root.
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

	return res, nil
}

// Download downloads a remote file to a local path via root relay.
func (t *RootRelayTransport) Download(ctx context.Context, remotePath, localPath string, progress func(Progress)) (TransferResult, error) {
	rp := normalizeRemotePath(remotePath)

	// Get remote file size
	statOutput, err := t.runAsRoot(ctx, fmt.Sprintf("stat -c '%%s' %s", shellSingleQuote(rp)))
	if err != nil {
		return TransferResult{}, err
	}
	fileSize, _ := strconv.ParseInt(strings.TrimSpace(statOutput), 10, 64)

	// Check space
	if fileSize > 0 {
		if err := t.checkTmpSpace(ctx, fileSize); err != nil {
			return TransferResult{}, err
		}
	}

	// Prepare relay directory
	relayDir, cleanup, err := t.prepareRelayDir(ctx)
	if err != nil {
		return TransferResult{}, err
	}
	defer cleanup()

	// Copy to relay as root, preserving filename
	fileName := path.Base(rp)
	relayPath := relayDir + fileName

	cmd := fmt.Sprintf("cp %s %s && chmod 644 %s",
		shellSingleQuote(rp),
		shellSingleQuote(relayPath),
		shellSingleQuote(relayPath),
	)
	if _, err := t.runAsRoot(ctx, cmd); err != nil {
		return TransferResult{}, err
	}

	// Download from relay via SFTP (as normal user)
	sftpTr := NewSFTPTransport(t.sshClient)
	return sftpTr.Download(ctx, relayPath, localPath, progress)
}

// List lists directory contents via su.
func (t *RootRelayTransport) List(ctx context.Context, remotePath string) ([]Entry, error) {
	p := normalizeRemotePath(remotePath)

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

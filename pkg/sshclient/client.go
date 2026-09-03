package sshclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/remote"
)

// ConnectConfig 是 remote.ConnectConfig 的类型别名。
//
// 历史上 sshclient 自定义 ConnectConfig 作为 sessions.json 持久化结构;
// 引入 remote 抽象层后,协议无关配置收敛到 remote.ConnectConfig。本别名
// 保证现有引用 sshclient.ConnectConfig 的代码零改动,且 JSON tag 完全
// 一致(下划线风格),sessions.json 向后兼容。
type ConnectConfig = remote.ConnectConfig

// init 在包初始化时向 remote 注册 SSH 协议的 Dialer。
// 这样 remote.Dial(cfg) 在 Protocol=="ssh"(或空)时分派到 NewClient,
// 上层无需 import sshclient,打破循环依赖。
func init() {
	remote.RegisterDialer(remote.ProtocolSSH, func(cfg *remote.ConnectConfig) (remote.Connection, error) {
		c, err := NewClient(cfg)
		if err != nil {
			return nil, err
		}
		return c, nil
	})
}

// 编译期接口断言:确保 *Client 实现 remote.Connection 与 remote.SFTPCapable。
// 若接口签名漂移,此处编译失败,提前暴露问题。
var (
	_ remote.Connection  = (*Client)(nil)
	_ remote.SFTPCapable = (*Client)(nil)
)

type Client struct {
	client *ssh.Client

	// lastSessionMu 保护 lastSession:StartShell 写入,Resize 读取,
	// 这两个调用可能来自不同 goroutine(app.go 的 read loop 与
	// ResizeTerminal 调用并发)。
	lastSessionMu sync.Mutex
	lastSession   *ssh.Session
}

func (c *Client) SSHClient() *ssh.Client {
	return c.client
}

func NewClient(config *ConnectConfig) (*Client, error) {
	slog.Info("ssh connecting", "host", config.Host, "port", config.Port, "user", config.User, "bastion", config.Bastion != nil)

	// 递归建立 Bastion 连接
	var bastionClient *Client // Change type to *Client to use dialViaNetcat
	if config.Bastion != nil {
		bastion, err := NewClient(config.Bastion)
		if err != nil {
			slog.Error("ssh bastion connect failed", "host", config.Bastion.Host, "error", err)
			return nil, fmt.Errorf("failed to connect to bastion: %w", err)
		}
		bastionClient = bastion
	}

	authMethods := []ssh.AuthMethod{}
	if config.Password != "" {
		authMethods = append(authMethods, ssh.Password(config.Password))
		authMethods = append(authMethods, ssh.KeyboardInteractive(
			func(user, instruction string, questions []string, echos []bool) (answers []string, err error) {
				answers = make([]string, len(questions))
				for i := range questions {
					answers[i] = config.Password
				}
				return answers, nil
			},
		))
	}

	sshConfig := &ssh.ClientConfig{
		User:            config.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // In production, use ssh.FixedHostKey or similar
		Timeout:         30 * time.Second,            // 增加超时时间
	}
	if config.HostKey != "" {
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(config.HostKey))
		if err != nil {
			return nil, fmt.Errorf("invalid pinned SSH host key")
		}
		sshConfig.HostKeyCallback = ssh.FixedHostKey(key)
	}

	// Handle IPv6 brackets if present
	host := config.Host
	if len(host) > 2 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}
	addr := net.JoinHostPort(host, fmt.Sprint(config.Port))

	var client *ssh.Client
	var err error

	if bastionClient != nil {
		// 通过 Bastion 建立连接
		// 优先尝试 TCP Forwarding (Dial)
		conn, err := bastionClient.client.Dial("tcp", addr)
		if err != nil {
			// 如果 Dial 失败（可能是 AllowTcpForwarding=no），尝试 netcat 模式
			// fmt.Printf("Bastion dial failed: %v. Retrying with netcat...\n", err)
			conn, err = bastionClient.dialViaConsole("tcp", addr)
			if err != nil {
				return nil, fmt.Errorf("failed to dial via bastion: %w", err)
			}
		}

		ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to create client conn: %w", err)
		}
		client = ssh.NewClient(ncc, chans, reqs)
	} else {
		// 直连
		client, err = ssh.Dial("tcp", addr, sshConfig)
		if err != nil {
			slog.Error("ssh direct connect failed", "host", config.Host, "error", err)
			return nil, fmt.Errorf("failed to dial: %w", err)
		}
	}

	slog.Info("ssh connected", "host", config.Host, "user", config.User, "bastion", config.Bastion != nil)
	return &Client{client: client}, nil
}

func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// RunWithContext 执行命令，支持 context 取消。
// 当 ctx 被取消（含超时）时，向远端进程发送 SIGKILL 并立即返回，
// 避免慢命令无限期阻塞、拖死共享的 SSH 连接。
// 返回的 error 在 ctx 取消时包含 context.Cause 信息，便于上层区分超时与正常失败。
func (c *Client) RunWithContext(ctx context.Context, cmd string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("client is not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdoutBuf bytes.Buffer
	session.Stdout = &stdoutBuf

	// Start 是非阻塞的，Wait 才阻塞等待命令结束
	if err := session.Start(cmd); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// 用一个 channel 接收 Wait 的结果
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- session.Wait()
	}()

	select {
	case err := <-waitErr:
		// 命令正常结束（成功或命令自身失败）
		if err != nil {
			return stdoutBuf.String(), fmt.Errorf("failed to run command: %w", err)
		}
		return stdoutBuf.String(), nil
	case <-ctx.Done():
		// ctx 超时/取消：杀掉远端进程，释放 session
		// Signal 失败也不能阻塞返回，尽力清理
		_ = session.Signal(ssh.SIGKILL)
		// 等待 Wait goroutine 退出，避免泄漏（给一个短超时）
		select {
		case <-waitErr:
		case <-time.After(3 * time.Second):
		}
		return stdoutBuf.String(), fmt.Errorf("command timed out: %w", ctx.Err())
	}
}

// NewSession 暴露创建 session 的能力，供上层做连接健康检查。
// 如果 client 内部连接已失效，NewSession 会返回错误。
func (c *Client) NewSession() (*ssh.Session, error) {
	if c.client == nil {
		return nil, fmt.Errorf("client is not connected")
	}
	return c.client.NewSession()
}

// === remote.Connection 接口适配方法 ===
//
// 以下方法实现 pkg/remote.Connection 接口。各方法内部转发到 SSH 的等价
// 语义,保证 SSH 路径行为与重构前一致。

// Resize 实现 remote.Connection:调整 PTY 尺寸。
// 转发到最近一次 StartShell 建立的 session 的 WindowChange。
// 若尚无活跃 session(未调过 StartShell),静默忽略 —— 与历史行为一致
// (历史 Resize 由 session.Manager 直接调 SSHSession.WindowChange,
// session 为空时也忽略)。
func (c *Client) Resize(cols, rows int) error {
	c.lastSessionMu.Lock()
	session := c.lastSession
	c.lastSessionMu.Unlock()
	if session == nil {
		return nil
	}
	return session.WindowChange(rows, cols)
}

// Run 实现 remote.Connection:执行单条命令,返回 stdout。
// 包装 RunWithContext(用 context.Background),保持与历史 Run 方法等价。
func (c *Client) Run(ctx context.Context, cmd string) (string, error) {
	return c.RunWithContext(ctx, cmd)
}

// Healthy 实现 remote.Connection:探测连接是否健康。
// 通过试建一个 session 判断底层 client 是否可用;失败返回 false。
// 与 ops.isConnectionDead 的历史探活逻辑等价(那里也用 NewSession)。
func (c *Client) Healthy() bool {
	if c.client == nil {
		return false
	}
	s, err := c.NewSession()
	if err != nil {
		return false
	}
	s.Close()
	return true
}

// Protocol 实现 remote.Connection:返回协议标识。
func (c *Client) Protocol() string {
	return remote.ProtocolSSH
}

// SFTPClient 实现 remote.SFTPCapable:返回底层 SSH 连接上的 SFTP 客户端。
// 每次调用新建一个 sftp.Client;调用方负责 Close。
// 文件传输路径(ops/transfer.go、app.go SFTP 相关)通过类型断言查询本方法。
func (c *Client) SFTPClient() (*sftp.Client, error) {
	if c.client == nil {
		return nil, fmt.Errorf("client is not connected")
	}
	return sftp.NewClient(c.client)
}

// startShellSession 建立 PTY 会话,返回 session、stdin、合并后的 stdout。
// 内部将 session 记录到 c.lastSession,供 Resize 使用。
// 这是 SSH 协议专属的会话建立逻辑;公开的 StartShell(接口方法)和
// StartShellWithSudo 都复用本方法。
func (c *Client) startShellSession(cols, rows int) (*ssh.Session, io.WriteCloser, io.Reader, error) {
	if c.client == nil {
		return nil, nil, nil, fmt.Errorf("client is not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create session: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	if err := session.RequestPty("xterm", rows, cols, modes); err != nil {
		session.Close()
		return nil, nil, nil, fmt.Errorf("failed to request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, nil, nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Also capture stderr
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		return nil, nil, nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := session.Shell(); err != nil {
		session.Close()
		return nil, nil, nil, fmt.Errorf("failed to start shell: %w", err)
	}

	// 记录 session 供 Resize 使用(接口方法不再返回 *ssh.Session)。
	c.lastSessionMu.Lock()
	c.lastSession = session
	c.lastSessionMu.Unlock()

	// 并行合并 stdout 与 stderr。
	// 不能用 io.MultiReader：它是串行的（读完 stdout 到 EOF 才读 stderr），
	// 而交互式 PTY 下 stdout 永不自然 EOF，会导致 stderr 数据读不到，
	// 且在流边界返回伪 EOF，被读取读取循环误判为"远程关闭"（Issue #39）。
	// 改为并行合并：两路都真正结束时才返回 EOF，匹配 PTY 的真实语义。
	combinedReader := newMuxReader(stdout, stderr)

	return session, stdin, combinedReader, nil
}

// StartShell 实现 remote.Connection 接口:启动交互式 shell,返回三元组。
// 不再向调用方暴露 *ssh.Session —— session 由 Client 内部持有(供 Resize)。
// 行为与历史四元组版本等价,只是收敛了 session 的归属。
func (c *Client) StartShell(cols, rows int) (io.WriteCloser, io.Reader, error) {
	_, stdin, stdout, err := c.startShellSession(cols, rows)
	if err != nil {
		return nil, nil, err
	}
	return stdin, stdout, nil
}

// muxReader 并发读取多个 io.Reader，把数据汇合到一个流。
// 仅当所有 reader 都结束时才返回 io.EOF，避免任一路的瞬时无数据导致伪 EOF。
type muxReader struct {
	ch chan readResult
}

type readResult struct {
	buf []byte
	err error
}

// newMuxReader 为给定 readers 启动独立的读取 goroutine，并行汇合输出。
// 每个 reader 拥有独立 buffer，数据拷贝后再送入 channel，避免并发写冲突。
func newMuxReader(readers ...io.Reader) *muxReader {
	m := &muxReader{ch: make(chan readResult)}
	var wg sync.WaitGroup
	for _, r := range readers {
		wg.Add(1)
		go func(r io.Reader) {
			defer wg.Done()
			buf := make([]byte, 32768)
			for {
				n, err := r.Read(buf)
				if n > 0 {
					// 拷贝一份再发送，避免下一轮 Read 覆盖 buffer
					cp := make([]byte, n)
					copy(cp, buf[:n])
					m.ch <- readResult{buf: cp}
				}
				if err != nil {
					// 任一路结束（含 EOF），该 goroutine 退出；
					// 不向下游传递错误，避免伪 EOF。
					return
				}
				// n==0 且 err==nil：流暂时无数据，短暂让出 CPU 避免忙循环
				if n == 0 {
					time.Sleep(5 * time.Millisecond)
				}
			}
		}(r)
	}
	// 所有 reader 结束后关闭 channel，使 Read 返回 io.EOF
	go func() {
		wg.Wait()
		close(m.ch)
	}()
	return m
}

// Read 实现 io.Reader：从汇合 channel 取数据填入 p。
// 只有当所有上游 reader 都结束（channel 关闭）时才返回 io.EOF。
func (m *muxReader) Read(p []byte) (int, error) {
	for res := range m.ch {
		if len(res.buf) > 0 {
			return copy(p, res.buf), nil
		}
		// 空 buf 的结果会被忽略（goroutine 不会发送此类），继续等下一条
	}
	// channel 关闭：所有 reader 都已结束
	return 0, io.EOF
}

type SudoHandler struct {
	RootPassword string
	Stdin        io.Writer
}

func (h *SudoHandler) Handle(data []byte) {
	if h.RootPassword == "" {
		return
	}
	s := string(data)
	// 简单的关键字匹配，可以根据需要优化正则
	sLower := strings.ToLower(s)
	if strings.Contains(s, "Password:") || strings.Contains(s, "密码：") || strings.Contains(sLower, "[sudo] password") {
		// 写入密码 + 回车
		h.Stdin.Write([]byte(h.RootPassword + "\n"))
	}
}

// AutoSudoReader 是一个包装器，用于在读取数据时触发 SudoHandler
type AutoSudoReader struct {
	Reader  io.Reader
	Handler *SudoHandler
}

func (r *AutoSudoReader) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	if n > 0 {
		// 异步处理，避免阻塞读取流
		// 注意：这里可能会有并发写入 Stdin 的问题，但在当前简单场景下，
		// StdinPipe 的 Write 是线程安全的（只要不是并发 Close）
		// 为了更严谨，最好在 Handler 内部加锁，或者确保 Stdin 的 Write 是安全的。
		// 在 ssh 包中，StdinPipe 返回的是一个 channel 包装的 writer，是并发安全的。
		go r.Handler.Handle(p[:n])
	}
	return n, err
}

// StartShellWithSudo 启动 shell 并(若提供 rootPassword)自动 su - 提权。
// SSH 路径专用(app.go SSH 分支调用),不进 remote 接口(sudo 是 SSH+root
// 密码的特定组合,telnet 无等价语义)。
// 返回三元组,与 StartShell 接口方法签名一致;session 仍由 Client 持有。
func (c *Client) StartShellWithSudo(cols, rows int, rootPassword string) (io.WriteCloser, io.Reader, error) {
	_, stdin, stdout, err := c.startShellSession(cols, rows)
	if err != nil {
		return nil, nil, err
	}

	if rootPassword != "" {
		handler := &SudoHandler{
			RootPassword: rootPassword,
			Stdin:        stdin,
		}
		wrappedStdout := &AutoSudoReader{
			Reader:  stdout,
			Handler: handler,
		}

		// 自动发送 su -
		go func() {
			time.Sleep(500 * time.Millisecond)
			stdin.Write([]byte("su -\n"))
		}()

		return stdin, wrappedStdout, nil
	}

	return stdin, stdout, nil
}

// ncConn implements net.Conn via ssh session and nc command
type ncConn struct {
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	addr    string
}

func (c *ncConn) Read(b []byte) (int, error) {
	return c.stdout.Read(b)
}

func (c *ncConn) Write(b []byte) (int, error) {
	return c.stdin.Write(b)
}

func (c *ncConn) Close() error {
	return c.session.Close()
}

func (c *ncConn) LocalAddr() net.Addr {
	return &addr{"127.0.0.1:0"}
}

func (c *ncConn) RemoteAddr() net.Addr {
	return &addr{c.addr}
}

func (c *ncConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *ncConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *ncConn) SetWriteDeadline(t time.Time) error {
	return nil
}

type addr struct {
	s string
}

func (a *addr) Network() string { return "tcp" }
func (a *addr) String() string  { return a.s }

// dialViaConsole attempts to establish a connection by executing commands on the remote server.
// It tries multiple tools in order: nc, ncat, netcat, and finally bash's /dev/tcp feature.
func (c *Client) dialViaConsole(network, addr string) (net.Conn, error) {
	if c.client == nil {
		return nil, fmt.Errorf("client is not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	host, port, _ := net.SplitHostPort(addr)

	// Robust command chain:
	// 1. nc (standard netcat)
	// 2. ncat (Nmap's netcat)
	// 3. netcat (Alternative name)
	// 4. bash /dev/tcp (Bash built-in networking)
	// 5. python3 (Standard library socket)
	// We use || to try the next one if the previous fails (command not found or execution error).
	cmd := fmt.Sprintf(
		"(nc %s %s 2>/dev/null) || (ncat %s %s 2>/dev/null) || (netcat %s %s 2>/dev/null) || (bash -c 'exec 3<>/dev/tcp/%s/%s; cat <&3 & cat >&3') || (python3 -c 'import sys,socket,select;s=socket.socket();s.connect((\"%s\",%s));\nwhile True:\n r,_,_=select.select([sys.stdin,s],[],[]);\n if s in r:d=s.recv(4096);(sys.stdout.buffer.write(d) if hasattr(sys.stdout,\"buffer\") else sys.stdout.write(d));sys.stdout.flush();\n if not d:break;\n if sys.stdin in r:d=(sys.stdin.buffer.read(4096) if hasattr(sys.stdin,\"buffer\") else sys.stdin.read(4096));s.send(d);\n if not d:break')",
		host, port,
		host, port,
		host, port,
		host, port,
		host, port,
	)

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, err
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, err
	}

	if err := session.Start(cmd); err != nil {
		session.Close()
		return nil, err
	}

	return &ncConn{
		session: session,
		stdin:   stdin,
		stdout:  stdout,
		addr:    addr,
	}, nil
}

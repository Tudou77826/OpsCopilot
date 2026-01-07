package sshclient

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type ConnectConfig struct {
	Name         string         `json:"name"`
	Host         string         `json:"host"`
	Port         int            `json:"port"`
	User         string         `json:"user"`
	Password     string         `json:"password"`
	RootPassword string         `json:"root_password"`
	Bastion      *ConnectConfig `json:"bastion"`
	Group        string         `json:"group,omitempty"` // UI Grouping
}

type Client struct {
	client *ssh.Client
}

func NewClient(config *ConnectConfig) (*Client, error) {
	// 递归建立 Bastion 连接
	var bastionClient *Client // Change type to *Client to use dialViaNetcat
	if config.Bastion != nil {
		bastion, err := NewClient(config.Bastion)
		if err != nil {
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
			return nil, fmt.Errorf("failed to dial: %w", err)
		}
	}

	return &Client{client: client}, nil
}

func (c *Client) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (c *Client) Run(cmd string) (string, error) {
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

	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("failed to run command: %w", err)
	}

	return stdoutBuf.String(), nil
}

func (c *Client) StartShell(cols, rows int) (*ssh.Session, io.WriteCloser, io.Reader, error) {
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

	// Combine stdout and stderr for simplicity in this reader
	combinedReader := io.MultiReader(stdout, stderr)

	return session, stdin, combinedReader, nil
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

func (c *Client) StartShellWithSudo(cols, rows int, rootPassword string) (*ssh.Session, io.WriteCloser, io.Reader, error) {
	session, stdin, stdout, err := c.StartShell(cols, rows)
	if err != nil {
		return nil, nil, nil, err
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

		return session, stdin, wrappedStdout, nil
	}

	return session, stdin, stdout, nil
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

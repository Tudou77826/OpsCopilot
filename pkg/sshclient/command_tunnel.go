package sshclient

import (
	"bufio"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/connectiondiag"
)

//go:embed command_tunnel.py
var commandTunnelScript string

// DialDatabaseContext uses the standard channel first. Only a structured SSH
// policy rejection enables the command transport; SQL is never retried here.
func (c *Client) DialDatabaseContext(ctx context.Context, target string) (net.Conn, error) {
	connectiondiag.Event(ctx, "ssh.database.channel.start", "数据库目标=%s 类型=direct-tcpip", target)
	conn, err := c.client.DialContext(ctx, "tcp", target)
	if err == nil {
		connectiondiag.Event(ctx, "ssh.database.channel.ok", "数据库目标=%s TCP通道已确认", target)
		return conn, nil
	}
	RecordChannelFailure(ctx, "ssh.database.channel", err)
	var failure *ssh.OpenChannelError
	if !errors.As(err, &failure) || failure.Reason != ssh.Prohibited || ctx.Err() != nil {
		return nil, err
	}
	connectiondiag.Event(ctx, "ssh.database.auto.compatibility", "标准通道被明确拒绝，自动尝试同账号、同目标的SSH exec/python3通道；未发送数据库请求或SQL")
	conn, commandErr := c.DialCommandContext(ctx, target)
	if commandErr != nil {
		connectiondiag.Event(ctx, "ssh.database.command.failed", "%v", commandErr)
		return nil, errors.Join(err, commandErr)
	}
	connectiondiag.Event(ctx, "ssh.database.command.ok", "数据库目标=%s 兼容通道TCP已确认", target)
	return conn, nil
}

// DialCommandContext opens a raw, bounded-memory Python3 relay on this SSH node.
// No remote files, listener, PTY, shell retries or alternate targets are used.
// ctx bounds startup only; the returned connection is owned by its caller.
func (c *Client) DialCommandContext(ctx context.Context, target string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(target)
	if err != nil || host == "" || strings.ContainsAny(host, "\x00\r\n") {
		return nil, fmt.Errorf("远程命令通道的目标地址无效")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, fmt.Errorf("远程命令通道的目标端口无效")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	opening, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(nonce)
	deadline, _ := opening.Deadline()
	seconds := max(0.1, time.Until(deadline).Seconds())
	command := "exec python3 -u -c " + shellArgument(commandTunnelScript) + " " + shellArgument(host) + " " + shellArgument(port) + " " + shellArgument(strconv.FormatFloat(seconds, 'f', 3, 64)) + " " + shellArgument(token)
	connectiondiag.Event(ctx, "ssh.command.setup", "目标=%s 程序=python3 无PTY 启动超时<=10s 缓冲每方向<=256KiB；等待远端TCP就绪确认，不记录脚本输出或数据载荷", target)
	type result struct {
		conn *commandTunnelConn
		err  error
	}
	ready := make(chan result)
	var mu sync.Mutex
	var pending *commandTunnelConn
	go func() {
		session, err := c.client.NewSession()
		var conn *commandTunnelConn
		if err == nil {
			conn = &commandTunnelConn{session: session, target: target, token: token, exited: make(chan struct{})}
			mu.Lock()
			pending = conn
			mu.Unlock()
			if err = opening.Err(); err == nil {
				err = conn.start(command)
			}
			if err == nil {
				err = conn.awaitReady()
			}
			if err != nil {
				conn.Close()
			}
		}
		select {
		case ready <- result{conn, err}:
		case <-opening.Done():
			if conn != nil {
				conn.Close()
			}
		}
	}()
	select {
	case r := <-ready:
		if err = opening.Err(); err != nil {
			if r.conn != nil {
				r.conn.Close()
			}
			return nil, err
		}
		if r.err != nil {
			return nil, fmt.Errorf("远程命令通道启动失败（需要目标 SSH 节点提供 python3 并允许执行命令）: %w", r.err)
		}
		connectiondiag.Event(ctx, "ssh.command.ready", "目标=%s 远端已确认TCP连接；数据库协议尚未验证", target)
		return r.conn, nil
	case <-opening.Done():
		mu.Lock()
		conn := pending
		mu.Unlock()
		if conn != nil {
			go conn.Close()
		}
		return nil, fmt.Errorf("远程命令通道启动取消或超时: %w", opening.Err())
	}
}

func shellArgument(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

type commandTunnelConn struct {
	session       *ssh.Session
	input         io.WriteCloser
	reader        *bufio.Reader
	readMu        sync.Mutex
	target, token string
	exited        chan struct{}
	mu            sync.Mutex
	exitErr       error
	status        commandStatus
	statusBytes   int
	statusLine    []byte
	closeOnce     sync.Once
}

type commandStatus struct {
	Phase string `json:"phase"`
	Kind  string `json:"kind"`
	Errno *int   `json:"errno"`
}

// Write consumes bounded structured stderr. Unrelated shell/driver text is discarded.
func (c *commandTunnelConn) WriteStatus(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, b := range p {
		if c.statusBytes >= 8192 {
			break
		}
		c.statusBytes++
		if b == '\n' {
			prefix := "OC_TCP_ERROR " + c.token + " "
			if strings.HasPrefix(string(c.statusLine), prefix) {
				var status commandStatus
				if json.Unmarshal(c.statusLine[len(prefix):], &status) == nil && (status.Phase == "connect" || status.Phase == "relay") {
					// Do not accept arbitrary message text as a diagnostic field.
					switch status.Kind {
					case "ConnectionRefusedError", "TimeoutError", "timeout", "gaierror", "OSError", "PermissionError", "ConnectionResetError", "BrokenPipeError":
						c.status = status
					}
				}
			}
			c.statusLine = c.statusLine[:0]
		} else if len(c.statusLine) < 1024 {
			c.statusLine = append(c.statusLine, b)
		}
	}
	return len(p), nil
}

type commandStatusWriter struct{ conn *commandTunnelConn }

func (w commandStatusWriter) Write(p []byte) (int, error) { return w.conn.WriteStatus(p) }

func (c *commandTunnelConn) start(command string) error {
	input, err := c.session.StdinPipe()
	if err != nil {
		return err
	}
	c.input = input
	output, err := c.session.StdoutPipe()
	if err != nil {
		return err
	}
	c.reader = bufio.NewReaderSize(output, 4096)
	c.session.Stderr = commandStatusWriter{c}
	if err = c.session.Start(command); err != nil {
		return err
	}
	go func() {
		err := c.session.Wait()
		c.mu.Lock()
		c.exitErr = err
		c.mu.Unlock()
		close(c.exited)
	}()
	return nil
}

func (c *commandTunnelConn) awaitReady() error {
	want := "OC_TCP_READY " + c.token + "\n"
	for used := 0; used < 8192; {
		line, err := c.reader.ReadSlice('\n')
		used += len(line)
		if string(line) == want {
			return nil
		}
		if err != nil {
			return c.failure(err)
		}
	}
	return fmt.Errorf("远端未返回有效TCP就绪确认，启动输出超过上限")
}

func (c *commandTunnelConn) failure(fallback error) error {
	select {
	case <-c.exited:
	case <-time.After(200 * time.Millisecond):
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status.Kind != "" {
		if c.status.Errno != nil {
			return fmt.Errorf("远端阶段=%s 类型=%s errno=%d", c.status.Phase, c.status.Kind, *c.status.Errno)
		}
		return fmt.Errorf("远端阶段=%s 类型=%s", c.status.Phase, c.status.Kind)
	}
	if c.exitErr != nil {
		return fmt.Errorf("远端进程退出: %w", c.exitErr)
	}
	return fallback
}

func (c *commandTunnelConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	n, err := c.reader.Read(p)
	if errors.Is(err, io.EOF) {
		err = c.failure(err)
	}
	return n, err
}
func (c *commandTunnelConn) Write(p []byte) (int, error) { return c.input.Write(p) }
func (c *commandTunnelConn) CloseWrite() error           { return c.input.Close() }
func (c *commandTunnelConn) Close() error {
	c.closeOnce.Do(func() {
		_ = c.session.Signal(ssh.SIGTERM)
		_ = c.session.Close()
	})
	return nil
}
func (c *commandTunnelConn) LocalAddr() net.Addr  { return &addr{s: "ssh-exec-local"} }
func (c *commandTunnelConn) RemoteAddr() net.Addr { return &addr{s: c.target} }
func (*commandTunnelConn) SetDeadline(time.Time) error {
	return errors.New("SSH command channel deadlines unsupported; use operation context")
}
func (*commandTunnelConn) SetReadDeadline(time.Time) error {
	return errors.New("SSH command channel deadlines unsupported; use operation context")
}
func (*commandTunnelConn) SetWriteDeadline(time.Time) error {
	return errors.New("SSH command channel deadlines unsupported; use operation context")
}

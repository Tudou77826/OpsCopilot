// Package telnetclient 实现 remote.Connection 接口的 Telnet 协议版本。
//
// telnet 协议本身简单(基于 TCP + IAC 转义),无加密、无标准认证。
// 连接建立后,用户名/密码在 PTY 数据流里手工输入(由 LoginHandler 自动回填,
// 见方案 B)。终端尺寸通过 NAWS 子协商通知(RFC 1073)。
//
// 本包不依赖任何第三方 telnet 库:协议有界(IAC 状态机 + NAWS),
// 手写约 300 行即可覆盖需求,且零供应链风险。
package telnetclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"opscopilot/pkg/remote"
)

// init 向 remote 注册 Telnet 协议的 Dialer。
// 这样 remote.Dial(cfg) 在 Protocol=="telnet" 时分派到 NewClient,
// 上层无需 import telnetclient。
func init() {
	remote.RegisterDialer(remote.ProtocolTelnet, func(cfg *remote.ConnectConfig) (remote.Connection, error) {
		return NewClient(cfg)
	})
}

// 编译期接口断言:确保 *Client 实现 remote.Connection 与 remote.AutoLoginCapable。
var (
	_ remote.Connection     = (*Client)(nil)
	_ remote.AutoLoginCapable = (*Client)(nil)
)

// Client 是 Telnet 连接的客户端,实现 remote.Connection。
type Client struct {
	conn *telnetConn

	// lastShellMu 保护 shell 的 stdin/stdout 引用:StartShell 写入,
	// Resize/Run 读取,可能来自不同 goroutine。
	lastShellMu    sync.Mutex
	lastShellStdin io.WriteCloser

	// 初始尺寸(StartShell 时记录,NAWS 协商用)
	initCols int
	initRows int
}

// NewClient 建立 Telnet 连接。
//
// telnet 不支持 bastion(实践不走 SSH 堡垒机);若 config.Bastion 非 nil,
// 忽略并继续直连(与 UI 上 telnet 隐藏 bastion 字段一致)。
func NewClient(config *remote.ConnectConfig) (*Client, error) {
	if config == nil {
		return nil, fmt.Errorf("telnet: nil config")
	}
	port := config.Port
	if port == 0 {
		port = 23 // telnet 默认端口
	}
	addr := net.JoinHostPort(config.Host, fmt.Sprint(port))

	nc, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("telnet connect failed: %w", err)
	}

	tc := newTelnetConn(nc)
	return &Client{conn: tc}, nil
}

// StartShell 实现 remote.Connection:启动交互式 shell。
//
// telnet 没有 PTY 请求概念(TCP 连上即是一个 PTY 数据流),本方法主要做:
//  1. 发送初始协商(NAWS/SGA/ECHO);
//  2. 发送一次初始窗口尺寸(部分服务端不主动 DO NAWS,但仍接受 SB);
//  3. 返回 stdin(0xFF 自动转义)/ stdout(IAC 自动剥离)。
func (c *Client) StartShell(cols, rows int) (io.WriteCloser, io.Reader, error) {
	if c.conn == nil {
		return nil, nil, fmt.Errorf("telnet: not connected")
	}
	c.initCols = cols
	c.initRows = rows

	// 初始协商
	c.conn.initialNegotiation()
	c.conn.nawsAnnounced = true
	// 立即发一次初始尺寸(即便对端尚未回 DO,多数实现也接受)
	c.conn.sendNAWS(cols, rows)

	stdin := &telnetWriter{c: c.conn}
	stdout := &telnetReader{c: c.conn}

	c.lastShellMu.Lock()
	c.lastShellStdin = stdin
	c.lastShellMu.Unlock()

	return stdin, stdout, nil
}

// StartShellWithAutoLogin 实现 remote.AutoLoginCapable:
// 启动 shell 并在 stdout 上套一层 LoginHandler,自动回填用户名/密码。
// 数据原样透传(提示符仍会显示在前端),回填在后台异步进行。
func (c *Client) StartShellWithAutoLogin(cols, rows int, user, password string) (io.WriteCloser, io.Reader, error) {
	stdin, stdout, err := c.StartShell(cols, rows)
	if err != nil {
		return nil, nil, err
	}
	handler := newLoginHandler(stdin, user, password)
	wrapped := &autoLoginReader{reader: stdout, handler: handler}
	return stdin, wrapped, nil
}

// Resize 实现 remote.Connection:发送 NAWS 子协商通知新尺寸。
// 重复调用安全(每次窗口拖动都会触发)。
func (c *Client) Resize(cols, rows int) error {
	if c.conn == nil {
		return fmt.Errorf("telnet: not connected")
	}
	if !c.conn.nawsAnnounced {
		// 尚未声明 WILL NAWS(未调过 StartShell?),先声明再发尺寸
		c.conn.initialNegotiation()
		c.conn.nawsAnnounced = true
	}
	c.conn.sendNAWS(cols, rows)
	return nil
}

// Healthy 实现 remote.Connection:发送 IAC NOP 探活。
// NOP 不要求应答;若底层连接已关闭(写失败),返回 false。
// 真正的健康度最终由后续 Read 是否返回错误反映。
func (c *Client) Healthy() bool {
	if c.conn == nil {
		return false
	}
	return c.conn.sendNOP() == nil
}

// Close 实现 remote.Connection:关闭底层 TCP 连接。
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Protocol 实现 remote.Connection:返回协议标识。
func (c *Client) Protocol() string { return remote.ProtocolTelnet }

// Run 实现 remote.Connection:执行单条命令并返回 stdout。
//
// telnet 没有 SSH 那样的"单次 exec"语义(连上就是一个交互 shell)。
// 这里采用通用做法:连一个临时 shell → 发命令 + 标记 → 读到标记 → 关闭。
// 标记用 echo 打印唯一字符串,读到该字符串即认为命令输出结束。
//
// 注意:此实现是 best-effort。对于无登录提示的设备(如已配置免密),
// 或命令输出含标记字符串的极端情况,可能不完美。CLI exec 路径对 telnet
// 属辅助能力,主交互走 StartShell。
func (c *Client) Run(ctx context.Context, cmd string) (string, error) {
	if c.conn == nil {
		return "", fmt.Errorf("telnet: not connected")
	}
	// 临时关闭 read deadline(StartShell 可能设过),用 ctx 控制
	c.conn.SetDeadline(time.Time{})
	defer c.conn.SetDeadline(time.Time{})

	stdin, stdout, err := c.StartShell(c.initCols, c.initRows)
	if err != nil {
		return "", err
	}
	defer stdin.Close()

	// 唯一结束标记。用 echo 打印,读到该行表示命令已完成。
	marker := "__OPSCOPILOT_TELNET_CMD_END_4f8a__"

	// 发送:命令本身 + 换行,然后 echo 标记 + 换行
	fullCmd := cmd + "\r\necho " + marker + "\r\n"
	if _, werr := stdin.Write([]byte(fullCmd)); werr != nil {
		return "", fmt.Errorf("telnet run: write command failed: %w", werr)
	}

	// 读直到出现标记行或 ctx 超时
	var buf bytes.Buffer
	deadline, ok := ctx.Deadline()
	if !ok {
		// 默认 30s 兜底,避免无限等待
		deadline = time.Now().Add(30 * time.Second)
	}
	c.conn.SetDeadline(deadline)
	defer c.conn.SetDeadline(time.Time{})

	readBuf := make([]byte, 4096)
	done := false
	for !done {
		n, rerr := stdout.Read(readBuf)
		if n > 0 {
			buf.Write(readBuf[:n])
			// 检查是否出现标记
			if idx := strings.Index(buf.String(), marker); idx >= 0 {
				// 截掉标记及之后内容
				out := buf.String()[:idx]
				return cleanTelnetOutput(out, cmd), nil
			}
		}
		if rerr != nil {
			if ctx.Err() != nil {
				return cleanTelnetOutput(buf.String(), cmd), fmt.Errorf("telnet run: %w", ctx.Err())
			}
			// 连接关闭,返回已读到的内容
			return cleanTelnetOutput(buf.String(), cmd), nil
		}
	}
	return cleanTelnetOutput(buf.String(), cmd), nil
}

// cleanTelnetOutput 简单清理命令输出:
// 去掉回显的命令行本身(首行通常是命令回显),并 trim 空白。
// 这是 best-effort,telnet 回显行为因设备而异。
func cleanTelnetOutput(output, cmd string) string {
	lines := strings.Split(output, "\n")
	// 去掉首行若它包含命令回显
	if len(lines) > 0 && strings.Contains(strings.TrimSpace(lines[0]), strings.TrimSpace(cmd)) {
		lines = lines[1:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

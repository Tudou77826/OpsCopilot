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

// 编译期接口断言:确保 *Client 实现 remote.Connection、remote.AutoLoginCapable
// 与 remote.RunWarningReporter。
var (
	_ remote.Connection          = (*Client)(nil)
	_ remote.AutoLoginCapable    = (*Client)(nil)
	_ remote.RunWarningReporter  = (*Client)(nil)
)

// TakeRunWarnings 实现 remote.RunWarningReporter:返回并清空本次 Run 累积的
// 协议警示。ops.Manager.Exec 在 Run 后调用,填入 ExecResult.Warnings。
func (c *Client) TakeRunWarnings() []string {
	ws := c.runWarnings
	c.runWarnings = nil
	return ws
}

// addWarning 向本次 Run 追加一条协议警示(内部辅助)。
func (c *Client) addWarning(w string) {
	c.runWarnings = append(c.runWarnings, w)
}

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

	// runWarnings 累积本次 Run 的协议层警示(登录超时/echo 超时/输出噪声)。
	// 由 Run 写入,TakeRunWarnings 取走。Run 开始时清空(每次只反映本次)。
	// 单 goroutine 访问(ops.Manager.Exec 顺序调 Run + Take),无需加锁。
	runWarnings []string

	// 凭据:telnet 无协议层认证,登录在 PTY 数据流里完成。
	// Run(CLI exec)需要这些来自动登录,否则卡在 login 提示。
	// GUI 路径走 StartShellWithAutoLogin 显式传参,不读这两个字段。
	user     string
	password string
}

// NewClient 建立 Telnet 连接。
//
// telnet 不支持 bastion(实践不走 SSH 堡垒机);若 config.Bastion 非 nil,
// 忽略并继续直连(与 UI 上 telnet 隐藏 bastion 字段一致)。
//
// 保存 user/password 到 Client,供 Run(CLI exec)自动登录使用 ——
// telnet 无 SSH 的协议层认证,Run 必须先在数据流里完成登录才能发命令。
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
	return &Client{conn: tc, user: config.User, password: config.Password}, nil
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
// 实现流程:
//  1. StartShell;若配置了用户名,套 AutoLogin 并等登录完成(同步等待,
//     带 ctx 超时)。不登录则卡在 login 提示,命令被当成凭据吞掉。
//  2. 发命令 + echo 唯一标记 + 换行。
//  3. 读 stdout 直到出现标记(命令执行完才会 echo 标记)或 ctx 超时。
//  4. 截取标记前的内容,清理回显后返回。
//
// 局限(best-effort,协议本质无法完美解决):
//   - 依赖远端 shell 有 echo 命令(Linux/Unix 必有,部分精简 CLI 可能没有);
//   - 输出含 ANSI 码或 banner 时可能混入少量噪声。
// 主交互(GUI 终端)走 StartShell,不经过本方法,完全可靠。
func (c *Client) Run(ctx context.Context, cmd string) (string, error) {
	if c.conn == nil {
		return "", fmt.Errorf("telnet: not connected")
	}
	// 清空上次 Run 的警示(每次只反映本次)
	c.runWarnings = nil
	// telnet Run 天然有协议局限,无条件提醒下游 Agent 输出可能不纯
	c.addWarning("telnet 协议通过终端流执行命令,输出可能含 banner/提示符/回显等终端噪声(非纯净 stdout);依赖远端 shell 支持 echo 命令作为结束标记")
	// 临时关闭 read deadline(StartShell 可能设过),用 ctx 控制
	c.conn.SetDeadline(time.Time{})
	defer c.conn.SetDeadline(time.Time{})

	// 若有用户名,走 AutoLogin 并同步等待登录完成。
	// 否则设备免登录(或已认证),直接 StartShell。
	var loginDone <-chan struct{}
	stdin, stdout, err := c.StartShell(c.initCols, c.initRows)
	if err != nil {
		return "", err
	}
	defer stdin.Close()

	if c.user != "" {
		// 用 AutoLogin 装饰器包装 stdout,并等登录完成。
		handler := newLoginHandler(stdin, c.user, c.password)
		stdout = &autoLoginReader{reader: stdout, handler: handler}
		loginDone = handler.Done()
	}

	// 等登录完成(若有)。必须边读 stdout 边等 —— AutoLogin 的 Handle 在
	// Read 时异步触发,不读 stdout 则永远等不到 login/password 提示。
	// 读到的登录阶段数据(banner/提示符)丢弃,不混入命令输出。
	if loginDone != nil {
		loginDeadline := time.Now().Add(10 * time.Second)
		readTmp := make([]byte, 4096)
	waitLogin:
		for {
			// 设短 deadline 让 Read 不阻塞太久,好轮询 loginDone 和 ctx
			c.conn.SetDeadline(time.Now().Add(200 * time.Millisecond))
			n, _ := stdout.Read(readTmp) // 驱动 AutoLogin,数据丢弃
			_ = n
			select {
			case <-loginDone:
				break waitLogin
			default:
			}
			if time.Now().After(loginDeadline) {
				c.conn.SetDeadline(time.Time{})
				c.addWarning("登录阶段超时:10s 内未收到 login/password 提示。可能原因:设备提示符非标准(未匹配 login:/username:/password:)、凭据错误、或设备无需登录但配置了用户名")
				return "", fmt.Errorf("telnet run: 登录超时(未收到 login/password 提示)")
			}
			if ctx.Err() != nil {
				c.conn.SetDeadline(time.Time{})
				return "", fmt.Errorf("telnet run: %w (登录前被取消)", ctx.Err())
			}
		}
		c.conn.SetDeadline(time.Time{}) // 恢复,下面用 ctx deadline
	}

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
	// conn 读 deadline 略晚于 ctx,确保 ctx(权威超时源)先超时:当 Read 因
	// deadline 返回时 ctx.Err() 必非 nil,避免被误判为"连接关闭"分支(竞态)。
	c.conn.SetDeadline(deadline.Add(200 * time.Millisecond))
	defer c.conn.SetDeadline(time.Time{})

	readBuf := make([]byte, 4096)
	for {
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
				c.addWarning("命令执行超时:未读到 echo 结束标记。最可能原因:远端 shell 不支持 echo 命令(部分精简 CLI/老式网络设备),或命令本身耗时过长。返回的是已读到的部分输出,可能不完整")
				return cleanTelnetOutput(buf.String(), cmd), fmt.Errorf("telnet run: %w", ctx.Err())
			}
			// 连接关闭,返回已读到的内容
			c.addWarning("连接在读到 echo 结束标记前关闭。返回的是已读到的部分输出,可能不完整")
			return cleanTelnetOutput(buf.String(), cmd), nil
		}
	}
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

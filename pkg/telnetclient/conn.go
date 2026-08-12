package telnetclient

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

// telnetConn 是 telnet 连接的核心:包装 net.Conn,处理 IAC 转义与选项协商。
//
// 读写路径分离:
//   - 读(processInput):解析 IAC 命令,把协商命令分流处理(自动应答),
//     干净数据(已剥离 IAC)追加到 rbuf 供业务读取。
//   - 写(telnetWriter.Write):把数据里的 0xFF 转义为 0xFF 0xFF。
//
// 跨包的半截 IAC 命令会缓存到 pending,避免丢字节(详见 processInput 注释)。
type telnetConn struct {
	netConn net.Conn

	writeMu sync.Mutex // 保护写路径与 SendNAWS 串行化

	// 读路径状态(仅在读 goroutine / telnetReader.Read 中访问,无需加锁,
	// 因为 app.go 的 read loop 是单 goroutine 顺序读取)。
	rbuf    []byte // 已剥离 IAC 的干净数据缓冲
	pending []byte // 跨包未解析完的 IAC 命令缓存

	// 协商状态:记录我们已 WILL/DO 的选项,避免重复协商。
	nawsAnnounced bool // 是否已发过 WILL NAWS
	binary        bool // 是否进入 BINARY 传输(对端 DO Binary);为 true 时写路径跳过 NVT 字节规范化
	closed        chan struct{}
}

// newTelnetConn 在已建立的 net.Conn 上构造 telnet 连接。
// 调用方负责后续 StartShell 时的初始协商。
func newTelnetConn(nc net.Conn) *telnetConn {
	return &telnetConn{
		netConn: nc,
		closed:  make(chan struct{}),
	}
}

// initialNegotiation 在 shell 启动时发送初始协商。
// 策略:主动声明我们愿意做的(NAWS、SGA),并请求对方做的(SGA、ECHO)。
// 这些是现代交互式 telnet 终端的常规组合。
func (c *telnetConn) initialNegotiation() {
	c.sendRaw([]byte{cmdIAC, cmdWILL, optNAWS}) // 我愿意报告窗口尺寸
	c.sendRaw([]byte{cmdIAC, cmdWILL, optSGA})  // 我愿意抑制 GA
	c.sendRaw([]byte{cmdIAC, cmdDO, optSGA})    // 请你也抑制 GA
	c.sendRaw([]byte{cmdIAC, cmdDO, optEcho})   // 请你回显我发的字符
}

// sendNAWS 发送窗口尺寸子协商(RFC 1073):
//
//	IAC SB NAWS <cols_hi> <cols_lo> <rows_hi> <rows_lo> IAC SE
//
// cols/rows 为 16 位大端。payload 中的 0xFF 字节需转义为 0xFF 0xFF
// (虽然正常尺寸不会出现,但严格遵循 RFC)。0 值有特殊语义(表示该维度
// 未知/不限),不能过滤。
func (c *telnetConn) sendNAWS(cols, rows int) {
	var buf [8]byte
	binary.BigEndian.PutUint16(buf[0:2], uint16(cols))
	binary.BigEndian.PutUint16(buf[2:4], uint16(rows))
	// 构造带转义的 payload
	out := make([]byte, 0, 12)
	out = append(out, cmdIAC, cmdSB, optNAWS)
	for _, b := range buf[:4] {
		out = append(out, b)
		if b == cmdIAC {
			out = append(out, cmdIAC) // 0xFF 转义
		}
	}
	out = append(out, cmdIAC, cmdSE)
	c.sendRaw(out)
}

// sendRaw 加锁写底层连接(与 telnetWriter.Write 串行化,避免业务数据
// 与协商命令交错)。
func (c *telnetConn) sendRaw(b []byte) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.isClosed() {
		return
	}
	_, _ = c.netConn.Write(b)
}

// handleOption 处理对端发来的 WILL/WONT/DO/DONT 协商,决定我们的应答。
// 策略:对我们需要的能力(NAWS/SGA/ECHO/Binary)接受,其余统一拒绝,
// 避免被对端拖入不支持的协商。
func (c *telnetConn) handleOption(cmd, opt byte) {
	switch cmd {
	case cmdDO: // 对方要我们启用某选项
		switch opt {
		case optNAWS:
			// 已 WILL 过,收到 DO 时无需重复 WILL;尺寸由 Resize 单独发送。
			// 标记已声明,确保 Resize 生效。
			c.nawsAnnounced = true
		case optBinary:
			// 对端要求二进制传输:进入 BINARY 模式,写路径跳过 NVT 字节规范化。
			c.binary = true
			c.sendRaw([]byte{cmdIAC, cmdWILL, opt})
		case optSGA, optEcho:
			c.sendRaw([]byte{cmdIAC, cmdWILL, opt})
		default:
			c.sendRaw([]byte{cmdIAC, cmdWONT, opt}) // 拒绝不认识的
		}
	case cmdDONT:
		c.sendRaw([]byte{cmdIAC, cmdWONT, opt})
	case cmdWILL: // 对方主动提供某选项
		switch opt {
		case optSGA, optEcho, optBinary:
			c.sendRaw([]byte{cmdIAC, cmdDO, opt}) // 接受
		default:
			c.sendRaw([]byte{cmdIAC, cmdDONT, opt}) // 拒绝
		}
	case cmdWONT:
		c.sendRaw([]byte{cmdIAC, cmdDONT, opt})
	}
}

// processInput 解析从 net.Conn 读到的原始字节:
//   - 遇 IAC 命令 → 协商分流(handleOption),不进 rbuf;
//   - 遇 IAC IAC → 还原为单个 0xFF 数据字节,进 rbuf;
//   - 其余字节 → 直接进 rbuf。
//
// 跨包半截命令(如读到 "IAC" 但命令字节还没到)缓存到 pending,
// 与下一批数据合并解析,避免丢字节。
func (c *telnetConn) processInput(buf []byte) {
	// 合并上次残留的 pending
	if len(c.pending) > 0 {
		buf = append(c.pending, buf...)
		c.pending = nil
	}

	for i := 0; i < len(buf); {
		b := buf[i]
		if b != cmdIAC {
			c.rbuf = append(c.rbuf, b)
			i++
			continue
		}
		// 遇到 IAC,需要至少 2 字节才能判断
		if i+1 >= len(buf) {
			// IAC 是最后一个字节,缓存等下次
			c.pending = append(c.pending, buf[i:]...)
			return
		}
		cmd := buf[i+1]
		switch {
		case cmd == cmdIAC:
			// 转义:IAC IAC → 一个数据字节 0xFF
			c.rbuf = append(c.rbuf, cmdIAC)
			i += 2
		case cmd == cmdSB:
			// 子协商:IAC SB opt ... IAC SE
			end := indexIACSE(buf[i:])
			if end < 0 {
				// 不完整,缓存等下次
				c.pending = append(c.pending, buf[i:]...)
				return
			}
			// NAWS 由我们发送不接收;TTYPE 等子协商这里忽略内容(直接跳过)。
			// 如未来需要响应 TTYPE 请求,在此扩展。
			i += end + 2 // 跳过 IAC SE
		case isNegotiation(cmd):
			// 三字节协商:IAC cmd opt
			if i+2 >= len(buf) {
				c.pending = append(c.pending, buf[i:]...)
				return
			}
			opt := buf[i+2]
			c.handleOption(cmd, opt)
			i += 3
		default:
			// 两字节命令:IAC cmd(GA/NOP/EC/EL/AYT/AO/IP/BRK/DM 等)
			// 大部分可忽略;NOP 用于探活但无需应答。
			i += 2
		}
	}
}

// indexIACSE 在 buf 中查找 IAC SE 的起始位置(即 IAC 的下标),找不到返回 -1。
func indexIACSE(buf []byte) int {
	for i := 0; i+1 < len(buf); i++ {
		if buf[i] == cmdIAC && buf[i+1] == cmdSE {
			return i
		}
	}
	return -1
}

// drain 把 rbuf 中的干净数据拷贝给调用方。返回拷贝字节数。
func (c *telnetConn) drain(p []byte) int {
	n := copy(p, c.rbuf)
	c.rbuf = c.rbuf[n:]
	return n
}

// --- telnetReader / telnetWriter ---

// telnetReader 实现 io.Reader:从 net.Conn 读原始数据,经 processInput
// 剥离 IAC 后返回干净数据。阻塞读直到有数据或 EOF/错误。
type telnetReader struct{ c *telnetConn }

func (r *telnetReader) Read(p []byte) (int, error) {
	// 先吐缓冲
	if len(r.c.rbuf) > 0 {
		return r.c.drain(p), nil
	}
	// 缓冲空,从底层读一批
	raw := make([]byte, 4096)
	for {
		n, err := r.c.netConn.Read(raw)
		if n > 0 {
			r.c.processInput(raw[:n])
			if len(r.c.rbuf) > 0 {
				return r.c.drain(p), nil
			}
			// 处理完仍无干净数据(整批都是协商命令),继续读
		}
		if err != nil {
			// 即将返回错误,但仍可能 rbuf 有残留数据 —— 先吐完
			if len(r.c.rbuf) > 0 {
				return r.c.drain(p), nil
			}
			return 0, err
		}
	}
}

// telnetWriter 实现 io.WriteCloser:把数据里的 0xFF 转义后写入 net.Conn。
type telnetWriter struct {
	c      *telnetConn
	closed bool
	mu     sync.Mutex
}

func (w *telnetWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("telnet writer closed")
	}
	w.c.writeMu.Lock()
	defer w.c.writeMu.Unlock()
	if w.c.isClosed() {
		return 0, io.ErrClosedPipe
	}
	// 字节规范化:
	//   - 0xFF 始终转义为 0xFF 0xFF(IAC 转义,BINARY 模式也适用)。
	//   - NVT(非 BINARY)模式下额外适配传统 telnet / 网络设备:
	//       * DEL(0x7f) → BS(0x08):前端 Backspace 发 DEL,但网络设备行编器
	//         普遍以 Ctrl-H(0x08) 为 erase 字符(Issue #60)。
	//       * 单独 CR(0x0d,后非 LF/NUL) → CR LF:RFC 854 要求 CR 不能单独出现,
	//         前端 Enter 发 \r,规范化为 \r\n。
	out := make([]byte, 0, len(p)+8)
	binary := w.c.binary
	for i := 0; i < len(p); i++ {
		b := p[i]
		if b == cmdIAC {
			out = append(out, cmdIAC, cmdIAC)
			continue
		}
		if !binary {
			if b == 0x7f { // DEL → BS
				out = append(out, 0x08)
				continue
			}
			if b == 0x0d { // 单独 CR → CR LF
				if i+1 < len(p) && (p[i+1] == 0x0a || p[i+1] == 0x00) {
					out = append(out, 0x0d) // 已是规范 CR 序列(LF/NUL 紧随),保持原样
				} else {
					out = append(out, 0x0d, 0x0a) // 单独 CR(含末尾) → CR LF
				}
				continue
			}
		}
		out = append(out, b)
	}
	if _, err := w.c.netConn.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *telnetWriter) Close() error {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	return w.c.Close()
}

// --- 连接生命周期 ---

func (c *telnetConn) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *telnetConn) Close() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.isClosed() {
		return nil
	}
	close(c.closed)
	return c.netConn.Close()
}

// sendNOP 发送 IAC NOP,用于 Healthy() 探活。
// NOP 不会触发对端应答,仅用于"连接是否可写"的轻量探测;
// 真正的健康度由后续 Read 是否返回错误反映。
func (c *telnetConn) sendNOP() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.isClosed() {
		return io.ErrClosedPipe
	}
	_, err := c.netConn.Write([]byte{cmdIAC, cmdNOP})
	return err
}

// SetDeadline 透传到底层 net.Conn。
func (c *telnetConn) SetDeadline(t time.Time) error { return c.netConn.SetDeadline(t) }

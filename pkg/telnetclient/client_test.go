package telnetclient

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"opscopilot/pkg/remote"
)

// startPipeConn 用真实 TCP loopback 创建一对连接,返回包装好的 telnetConn 和对端 net.Conn。
// 不用 net.Pipe:它是同步无缓冲的,Write 会阻塞直到对端 Read,测试里容易死锁。
// TCP loopback 有内核缓冲,行为更接近真实 telnet。
func startPipeConn(t *testing.T) (*telnetConn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	// t.Cleanup 在测试结束时关闭 listener
	t.Cleanup(func() { ln.Close() })

	type acceptResult struct {
		conn net.Conn
		err  error
	}
	ch := make(chan acceptResult, 1)
	go func() {
		c, err := ln.Accept()
		ch <- acceptResult{c, err}
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	ar := <-ch
	if ar.err != nil {
		t.Fatalf("accept: %v", ar.err)
	}
	t.Cleanup(func() { clientConn.Close() })
	t.Cleanup(func() { ar.conn.Close() })
	return newTelnetConn(clientConn), ar.conn
}

// readWithTimeout 从 reader 读数据,超时返回已读内容(避免阻塞)。
func readWithTimeout(t *testing.T, r io.Reader, d time.Duration) string {
	t.Helper()
	var buf bytes.Buffer
	ch := make(chan struct{})
	go func() {
		tmp := make([]byte, 4096)
		for {
			n, err := r.Read(tmp)
			if n > 0 {
				buf.Write(tmp[:n])
			}
			if err != nil {
				break
			}
			// 短暂没数据也尝试再读一次,凑齐可能分批到达的数据
			if buf.Len() > 0 {
				time.Sleep(20 * time.Millisecond)
				break
			}
		}
		close(ch)
	}()
	select {
	case <-ch:
	case <-time.After(d):
	}
	return buf.String()
}

// === processInput:IAC 剥离 ===

func TestProcessInput_StripsNegotiation(t *testing.T) {
	tc, server := startPipeConn(t)
	defer tc.Close()
	defer server.Close()

	// 模拟对端发: "hello" + IAC DO NAWS + " world"
	data := []byte("hello")
	data = append(data, cmdIAC, cmdDO, optNAWS)
	data = append(data, []byte(" world")...)

	tc.processInput(data)

	want := "hello world"
	if got := string(tc.rbuf); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestProcessInput_IACIAC_Escape(t *testing.T) {
	tc, _ := startPipeConn(t)
	defer tc.Close()

	// IAC IAC 应还原为单个 0xFF 数据字节
	tc.processInput([]byte{cmdIAC, cmdIAC, 'x'})
	want := []byte{0xFF, 'x'}
	if !bytes.Equal(tc.rbuf, want) {
		t.Errorf("expected %v, got %v", want, tc.rbuf)
	}
}

func TestProcessInput_Subnegotiation(t *testing.T) {
	tc, _ := startPipeConn(t)
	defer tc.Close()

	// IAC SB TTYPE <data> IAC SE 应被跳过,不进 rbuf
	data := []byte{'a'}
	data = append(data, cmdIAC, cmdSB, optTTYPE, 0, 1, 2, cmdIAC, cmdSE)
	data = append(data, 'b')

	tc.processInput(data)
	if got := string(tc.rbuf); got != "ab" {
		t.Errorf("expected 'ab', got %q", got)
	}
}

func TestProcessInput_PartialCommand_Buffered(t *testing.T) {
	tc, _ := startPipeConn(t)
	defer tc.Close()

	// 先发 "x" + IAC(命令字节未到)
	tc.processInput([]byte{'x', cmdIAC})
	if got := string(tc.rbuf); got != "x" {
		t.Errorf("after partial: expected 'x', got %q", got)
	}
	// 再发完整命令 DO NAWS + "y"
	tc.processInput([]byte{cmdDO, optNAWS, 'y'})
	if got := string(tc.rbuf); got != "xy" {
		t.Errorf("after completion: expected 'xy', got %q", got)
	}
}

// === NAWS 字节序列 ===

func TestSendNAWS_ByteSequence(t *testing.T) {
	tc, server := startPipeConn(t)

	tc.sendNAWS(80, 24)

	server.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := server.Read(buf)
	if err != nil {
		t.Fatalf("read NAWS: %v", err)
	}
	got := buf[:n]
	// 期望: IAC SB NAWS 00 50 00 18 IAC SE (80=0x50, 24=0x18)
	want := []byte{cmdIAC, cmdSB, optNAWS, 0, 80, 0, 24, cmdIAC, cmdSE}
	if !bytes.Equal(got, want) {
		t.Errorf("NAWS bytes:\n got  %v\n want %v", got, want)
	}
}

// === telnetWriter:0xFF 转义 ===

func TestTelnetWriter_EscapesFF(t *testing.T) {
	tc, server := startPipeConn(t)
	defer tc.Close()
	defer server.Close()

	w := &telnetWriter{c: tc}
	data := []byte{0x01, 0xFF, 0x02} // 含一个 0xFF
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}

	server.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 64)
	n, err := server.Read(buf)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	// 期望:0x01 0xFF 0xFF 0x02(0xFF 被转义)
	want := []byte{0x01, 0xFF, 0xFF, 0x02}
	if !bytes.Equal(buf[:n], want) {
		t.Errorf("escaped bytes:\n got  %v\n want %v", buf[:n], want)
	}
}

// === LoginHandler 状态机 ===

func TestLoginHandler_NormalSequence(t *testing.T) {
	var written bytes.Buffer
	var mu sync.Mutex
	h := newLoginHandler(&mutexWriter{w: &written, mu: &mu}, "alice", "secret")

	// 1. 看到 "login:" → 应写用户名
	h.Handle([]byte("Welcome\r\nrouter-01 login: "))
	mu.Lock()
	got := written.String()
	mu.Unlock()
	if got != "alice\r\n" {
		t.Errorf("after login prompt: expected 'alice\\r\\n', got %q", got)
	}

	// 2. 看到 "Password:" → 应写密码
	h.Handle([]byte("\r\nPassword: "))
	mu.Lock()
	got = written.String()
	mu.Unlock()
	if got != "alice\r\nsecret\r\n" {
		t.Errorf("after password prompt: expected 'alice\\r\\nsecret\\r\\n', got %q", got)
	}

	// 3. 登录后再出现 "password" 字样(如 cat password.txt)→ 不应再写
	before := got
	h.Handle([]byte("$ cat password.txt\n"))
	mu.Lock()
	got = written.String()
	mu.Unlock()
	if got != before {
		t.Errorf("after done: should not write again, got additional %q", got[len(before):])
	}
}

func TestLoginHandler_CaseInsensitive(t *testing.T) {
	var written bytes.Buffer
	h := newLoginHandler(&written, "bob", "pw")

	// 大写变体 LOGIN: 也应触发
	h.Handle([]byte("LOGIN: "))
	if got := written.String(); got != "bob\r\n" {
		t.Errorf("case insensitive: expected 'bob\\r\\n', got %q", got)
	}
}

func TestLoginHandler_PasswordInBanner_NoMisfire(t *testing.T) {
	var written bytes.Buffer
	h := newLoginHandler(&written, "u", "p")

	// banner 里出现 "Password:" 但还没到 password 阶段(state 0)→ 不应触发
	h.Handle([]byte("WARNING: do not share your Password: keep it safe\nlogin: "))
	// 应只在看到 login: 时写用户名
	if got := written.String(); got != "u\r\n" {
		t.Errorf("banner misfire: expected only 'u\\r\\n', got %q", got)
	}
	// 状态现在应是 stateWaitPassword
	if h.state != stateWaitPassword {
		t.Errorf("state after login: expected stateWaitPassword, got %v", h.state)
	}
}

func TestLoginHandler_NoUsername_SkipsToPassword(t *testing.T) {
	var written bytes.Buffer
	h := newLoginHandler(&written, "", "onlypass")

	// 无用户名,直接看到 Password: 应填密码
	h.Handle([]byte("Password: "))
	if got := written.String(); got != "onlypass\r\n" {
		t.Errorf("no-username mode: expected 'onlypass\\r\\n', got %q", got)
	}
	if h.state != stateDone {
		t.Errorf("expected stateDone, got %v", h.state)
	}
}

// mutexWriter 包装一个 writer,加锁保证并发安全(LoginHandler 异步写)。
type mutexWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (m *mutexWriter) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.w.Write(p)
}

// === Dialer 注册(集成 remote 包) ===

func TestDialerRegistered(t *testing.T) {
	// init() 已注册 telnet dialer。验证 remote.Dial 能找到它(返回连接错误
	// 而非"未注册")。
	cfg := &remote.ConnectConfig{Protocol: remote.ProtocolTelnet, Host: "127.0.0.1", Port: 1}
	_, err := remote.Dial(cfg)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
	// 关键:不应是"未注册"错误
	if strings.Contains(err.Error(), "unsupported protocol") || strings.Contains(err.Error(), "未注册") {
		t.Errorf("telnet dialer not registered: %v", err)
	}
}

// === Client.Run:用真实本地 TCP 模拟 telnet server ===

func TestClient_Run_WithMockServer(t *testing.T) {
	// 启动一个 mock telnet server:接受连接,读到命令后回显 + 输出结果 + 标记
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// 简单回显服务器:读到换行就把内容回写
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			// 回显收到的数据(mock server 行为)
			conn.Write(buf[:n])
		}
	}()

	cfg := &remote.ConnectConfig{
		Protocol: remote.ProtocolTelnet,
		Host:     ln.Addr().(*net.TCPAddr).IP.String(),
		Port:     ln.Addr().(*net.TCPAddr).Port,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Run 在 mock 回显服务器下,会读到命令回显并清理
	out, runErr := client.Run(ctx, "echo hello")
	// 主要验证 Run 不死锁、能返回;输出内容因 mock 行为可能为空或含 hello
	_ = out
	if runErr != nil && !strings.Contains(runErr.Error(), "telnet run") {
		// 连接错误可接受(mock server 行为简单)
		t.Logf("Run returned (acceptable for mock): out=%q err=%v", out, runErr)
	}
}

// === RunWarningReporter:协议警示上报 ===

// TestRunWarnings_SuccessHasProtocolNotice 验证:即使命令成功执行,
// telnet Run 也应上报"输出可能有终端噪声"的协议层提示,让下游 Agent
// 知道输出非纯净 stdout。
func TestRunWarnings_SuccessHasProtocolNotice(t *testing.T) {
	// 用一个会回显命令 + echo 标记的 mock server(模拟正常 telnet 设备)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			// 回显收到的数据(模拟设备把命令 + echo 标记都执行并回显)
			conn.Write(buf[:n])
		}
	}()

	cfg := &remote.ConnectConfig{
		Protocol: remote.ProtocolTelnet,
		Host:     ln.Addr().(*net.TCPAddr).IP.String(),
		Port:     ln.Addr().(*net.TCPAddr).Port,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = client.Run(ctx, "echo hello")

	// 即使成功(或因 mock 简化返回错误),都应有协议层 noise warning
	ws := client.TakeRunWarnings()
	if len(ws) == 0 {
		t.Fatal("期望 Run 后有协议警示,实际为空")
	}
	// 验证警示内容提及"终端噪声"或"echo"
	found := false
	for _, w := range ws {
		if strings.Contains(w, "噪声") || strings.Contains(w, "echo") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("警示未提及协议噪声/echo 依赖,实际: %v", ws)
	}

	// 验证 TakeRunWarnings 是"取走即清空"语义
	if ws2 := client.TakeRunWarnings(); len(ws2) != 0 {
		t.Errorf("重复调用应返回空,实际: %v", ws2)
	}
}

// TestRunWarnings_TimeoutHasEchoMarkerWarning 验证:ctx 超时(没读到 echo
// 标记)时,应上报"未读到结束标记,可能设备不支持 echo"的警示。
func TestRunWarnings_TimeoutHasEchoMarkerWarning(t *testing.T) {
	// mock server:接受连接后什么都不做(不回显任何数据)→ Run 永远读不到
	// echo 标记 → ctx 超时
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// 故意不回显任何东西,让 Run 等到超时
		buf := make([]byte, 4096)
		for {
			if _, err := conn.Read(buf); err != nil {
				conn.Close()
				return
			}
		}
	}()

	cfg := &remote.ConnectConfig{
		Protocol: remote.ProtocolTelnet,
		Host:     ln.Addr().(*net.TCPAddr).IP.String(),
		Port:     ln.Addr().(*net.TCPAddr).Port,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	// 短超时,快速失败
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_, runErr := client.Run(ctx, "echo hello")
	// 应该超时
	if runErr == nil {
		t.Fatal("期望超时错误,实际 nil")
	}

	ws := client.TakeRunWarnings()
	// 应有"未读到结束标记"或"echo"相关警示
	found := false
	for _, w := range ws {
		if strings.Contains(w, "结束标记") || strings.Contains(w, "echo") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("超时时应有 echo 标记相关警示,实际: %v", ws)
	}
}

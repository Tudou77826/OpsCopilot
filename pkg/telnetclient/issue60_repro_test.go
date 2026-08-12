package telnetclient

// 该文件覆盖 Issue #60（telnet 模式下退格删除异常）及其同类字节兼容性问题。
// 原为复现测试，修复（conn.go telnetWriter.Write 的 NVT 字节规范化 +
// autologin.go 凭据改用 CRLF）后翻转为断言"修复后行为"，作回归保护。

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/remote"
)

// stripIACRepro 极简 IAC 剥离，供 mock server 过滤掉客户端的协商字节。
func stripIACRepro(p []byte) []byte {
	out := make([]byte, 0, len(p))
	for i := 0; i < len(p); {
		if p[i] != 0xFF {
			out = append(out, p[i])
			i++
			continue
		}
		if i+1 >= len(p) {
			return out
		}
		cmd := p[i+1]
		switch {
		case cmd == 0xFF: // IAC IAC -> 0xFF
			out = append(out, 0xFF)
			i += 2
		case cmd == 0xFA: // SB ... IAC SE
			j := i + 2
			moved := false
			for j+1 < len(p) {
				if p[j] == 0xFF && p[j+1] == 0xF0 {
					j += 2
					moved = true
					break
				}
				j++
			}
			if !moved {
				return out
			}
			i = j
		case cmd >= 0xFB && cmd <= 0xFE: // WILL/WONT/DO/DONT 三字节
			i += 3
		default: // 两字节命令
			i += 2
		}
	}
	return out
}

// startDeviceLikeServer 启动一个模拟 erase=Ctrl-H(0x08) 的网络设备 CLI 行编辑器。
// 行编辑规则（典型网络设备）：
//   - 0x08 (BS, Ctrl-H) = erase：删除行缓冲最后一个字符，回显 "\b \b"
//   - 0x7f (DEL)        : 不在 erase 定义内，设备忽略，不改行缓冲
//   - 0x0d (CR)         : 提交当前行缓冲，回显 "\r\nECHO:<buffer>\r\n"
//   - 其他可打印字节    : 追加到行缓冲并回显
func startDeviceLikeServer(t *testing.T) *net.TCPAddr {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var line []byte
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				data := stripIACRepro(buf[:n])
				for _, b := range data {
					switch {
					case b == 0x08: // BS = erase
						if len(line) > 0 {
							line = line[:len(line)-1]
							conn.Write([]byte("\b \b"))
						}
					case b == 0x7f: // DEL — 设备 erase=0x08，不识别，忽略
						// 不改 line，不回显
					case b == 0x0d: // CR 提交
						conn.Write([]byte("\r\nECHO:" + string(line) + "\r\n"))
						line = nil
					case b >= 0x20: // 可打印
						line = append(line, b)
						conn.Write([]byte{b})
					}
				}
			}
			if err != nil {
				return
			}
		}
	}()

	return ln.Addr().(*net.TCPAddr)
}

// extractEchoValues 从聚合的输出里取出所有 "ECHO:<value>" 的 value。
func extractEchoValues(s string) []string {
	var vals []string
	rest := s
	for {
		idx := strings.Index(rest, "ECHO:")
		if idx < 0 {
			break
		}
		rest = rest[idx+len("ECHO:"):]
		nl := strings.IndexAny(rest, "\r\n")
		if nl < 0 {
			vals = append(vals, rest)
			break
		}
		vals = append(vals, rest[:nl])
		rest = rest[nl:]
	}
	return vals
}

// TestIssue60_WriterTranslatesDELToBS 修复后：telnetWriter.Write 把前端
// Backspace 发出的 DEL(0x7f) 翻译为 BS(0x08)。见 conn.go telnetWriter.Write。
func TestIssue60_WriterTranslatesDELToBS(t *testing.T) {
	tc, server := startPipeConn(t)
	defer tc.Close()
	defer server.Close()

	w := &telnetWriter{c: tc}
	if _, err := w.Write([]byte{0x7f}); err != nil {
		t.Fatalf("write: %v", err)
	}

	server.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 16)
	n, err := server.Read(buf)
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	got := buf[:n]
	// 修复后：DEL(0x7f) 被翻译为 BS(0x08)
	if len(got) != 1 || got[0] != 0x08 {
		t.Fatalf("预期 DEL(0x7f)→BS(0x08)，设备收到 %v", got)
	}
	t.Logf("✓ 修复：DEL(0x7f) 已翻译为 BS(0x08) 发出")
}

// TestIssue60_BackspaceWorksOnEraseCtrlHDevice 修复后：在 erase=Ctrl-H 的
// 网络设备上，前端 Backspace(发 0x7f，经 telnetWriter 翻译为 0x08) 能正确
// 删除字符，与直接发 0x08 行为一致。
func TestIssue60_BackspaceWorksOnEraseCtrlHDevice(t *testing.T) {
	addr := startDeviceLikeServer(t)

	cfg := &remote.ConnectConfig{
		Protocol: remote.ProtocolTelnet,
		Host:     addr.IP.String(),
		Port:     addr.Port,
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	stdin, stdout, err := client.StartShell(80, 24)
	if err != nil {
		t.Fatalf("StartShell: %v", err)
	}

	// 后台聚合 stdout（goroutine 在 client.Close 后自然退出）
	var allBuf bytes.Buffer
	go func() {
		tmp := make([]byte, 1024)
		for {
			n, err := stdout.Read(tmp)
			if n > 0 {
				allBuf.Write(tmp[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	// 让协商字节先发出去
	time.Sleep(100 * time.Millisecond)

	// 场景A：用户输入 "ab" 然后按 Backspace —— xterm 发 0x7f，经翻译变 0x08
	if _, err := stdin.Write([]byte("ab\x7f\r")); err != nil {
		t.Fatalf("write A: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// 场景B：对照，输入 "ab" 然后直接发 0x08(Ctrl-H)
	if _, err := stdin.Write([]byte("ab\x08\r")); err != nil {
		t.Fatalf("write B: %v", err)
	}
	time.Sleep(400 * time.Millisecond)

	echoes := extractEchoValues(allBuf.String())
	t.Logf("设备回显的行缓冲 ECHO 值 = %v", echoes)

	var aResult, bResult string
	if len(echoes) >= 1 {
		aResult = echoes[0]
	}
	if len(echoes) >= 2 {
		bResult = echoes[1]
	}
	if aResult != "a" {
		t.Fatalf("场景A：Backspace(0x7f) 经翻译应删一字符（缓冲 \"a\"），实际 %q", aResult)
	}
	t.Logf("✓ 场景A：Backspace(0x7f) 经翻译后设备缓冲 = %q —— Issue #60 已修复", aResult)

	if bResult != "a" {
		t.Fatalf("场景B：0x08 应删一字符（缓冲 \"a\"），实际 %q", bResult)
	}
	t.Logf("✓ 场景B：直接 0x08(Ctrl-H) 设备缓冲 = %q —— 两者行为一致", bResult)
}

// TestIssue60_EnterCRNormalizedToCRLF 修复后：前端 Enter 发的单独 CR(0x0d)
// 被规范化为 CR LF（RFC 854 要求 CR 不能单独出现）；已是 CR LF 的保持原样。
func TestIssue60_EnterCRNormalizedToCRLF(t *testing.T) {
	// 单独 CR → CR LF
	tc1, server1 := startPipeConn(t)
	defer tc1.Close()
	defer server1.Close()
	w1 := &telnetWriter{c: tc1}
	if _, err := w1.Write([]byte("ls\r")); err != nil {
		t.Fatalf("write1: %v", err)
	}
	server1.SetReadDeadline(time.Now().Add(time.Second))
	buf1 := make([]byte, 32)
	n1, err := server1.Read(buf1)
	if err != nil {
		t.Fatalf("server read1: %v", err)
	}
	if got, want := buf1[:n1], []byte("ls\r\n"); !bytes.Equal(got, want) {
		t.Fatalf("单独 CR 规范化：预期 %v，实际 %v", want, got)
	}
	t.Logf("✓ 修复：单独 CR(Enter) 规范化为 CR LF")

	// 已规范的 CR LF 保持原样（不重复加 LF）
	tc2, server2 := startPipeConn(t)
	defer tc2.Close()
	defer server2.Close()
	w2 := &telnetWriter{c: tc2}
	if _, err := w2.Write([]byte("ls\r\n")); err != nil {
		t.Fatalf("write2: %v", err)
	}
	server2.SetReadDeadline(time.Now().Add(time.Second))
	buf2 := make([]byte, 32)
	n2, err := server2.Read(buf2)
	if err != nil {
		t.Fatalf("server read2: %v", err)
	}
	if got, want := buf2[:n2], []byte("ls\r\n"); !bytes.Equal(got, want) {
		t.Fatalf("已规范 CRLF 应保持：预期 %v，实际 %v", want, got)
	}
	t.Logf("✓ 已规范的 CR LF 保持原样，未重复加 LF")
}

// TestIssue60_AutologinSubmitsWithCRLF 修复后：loginHandler 提交用户名/密码
// 用 CR LF（与 client.go Run() 的 \r\n 一致），而非裸 LF。
func TestIssue60_AutologinSubmitsWithCRLF(t *testing.T) {
	var buf bytes.Buffer
	h := newLoginHandler(&buf, "alice", "secret")

	h.Handle([]byte("router-01 login: "))
	userWritten := buf.String()
	t.Logf("收到 login 提示后写出 = %q", userWritten)
	if !strings.HasSuffix(userWritten, "\r\n") {
		t.Fatalf("预期用户名以 CRLF 提交，实际 %q", userWritten)
	}
	t.Logf("✓ 修复：用户名以 CRLF 提交（autologin.go）")

	h.Handle([]byte("\r\nPassword: "))
	pwWritten := buf.String()[len(userWritten):]
	t.Logf("收到 password 提示后写出 = %q", pwWritten)
	if !strings.HasSuffix(pwWritten, "\r\n") {
		t.Fatalf("预期密码以 CRLF 提交，实际 %q", pwWritten)
	}
	t.Logf("✓ 修复：密码以 CRLF 提交（autologin.go writePassword）")
	t.Logf("对比：client.go:259 Run() 命令也用 \\r\\n —— 包内行结束约定现已一致")
}

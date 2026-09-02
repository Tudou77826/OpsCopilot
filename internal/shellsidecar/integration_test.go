package shellsidecar

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"opscopilot/internal/shellsidecar/fakessh"
	"opscopilot/pkg/remote"
)

// 集成测试：不走 fake Connection，而是 sidecar → 真实 pkg/sshclient →
// 进程内 fake SSH 服务器。整条链（SSH 握手/认证/pty-req/shell/数据/resize/
// 断开）都走真实协议，仅对端是测试替身。

func startFakeSSHWithSFTP(t *testing.T, root string) *fakessh.Server {
	t.Helper()
	server, err := fakessh.Start("", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func startFakeSSH(t *testing.T) *fakessh.Server {
	t.Helper()
	server, err := fakessh.Start("== fakessh ready ==\r\n", os.Getenv("FAKESSH_SFTP_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func TestIntegrationRealSSHFullChain(t *testing.T) {
	server := startFakeSSH(t)
	svc := NewTerminalService("test")
	svc.dialer = func(cfg *remote.ConnectConfig) (remote.Connection, error) {
		// 走真实注册的 SSH Dialer（pkg/sshclient）
		return remote.Dial(cfg)
	}
	var mu sync.Mutex
	notes := &[]notification{}
	svc.SetNotify(func(method string, params any) {
		data, _ := json.Marshal(params)
		mu.Lock()
		*notes = append(*notes, notification{Method: method, Params: data})
		mu.Unlock()
	})
	t.Cleanup(svc.Shutdown)

	connID, err := svc.Connect(remote.ConnectConfig{
		Host: "127.0.0.1", Port: server.Port(), User: "test", Password: "test",
		Protocol: remote.ProtocolSSH,
	})
	if err != nil {
		t.Fatalf("真实 SSH 连接失败: %v", err)
	}
	termID, err := svc.OpenTerminal(connID, 100, 30)
	if err != nil {
		t.Fatalf("真实 PTY 打开失败: %v", err)
	}
	att, err := svc.Attach(termID)
	if err != nil {
		t.Fatal(err)
	}
	defer att.Detach()

	// 横幅（服务器主动下发）应出现在订阅流
	bannerSeen := false
	select {
	case data := <-att.Ch:
		if strings.Contains(string(data), "fakessh ready") {
			bannerSeen = true
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting banner")
	}
	// 若首个包不含横幅（拆包），继续读有限次
	for i := 0; i < 5 && !bannerSeen; i++ {
		select {
		case data := <-att.Ch:
			if strings.Contains(string(data), "fakessh ready") {
				bannerSeen = true
			}
		case <-time.After(1 * time.Second):
		}
	}
	if !bannerSeen {
		t.Fatal("banner not received")
	}

	// 键入 → 真实 SSH 传输 → 服务器回显 → 订阅流
	if err := svc.WriteInput(termID, []byte("echo marker42\r")); err != nil {
		t.Fatal(err)
	}
	echoSeen := false
	for i := 0; i < 20 && !echoSeen; i++ {
		select {
		case data := <-att.Ch:
			if strings.Contains(string(data), "echo marker42") {
				echoSeen = true
			}
		case <-time.After(1 * time.Second):
		}
	}
	if !echoSeen {
		t.Fatal("echo roundtrip not received")
	}

	// resize → 真实 window-change 请求，无错误即通过
	if err := svc.Resize(termID, 132, 43); err != nil {
		t.Fatalf("resize over real ssh failed: %v", err)
	}

	// 断开连接 → 终端退出通知
	if err := svc.Disconnect(connID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := append([]notification(nil), *notes...)
		mu.Unlock()
		exited := false
		for _, n := range got {
			if n.Method == "terminal/exited" && strings.Contains(string(n.Params), termID) {
				exited = true
			}
		}
		if exited {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("terminal/exited notification missing after disconnect")
}

// S6：真实 SFTP 链路——fakessh 沙箱子系统 + sshclient.SFTPClient 往返。
func TestIntegrationSFTPRoundtrip(t *testing.T) {
	root := t.TempDir()
	server := startFakeSSHWithSFTP(t, root)
	svc := NewTerminalService("test")
	svc.dialer = func(cfg *remote.ConnectConfig) (remote.Connection, error) { return remote.Dial(cfg) }
	t.Cleanup(svc.Shutdown)

	connID, err := svc.Connect(remote.ConnectConfig{
		Host: "127.0.0.1", Port: server.Port(), User: "test", Password: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := svc.Connection(connID)
	if err != nil {
		t.Fatal(err)
	}
	capable, ok := conn.(remote.SFTPCapable)
	if !ok {
		t.Fatal("sshclient should be SFTPCapable")
	}
	client, err := capable.SFTPClient()
	if err != nil {
		t.Fatalf("SFTP 子系统握手失败: %v", err)
	}
	defer client.Close()

	// 上传（模拟 UploadEnd 的成传语义）
	remoteFile, err := client.Create("/uploaded.txt")
	if err != nil {
		t.Fatalf("SFTP create 失败: %v", err)
	}
	if _, err := remoteFile.Write([]byte("sftp-roundtrip-OK")); err != nil {
		t.Fatal(err)
	}
	_ = remoteFile.Close()

	// 下载读回
	readBack, err := client.Open("/uploaded.txt")
	if err != nil {
		t.Fatalf("SFTP open 失败: %v", err)
	}
	defer readBack.Close()
	content, err := io.ReadAll(readBack)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "sftp-roundtrip-OK" {
		t.Fatalf("content = %q", content)
	}
	// 沙箱越界必须失败
	if _, err := client.Open("/../../etc/hosts"); err == nil {
		t.Fatal("sandbox escape must fail")
	}
}

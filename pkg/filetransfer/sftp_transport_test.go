package filetransfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSFTP_ListUploadDownload_HappyPath(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, EnableSFTP: true})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	tr := NewSFTPTransport(client)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	localSrc := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(localSrc, []byte("hello"), 0644); err != nil {
		t.Fatalf("write local: %v", err)
	}

	if _, err := tr.Upload(ctx, localSrc, "a.txt", nil); err != nil {
		t.Fatalf("upload: %v", err)
	}

	entries, err := tr.List(ctx, ".")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "a.txt" && !e.IsDir && e.Size == 5 {
			found = true
		}
	}
	if !found {
		t.Fatalf("uploaded file not found in list")
	}

	downloadDest := filepath.Join(t.TempDir(), "b.txt")
	if _, err := tr.Download(ctx, "a.txt", downloadDest, nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	b, err := os.ReadFile(downloadDest)
	if err != nil {
		t.Fatalf("read downloaded: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("downloaded content = %q, want %q", string(b), "hello")
	}
}

func TestSFTP_RemoteOps_MkdirRenameRemove_ReadWrite(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, EnableSFTP: true})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	tr := NewSFTPTransport(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tr.Mkdir(ctx, "dir1"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	entries, err := tr.List(ctx, ".")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	foundDir := false
	for _, e := range entries {
		if e.Name == "dir1" && e.IsDir {
			foundDir = true
		}
	}
	if !foundDir {
		t.Fatalf("dir1 not found after mkdir")
	}

	if err := tr.WriteFile(ctx, "dir1/a.txt", []byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, err := tr.ReadFile(ctx, "dir1/a.txt", 1024)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "x" {
		t.Fatalf("content = %q, want %q", string(b), "x")
	}

	if err := tr.Rename(ctx, "dir1/a.txt", "dir1/b.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	_, err = tr.ReadFile(ctx, "dir1/b.txt", 1024)
	if err != nil {
		t.Fatalf("read after rename: %v", err)
	}

	if err := tr.Remove(ctx, "dir1", true); err != nil {
		t.Fatalf("remove recursive: %v", err)
	}
	_, err = tr.Stat(ctx, "dir1")
	if err == nil {
		t.Fatalf("expected stat error after remove")
	}
}

func TestSFTP_NotSupported_SubsystemDisabled(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, EnableSFTP: false})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	tr := NewSFTPTransport(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err = tr.Check(ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	te, ok := err.(*TransferError)
	if !ok {
		t.Fatalf("expected TransferError, got %T: %v", err, err)
	}
	if te.Code != ErrorCodeSFTPNotSupported {
		t.Fatalf("code = %s, want %s", te.Code, ErrorCodeSFTPNotSupported)
	}
}

// --- Issue #64 防护用例：不支持 SFTP 的设备上传输不得卡死 ---
//
// 真实故障链：网络设备/嵌入式 sshd 接受 sftp subsystem 请求但从不响应 SFTP
// 协议字节，无超时的 sftp.NewClient 永久阻塞 → 文件面板列表挂死、传输任务
// 占住并发槽位、取消失效。以下用例以 SilentSFTP 假服务器复现该设备行为。

// TestDialSFTP_SilentPeer_Bounded 覆盖业务约束："SFTP 握手必须在有限时间
// 内以明确错误结束，而不是永久挂起"。
func TestDialSFTP_SilentPeer_Bounded(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, SilentSFTP: true})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	start := time.Now()
	_, err = DialSFTP(context.Background(), client, 300*time.Millisecond)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected timeout error on silent sftp peer, got nil")
	}
	var te *TransferError
	if !errors.As(err, &te) {
		t.Fatalf("expected TransferError, got %T: %v", err, err)
	}
	if te.Code != ErrorCodeSFTPNotSupported {
		t.Errorf("code = %s, want %s (触发 SCP 降级路径)", te.Code, ErrorCodeSFTPNotSupported)
	}
	if !strings.Contains(te.Message, "握手超时") {
		t.Errorf("message should mention handshake timeout, got %q", te.Message)
	}
	// 300ms 超时 + 清理开销，给到 2s 上限已经非常宽裕；超过即视为仍会挂起。
	if elapsed > 2*time.Second {
		t.Errorf("dial returned after %v, handshake is not bounded", elapsed)
	}
}

// TestDialSFTP_CancelDuringHandshake 覆盖业务约束："取消按钮必须能打断
// 进行中的 SFTP 握手"，而不是像修复前那样取消无效、任务永久占用并发槽位。
func TestDialSFTP_CancelDuringHandshake(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, SilentSFTP: true})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	// 默认握手超时 10s 远大于取消点，验证是 ctx 而非超时在起作用。
	_, err = DialSFTP(ctx, client, 0)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected cancellation error, got nil")
	}
	var te *TransferError
	if !errors.As(err, &te) || te.Code != ErrorCodeUnknown || !strings.Contains(te.Message, "取消") {
		t.Fatalf("expected cancellation TransferError, got %#v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("cancel returned after %v, cancellation is not effective", elapsed)
	}
}

// TestSFTP_List_SilentPeer_Bounded 覆盖用户操作流："在不支持 SFTP 的机器上
// 打开文件面板（目录列表），必须在有限时间内得到错误并显示，而不是无限加载"。
func TestSFTP_List_SilentPeer_Bounded(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, SilentSFTP: true})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	orig := sftpHandshakeTimeout
	sftpHandshakeTimeout = 300 * time.Millisecond
	defer func() { sftpHandshakeTimeout = orig }()

	tr := NewSFTPTransport(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	_, err = tr.List(ctx, ".")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected error on silent sftp peer, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("list returned after %v, still hangs on silent peer", elapsed)
	}
	var te *TransferError
	if !errors.As(err, &te) || te.Code != ErrorCodeSFTPNotSupported {
		t.Fatalf("expected SFTP_NOT_SUPPORTED timeout error, got %#v", err)
	}
}

// TestDialSFTP_HappyPath_SessionClosedWithClient 保证有界握手建立的客户端
// 在 Close 时连带关闭底层 SSH session：SFTP 正常路径不受改动影响。
func TestDialSFTP_HappyPath_SessionClosedWithClient(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, EnableSFTP: true})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	c, err := DialSFTP(context.Background(), client, 0)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := c.Stat("."); err != nil {
		t.Fatalf("stat after dial: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

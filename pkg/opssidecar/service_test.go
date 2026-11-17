package opssidecar

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"opscopilot/internal/shellsidecar/fakessh"
	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/remote"

	// 注册协议拨号器（与 cmd/ops-sidecar 的空白 import 语义一致）
	_ "opscopilot/pkg/sshclient"
	_ "opscopilot/pkg/telnetclient"
)

// syncBuffer 并发安全缓冲：事件由终端 pump / 传输 goroutine 写入，测试主
// goroutine 轮询读取。
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// fixture fakessh + 数据目录 + Service 的端到端环境。
type fixture struct {
	svc      *Service
	out      *syncBuffer
	server   *fakessh.Server
	serverID string
	sftpRoot string
	dataDir  string
}

const fakesshBanner = "WELCOME-FAKESSH-BANNER"

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	sftpRoot := filepath.Join(root, "sftp")
	dataDir := filepath.Join(root, "data")
	localDir := filepath.Join(root, "local")
	for _, dir := range []string{sftpRoot, dataDir, localDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}

	server, err := fakessh.StartAddr(fakesshBanner, sftpRoot, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动 fakessh 失败: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	// 连接树：先用独立 Store 写盘，Service 自行加载同一份文件
	store := connectionstore.NewStoreWithPath(filepath.Join(dataDir, "sessions.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("初始化连接树失败: %v", err)
	}
	node, err := store.CreateConnection(remote.ConnectConfig{
		Name:     "test-server",
		Protocol: "ssh",
		Host:     "127.0.0.1",
		Port:     server.Port(),
		User:     "test",
		Password: "test",
	}, "")
	if err != nil {
		t.Fatalf("创建连接失败: %v", err)
	}

	// 文件访问闸门：放行本机 IP 的 /data /logs 读写与本地目录
	fileAccess := map[string]any{
		"version": "1",
		"policies": []map[string]any{{
			"id":                 "test-all",
			"name":               "test-all",
			"ip_ranges":          []string{"127.0.0.1/32"},
			"read_paths":         []string{"/data", "/logs"},
			"write_paths":        []string{"/data"},
			"allowed_local_dirs": []string{localDir},
			"max_read_bytes":     268435456,
			"max_write_bytes":    268435456,
		}},
	}
	raw, err := json.Marshal(fileAccess)
	if err != nil {
		t.Fatalf("编码 file_access 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "file_access.json"), raw, 0o644); err != nil {
		t.Fatalf("写入 file_access 失败: %v", err)
	}

	out := &syncBuffer{}
	svc, err := NewService(dataDir, out)
	if err != nil {
		t.Fatalf("构造 Service 失败: %v", err)
	}
	t.Cleanup(svc.Close)
	return &fixture{svc: svc, out: out, server: server, serverID: node.ID, sftpRoot: sftpRoot, dataDir: dataDir}
}

// call 同步调用一个方法并返回解析后的响应 data。
func (f *fixture) call(t *testing.T, method string, params any) (map[string]any, *ErrorWire) {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("编码参数失败: %v", err)
	}
	resp, _ := f.svc.handle(Request{ID: "t-" + method, Method: method, Params: raw})
	if !resp.OK {
		return nil, resp.Error
	}
	data, _ := resp.Data.(map[string]any)
	return data, nil
}

// events 解析当前缓冲中的全部事件行。
func (f *fixture) events() []map[string]any {
	var result []map[string]any
	for _, line := range strings.Split(f.out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var payload map[string]any
		if json.Unmarshal([]byte(line), &payload) != nil {
			continue
		}
		if kind, _ := payload["event"].(string); kind != "" {
			result = append(result, payload)
		}
	}
	return result
}

// eventParams 取事件参数体。
func eventParams(ev map[string]any) map[string]any {
	params, _ := ev["params"].(map[string]any)
	return params
}

// waitForEvent 轮询等待首个满足条件的事件，超时 fail。
func waitForEvent(t *testing.T, f *fixture, timeout time.Duration, match func(ev map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, ev := range f.events() {
			if match(ev) {
				return ev
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待事件超时（%v）", timeout)
	return nil
}

func TestTerminalOpenWriteResizeReattachClose(t *testing.T) {
	f := newFixture(t)

	data, rpcErr := f.call(t, MethodTerminalOpen, map[string]any{
		"serverId": f.serverID, "cols": 80, "rows": 24,
	})
	if rpcErr != nil {
		t.Fatalf("打开终端失败: %+v", rpcErr)
	}
	sessionID, _ := data["sessionId"].(string)
	if sessionID == "" {
		t.Fatal("响应缺少 sessionId")
	}

	// 闸门未就绪前，banner 只暂存不下发
	if hasEvent(f, EventTerminalOutput) {
		t.Fatal("ready 前不应有输出事件")
	}
	if _, rpcErr := f.call(t, MethodTerminalReady, map[string]any{"sessionId": sessionID}); rpcErr != nil {
		t.Fatalf("ready 失败: %+v", rpcErr)
	}
	if ev := waitForEvent(t, f, 2*time.Second, func(ev map[string]any) bool {
		params := eventParams(ev)
		id, _ := params["sessionId"].(string)
		return ev["event"] == EventTerminalOutput && id == sessionID &&
			strings.Contains(decodeEventData(ev), fakesshBanner)
	}); ev == nil {
		t.Fatal("ready 后未收到 banner")
	}

	// 按键回显
	f.out.reset()
	if _, rpcErr := f.call(t, MethodTerminalWrite, map[string]any{"sessionId": sessionID, "data": "echo-me\r\n"}); rpcErr != nil {
		t.Fatalf("write 失败: %+v", rpcErr)
	}
	waitForEvent(t, f, 2*time.Second, func(ev map[string]any) bool {
		return ev["event"] == EventTerminalOutput && strings.Contains(decodeEventData(ev), "echo-me")
	})

	// resize（fakessh 处理 window-change）
	if _, rpcErr := f.call(t, MethodTerminalResize, map[string]any{"sessionId": sessionID, "cols": 120, "rows": 40}); rpcErr != nil {
		t.Fatalf("resize 失败: %+v", rpcErr)
	}

	// 重挂：同 sessionId 重放现场
	f.out.reset()
	reattach, rpcErr := f.call(t, MethodTerminalOpen, map[string]any{
		"serverId": f.serverID, "sessionId": sessionID, "cols": 120, "rows": 40,
	})
	if rpcErr != nil {
		t.Fatalf("重挂失败: %+v", rpcErr)
	}
	replay, err := base64.StdEncoding.DecodeString(reattach["replay"].(string))
	if err != nil {
		t.Fatalf("replay 不是合法 base64: %v", err)
	}
	if !strings.Contains(string(replay), fakesshBanner) || !strings.Contains(string(replay), "echo-me") {
		t.Fatalf("重放现场不完整: %q", string(replay))
	}
	// 重挂后新闸门待 ready，旧流输出不再直通
	if hasEvent(f, EventTerminalOutput) {
		t.Fatal("重挂后 ready 前不应有输出事件")
	}

	// 关闭后写入应报会话不存在
	if _, rpcErr := f.call(t, MethodTerminalClose, map[string]any{"sessionId": sessionID}); rpcErr != nil {
		t.Fatalf("close 失败: %+v", rpcErr)
	}
	_, rpcErr = f.call(t, MethodTerminalWrite, map[string]any{"sessionId": sessionID, "data": "x"})
	if rpcErr == nil || rpcErr.Code != CodeNotFound {
		t.Fatalf("关闭后 write 应报 not_found，实际 %+v", rpcErr)
	}
}

func TestTerminalSecondSessionRejected(t *testing.T) {
	f := newFixture(t)
	data, rpcErr := f.call(t, MethodTerminalOpen, map[string]any{"serverId": f.serverID})
	if rpcErr != nil {
		t.Fatalf("打开终端失败: %+v", rpcErr)
	}
	_, rpcErr = f.call(t, MethodTerminalOpen, map[string]any{"serverId": f.serverID})
	if rpcErr == nil || rpcErr.Code != CodeTerminalBusy {
		t.Fatalf("同服务器第二个终端应报 terminal_busy，实际 %+v", rpcErr)
	}
	_ = data
}

func TestTransferUploadDownloadWithProgress(t *testing.T) {
	f := newFixture(t)
	localDir := filepath.Join(f.dataDir, "..", "local")
	payload := bytes.Repeat([]byte("aaw-transfer-payload-"), 100*1024) // 约 2MB
	src := filepath.Join(localDir, "upload-src.bin")
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatalf("准备源文件失败: %v", err)
	}

	data, rpcErr := f.call(t, MethodTransferUpload, map[string]any{
		"serverId": f.serverID, "localPath": src, "remotePath": "/data/uploaded.bin",
		"backup": true, "mkdir": true,
	})
	if rpcErr != nil {
		t.Fatalf("启动上传失败: %+v", rpcErr)
	}
	transferID, _ := data["transferId"].(string)
	if transferID == "" {
		t.Fatal("响应缺少 transferId")
	}

	done := waitForEvent(t, f, 10*time.Second, func(ev map[string]any) bool {
		params := eventParams(ev)
		id, _ := params["transferId"].(string)
		return ev["event"] == EventTransferDone && id == transferID
	})
	doneParams := eventParams(done)
	if doneParams["ok"] != true {
		t.Fatalf("上传未成功: %+v", doneParams)
	}
	remoteFile := filepath.Join(f.sftpRoot, "data", "uploaded.bin")
	got, err := os.ReadFile(remoteFile)
	if err != nil {
		t.Fatalf("远端文件不存在: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("上传内容不一致: got %d bytes, want %d", len(got), len(payload))
	}

	// 下载回本地并比对
	dst := filepath.Join(localDir, "downloaded.bin")
	f.out.reset()
	data, rpcErr = f.call(t, MethodTransferDownload, map[string]any{
		"serverId": f.serverID, "localPath": dst, "remotePath": "/data/uploaded.bin",
	})
	if rpcErr != nil {
		t.Fatalf("启动下载失败: %+v", rpcErr)
	}
	downloadID, _ := data["transferId"].(string)
	waitForEvent(t, f, 10*time.Second, func(ev map[string]any) bool {
		params := eventParams(ev)
		id, _ := params["transferId"].(string)
		return ev["event"] == EventTransferDone && id == downloadID
	})
	got, err = os.ReadFile(dst)
	if err != nil {
		t.Fatalf("下载文件不存在: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("下载内容不一致")
	}
}

func TestTransferConcurrencyAllComplete(t *testing.T) {
	f := newFixture(t)
	localDir := filepath.Join(f.dataDir, "..", "local")
	payload := bytes.Repeat([]byte("z"), 512*1024)

	var ids []string
	for i := 0; i < 3; i++ { // 每服务器限流 2：第 3 个必须排队
		src := filepath.Join(localDir, fmt.Sprintf("src-%d.bin", i))
		if err := os.WriteFile(src, payload, 0o644); err != nil {
			t.Fatalf("准备源文件失败: %v", err)
		}
		data, rpcErr := f.call(t, MethodTransferUpload, map[string]any{
			"serverId": f.serverID, "localPath": src, "remotePath": fmt.Sprintf("/data/c-%d.bin", i),
			"mkdir": true,
		})
		if rpcErr != nil {
			t.Fatalf("启动上传 %d 失败: %+v", i, rpcErr)
		}
		ids = append(ids, data["transferId"].(string))
	}

	deadline := time.Now().Add(20 * time.Second)
	for len(ids) > 0 && time.Now().Before(deadline) {
		for _, ev := range f.events() {
			if ev["event"] != EventTransferDone {
				continue
			}
			params := eventParams(ev)
			id, _ := params["transferId"].(string)
			if params["ok"] != true {
				t.Fatalf("任务 %s 未成功: %+v", id, params)
			}
			if idx := slices.Index(ids, id); idx >= 0 {
				ids = slices.Delete(ids, idx, idx+1)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(ids) > 0 {
		t.Fatalf("任务未全部完成: %v", ids)
	}
}

func TestTransferCancelUnknown(t *testing.T) {
	f := newFixture(t)
	_, rpcErr := f.call(t, MethodTransferCancel, map[string]any{"transferId": "missing"})
	if rpcErr == nil || rpcErr.Code != CodeNotFound {
		t.Fatalf("取消未知任务应报 not_found，实际 %+v", rpcErr)
	}
}

// hasEvent 快速判断当前缓冲是否已有某类事件。
func hasEvent(f *fixture, event string) bool {
	for _, ev := range f.events() {
		if ev["event"] == event {
			return true
		}
	}
	return false
}

// decodeEventData 解出 terminal.output 事件的 data 字段（base64）。
func decodeEventData(ev map[string]any) string {
	data, _ := eventParams(ev)["data"].(string)
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return ""
	}
	return string(raw)
}

package opssidecar

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"opscopilot/internal/shellsidecar/fakessh"
	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/remote"

	_ "opscopilot/pkg/sshclient"
	_ "opscopilot/pkg/telnetclient"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *syncBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buf.String() }
func (b *syncBuffer) reset()         { b.mu.Lock(); defer b.mu.Unlock(); b.buf.Reset() }

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

	store := connectionstore.NewStoreWithPath(filepath.Join(dataDir, "sessions.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("初始化连接树失败: %v", err)
	}
	node, err := store.CreateConnection(remote.ConnectConfig{
		Name: "test-server", Host: "127.0.0.1", Port: server.Port(), User: "test", Password: "test",
	}, "")
	if err != nil {
		t.Fatalf("创建连接失败: %v", err)
	}
	raw, _ := json.Marshal(map[string]any{
		"version": "1",
		"policies": []map[string]any{{
			"id": "test", "name": "test", "ip_ranges": []string{"127.0.0.1/32"},
			"read_paths": []string{"/data", "/logs"}, "write_paths": []string{"/data"},
			"allowed_local_dirs": []string{localDir},
			"max_read_bytes":     268435456, "max_write_bytes": 268435456,
		}},
	})
	if err := os.WriteFile(filepath.Join(dataDir, "file_access.json"), raw, 0o644); err != nil {
		t.Fatalf("写入 file_access 失败: %v", err)
	}
	out := &syncBuffer{}
	svc, err := NewService(dataDir, out)
	if err != nil {
		t.Fatalf("构造 Service 失败: %v", err)
	}
	t.Cleanup(svc.Close)
	return &fixture{svc: svc, out: out, serverID: node.ID, sftpRoot: sftpRoot, dataDir: dataDir, localDir: localDir}
}

type fixture struct {
	svc      *Service
	out      *syncBuffer
	serverID string
	sftpRoot string
	dataDir  string
	localDir string
}

func (f *fixture) call(t *testing.T, method string, params any) (any, *ErrorWire) {
	t.Helper()
	raw, _ := json.Marshal(params)
	resp, _ := f.svc.handle(Request{ID: "t", Method: method, Params: raw})
	if !resp.OK {
		return nil, resp.Error
	}
	return resp.Data, nil
}

func (f *fixture) events() []map[string]any {
	var result []map[string]any
	for _, line := range strings.Split(f.out.String(), "\n") {
		var payload map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &payload) == nil {
			if kind, _ := payload["event"].(string); kind != "" {
				result = append(result, payload)
			}
		}
	}
	return result
}

func waitFor(t *testing.T, f *fixture, timeout time.Duration, match func(map[string]any) bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, ev := range f.events() {
			if match(ev) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待事件超时，已有事件: %d 条", len(f.events()))
}

func TestTreeCrudAndReorder(t *testing.T) {
	f := newFixture(t)
	folderData, err := f.call(t, MethodCreateFolder, map[string]any{"name": "分组"})
	if err != nil {
		t.Fatalf("建目录失败: %+v", err)
	}
	folder := folderData.(*connectionstore.Node)
	if _, err := f.call(t, MethodCreateConnection, map[string]any{
		"config":   map[string]any{"name": "srv2", "host": "10.0.0.9", "port": 22, "user": "root"},
		"parentId": folder.ID,
	}); err != nil {
		t.Fatalf("建连接失败: %+v", err)
	}
	listData, err := f.call(t, MethodList, map[string]any{})
	if err != nil {
		t.Fatalf("列树失败: %+v", err)
	}
	nodes := listData.(map[string]any)["nodes"].([]*connectionstore.Node)
	if len(nodes) < 2 {
		t.Fatalf("树顶层节点应至少 2 个，实际 %d", len(nodes))
	}
	ordered := []string{}
	for _, node := range nodes {
		ordered = append(ordered, node.ID)
	}
	if _, err := f.call(t, MethodReorderChildren, map[string]any{"parentId": "", "orderedIds": ordered}); err != nil {
		t.Fatalf("排序失败: %+v", err)
	}
}

func TestExecWhitelistDeny(t *testing.T) {
	f := newFixture(t)
	if _, err := f.call(t, MethodExec, map[string]any{"serverId": f.serverID, "command": "definitely-not-allowed"}); err == nil || err.Code != CodeWhitelistDeny {
		t.Fatalf("非白名单命令应拒绝: %+v", err)
	}
}

func TestMultipleTerminalsPerServer(t *testing.T) {
	f := newFixture(t)
	firstData, err := f.call(t, MethodTerminalOpen, map[string]any{"serverId": f.serverID})
	if err != nil {
		t.Fatalf("第一个终端失败: %+v", err)
	}
	secondData, err := f.call(t, MethodTerminalOpen, map[string]any{"serverId": f.serverID})
	if err != nil {
		t.Fatalf("同服务器第二个终端应成功（独立拨号）: %+v", err)
	}
	first := firstData.(map[string]any)
	second := secondData.(map[string]any)
	if first["sessionId"] == second["sessionId"] {
		t.Fatal("两个终端 sessionId 不应相同")
	}
	f.out.reset()
	if _, err := f.call(t, MethodTerminalReady, map[string]any{"sessionId": second["sessionId"]}); err != nil {
		t.Fatalf("ready 失败: %+v", err)
	}
	waitFor(t, f, 2*time.Second, func(ev map[string]any) bool {
		return strings.HasPrefix(ev["event"].(string), EventTerminalDataPrefix)
	})
}

func TestFTUploadDownloadEnvelopeAndEvents(t *testing.T) {
	f := newFixture(t)
	payload := bytes.Repeat([]byte("ft-payload-"), 64*1024)
	src := filepath.Join(f.localDir, "src.bin")
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatalf("准备源文件失败: %v", err)
	}
	// 远端目录列表（信封）
	ftListRaw, err := f.call(t, MethodFTList, map[string]any{"sessionId": f.serverID, "path": "/"})
	if err != nil {
		t.Fatalf("FTList 失败: %+v", err)
	}
	var envelope ftResponse
	if json.Unmarshal([]byte(ftListRaw.(string)), &envelope) != nil || !envelope.OK {
		t.Fatalf("FTList 信封异常: %v", ftListRaw)
	}
	// 上传（异步任务 + 事件）
	if _, err := f.call(t, MethodFTRemoteMkdir, map[string]any{"sessionId": f.serverID, "path": "/data"}); err != nil {
		t.Fatalf("FTRemoteMkdir 失败: %+v", err)
	}
	startRaw, err := f.call(t, MethodFTUpload, map[string]any{"sessionId": f.serverID, "localPath": src, "remotePath": "/data/up.bin"})
	if err != nil {
		t.Fatalf("FTUpload 失败: %+v", err)
	}
	var startEnvelope ftResponse
	_ = json.Unmarshal([]byte(startRaw.(string)), &startEnvelope)
	if !startEnvelope.OK || startEnvelope.TaskID == "" {
		t.Fatalf("FTUpload 应返回 taskId 信封: %v", startRaw)
	}
	waitFor(t, f, 10*time.Second, func(ev map[string]any) bool {
		if ev["event"] != EventTransferDone {
			return false
		}
		params, _ := ev["params"].(map[string]any)
		return params["taskId"] == startEnvelope.TaskID && params["ok"] == true
	})
	got, readErr := os.ReadFile(filepath.Join(f.sftpRoot, "data", "up.bin"))
	if readErr != nil || !bytes.Equal(got, payload) {
		t.Fatalf("远端内容不一致: %v", readErr)
	}
	// 本地列目录
	localRaw, _ := f.call(t, MethodLocalList, map[string]any{"path": f.localDir})
	var localEnvelope ftResponse
	_ = json.Unmarshal([]byte(localRaw.(string)), &localEnvelope)
	if !localEnvelope.OK || len(localEnvelope.Entries) == 0 {
		t.Fatalf("LocalList 信封异常: %v", localRaw)
	}
}

func TestConcurrentHandoffApprovals(t *testing.T) {
	f := newFixture(t)
	openData, err := f.call(t, MethodTerminalOpen, map[string]any{"serverId": f.serverID})
	if err != nil {
		t.Fatalf("打开终端失败: %+v", err)
	}
	sid := openData.(map[string]any)["sessionId"].(string)
	// 两条待审批并发挂起，互不拒绝
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := f.svc.terminalWriteForAgent("hint-a", sid, "echo x\r")
			results <- err
		}()
	}
	deadline := time.Now().Add(2 * time.Second)
	var requestIds []string
	for len(requestIds) < 2 && time.Now().Before(deadline) {
		for _, ev := range f.events() {
			if ev["event"] == EventTerminalHandoff {
				if id, _ := ev["params"].(map[string]any)["requestId"].(string); id != "" {
					exists := false
					for _, existing := range requestIds {
						if existing == id {
							exists = true
						}
					}
					if !exists {
						requestIds = append(requestIds, id)
					}
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(requestIds) != 2 {
		t.Fatalf("应有 2 条并发待审批，实际 %d", len(requestIds))
	}
	for i, id := range requestIds {
		if _, err := f.call(t, MethodTerminalApproveHandoff, map[string]any{"requestId": id, "approved": i == 0}); err != nil {
			t.Fatalf("裁决失败: %+v", err)
		}
	}
	rejected := 0
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				rejected++
			}
		case <-time.After(3 * time.Second):
			t.Fatal("裁决后写入未返回")
		}
	}
	if rejected != 1 {
		t.Fatalf("应一条成功一条被拒，拒绝数 = %d", rejected)
	}
}

func TestSplitSessionToken(t *testing.T) {
	if hint, ok := splitSessionToken("abc.base", "base"); !ok || hint != "abc" {
		t.Fatalf("组合 token 解析失败: %q %v", hint, ok)
	}
	if _, ok := splitSessionToken("base", "base"); !ok {
		t.Fatal("裸 token 应匹配")
	}
	if _, ok := splitSessionToken("abc.other", "base"); ok {
		t.Fatal("错误后缀不应匹配")
	}
}

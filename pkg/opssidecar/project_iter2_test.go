package opssidecar

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// fakeSSHExecServer 同时支持 SFTP 和可控的 SSH exec（用于并发命令、取消、Git 查询与 MCP 闭环测试）。
type fakeSSHExecServer struct {
	ln       net.Listener
	hostPort int
	signer   ssh.Signer
	rootDir  string
	execFn   func(cmd string, stdout, stderr io.Writer, cancel <-chan struct{}) int
}

func startFakeSSHExecServer(
	t *testing.T,
	rootDir string,
	execFn func(cmd string, stdout, stderr io.Writer, cancel <-chan struct{}) int,
) *fakeSSHExecServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &fakeSSHExecServer{
		ln:       ln,
		hostPort: ln.Addr().(*net.TCPAddr).Port,
		signer:   newSigner(t),
		rootDir:  rootDir,
		execFn:   execFn,
	}
	go srv.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return srv
}

func (f *fakeSSHExecServer) authorizedHostKey() string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(f.signer.PublicKey())))
}

func (f *fakeSSHExecServer) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handleConn(conn)
	}
}

func (f *fakeSSHExecServer) handleConn(raw net.Conn) {
	defer raw.Close()
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if meta.User() == "dev" && string(pass) == "secret" {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("bad auth")
		},
	}
	cfg.AddHostKey(f.signer)

	sConn, chans, reqs, err := ssh.NewServerConn(raw, cfg)
	if err != nil {
		return
	}
	defer sConn.Close()
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		ch, requests, err := newChan.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer ch.Close()
			cancelCh := make(chan struct{})
			var cancelOnce sync.Once
			cancel := func() { cancelOnce.Do(func() { close(cancelCh) }) }
			for req := range requests {
				switch req.Type {
				case "subsystem":
					if len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp" {
						_ = req.Reply(true, nil)
						server, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(f.rootDir))
						if err == nil {
							_ = server.Serve()
							_ = server.Close()
						}
						return
					}
					_ = req.Reply(false, nil)
				case "exec":
					if len(req.Payload) < 4 {
						_ = req.Reply(false, nil)
						continue
					}
					cmdLen := binary.BigEndian.Uint32(req.Payload[:4])
					cmd := string(req.Payload[4 : 4+cmdLen])
					_ = req.Reply(true, nil)
					go func(command string) {
						defer ch.Close()
						code := 0
						if f.execFn != nil {
							code = f.execFn(command, ch, ch.Stderr(), cancelCh)
						}
						var status [4]byte
						binary.BigEndian.PutUint32(status[:], uint32(code))
						_, _ = ch.SendRequest("exit-status", false, status[:])
					}(cmd)
				case "signal":
					_ = req.Reply(true, nil)
					cancel()
				default:
					_ = req.Reply(false, nil)
				}
			}
			cancel()
		}()
	}
}

func postMCP(t *testing.T, url, token string, payload map[string]any) (*http.Response, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	_ = json.Unmarshal(body, &parsed)
	return resp, parsed
}

func TestProjectIter2_VersionConflict_ExecConcurrency_Git_And_MCPApproval(t *testing.T) {
	rootDir := t.TempDir()
	notePath := filepath.Join(rootDir, "notes.md")
	if err := os.WriteFile(notePath, []byte("retry = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := startFakeSSHExecServer(t, rootDir, func(cmd string, stdout, stderr io.Writer, cancel <-chan struct{}) int {
		switch {
		case strings.Contains(cmd, "sleep-long"):
			select {
			case <-cancel:
				_, _ = io.WriteString(stderr, "killed")
				return 137
			case <-time.After(10 * time.Second):
				return 0
			}
		case strings.Contains(cmd, "git rev-parse --show-toplevel"):
			_, _ = io.WriteString(stdout, "/srv/repo\n")
			return 0
		case strings.Contains(cmd, "git symbolic-ref --short HEAD"):
			_, _ = io.WriteString(stdout, "main\n")
			return 0
		case strings.Contains(cmd, "git status --porcelain=v1 -z -uall"):
			_, _ = io.WriteString(stdout, " M notes.md\x00?? CHANGELOG.md\x00")
			return 0
		case strings.Contains(cmd, "git diff --no-color -- 'notes.md'"):
			_, _ = io.WriteString(stdout, "--- a/notes.md\n+++ b/notes.md\n@@ -1 +1 @@\n-retry = 2\n+retry = 3\n")
			return 0
		case strings.Contains(cmd, "git show :0:'notes.md'"):
			_, _ = io.WriteString(stdout, "retry = 2\n")
			return 0
		case strings.Contains(cmd, "cat -- 'notes.md'"):
			_, _ = io.WriteString(stdout, "retry = 3\n")
			return 0
		default:
			_, _ = io.WriteString(stdout, "ok-from-remote\n")
			return 0
		}
	})

	pr, pw := io.Pipe()
	svc, err := NewService(t.TempDir(), pw)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer func() {
		svc.Close()
		_ = pw.Close()
		_ = pr.Close()
	}()
	svc.SetSecretStoreForTest(newMemorySecretStore())

	// 异步读取 sidecar 主动事件（用于捕获 project.approval.request）。
	eventsCh := make(chan Event, 16)
	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			var ev Event
			if json.Unmarshal(scanner.Bytes(), &ev) == nil && ev.Event != "" {
				eventsCh <- ev
			}
		}
	}()

	created := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionCreate, map[string]any{
		"name":     "Iter2 Box",
		"host":     "127.0.0.1",
		"port":     srv.hostPort,
		"user":     "dev",
		"password": "secret",
		"hostKey":  srv.authorizedHostKey(),
	}))
	connID := created["connection"].(map[string]any)["id"].(string)

	listResp := mustDataMap(t, callRPC(t, svc, MethodProjectFSList, map[string]any{"connectionId": connID, "path": ""}))
	remoteRoot := listResp["path"].(string)
	remoteNote := remoteRoot + "/notes.md"

	// 1. 乐观版本检查 + 冲突拦截 + force 覆盖
	read1 := mustDataMap(t, callRPC(t, svc, MethodProjectFSRead, map[string]any{"connectionId": connID, "path": remoteNote}))
	v1 := read1["version"].(string)
	// 外部修改文件
	_ = os.WriteFile(notePath, []byte("retry = 99\n"), 0o644)
	conflictResp := callRPC(t, svc, MethodProjectFSWrite, map[string]any{
		"connectionId":    connID,
		"path":            remoteNote,
		"content":         "retry = 3\n",
		"expectedVersion": v1,
	})
	if conflictResp.OK || conflictResp.Error == nil || conflictResp.Error.Code != CodeConflict {
		t.Fatalf("expected conflict error, got %+v", conflictResp)
	}
	// force=true 强制保存
	_ = mustDataMap(t, callRPC(t, svc, MethodProjectFSWrite, map[string]any{
		"connectionId": connID,
		"path":         remoteNote,
		"content":      "retry = 2\n",
		"force":        true,
	}))

	// 2. 长命令执行期间可并发浏览目录，且可按 opId 取消长命令
	execDone := make(chan Response, 1)
	go func() {
		execDone <- callRPC(t, svc, MethodProjectExec, map[string]any{
			"opId":         "op-long-1",
			"connectionId": connID,
			"remoteRoot":   remoteRoot,
			"command":      "sleep-long",
		})
	}()
	time.Sleep(100 * time.Millisecond)
	// 并发目录查询不被阻塞
	_ = mustDataMap(t, callRPC(t, svc, MethodProjectFSList, map[string]any{"connectionId": connID, "path": remoteRoot}))
	cancelResp := mustDataMap(t, callRPC(t, svc, MethodProjectExecCancel, map[string]any{"opId": "op-long-1"}))
	if cancelResp["cancelled"] != true {
		t.Fatalf("expected op-long-1 cancelled=true, got %+v", cancelResp)
	}
	longRes := mustDataMap(t, <-execDone)
	if longRes["cancelled"] != true {
		t.Fatalf("expected cancelled exec result, got %+v", longRes)
	}

	// 3. Git 状态与 diff 查询
	gitStatus := mustDataMap(t, callRPC(t, svc, MethodProjectGitStatus, map[string]any{
		"connectionId": connID,
		"remoteRoot":   remoteRoot,
	}))
	if gitStatus["branch"] != "main" {
		t.Fatalf("expected branch=main, got %+v", gitStatus)
	}
	changes := gitStatus["changes"].([]any)
	if len(changes) != 2 {
		t.Fatalf("expected 2 git changes, got %+v", changes)
	}
	gitDiff := mustDataMap(t, callRPC(t, svc, MethodProjectGitDiff, map[string]any{
		"connectionId": connID,
		"remoteRoot":   remoteRoot,
		"path":         "notes.md",
		"source":       "unstaged",
	}))
	if !strings.Contains(gitDiff["patch"].(string), "+retry = 3") {
		t.Fatalf("unexpected git diff: %+v", gitDiff)
	}

	// 4. 正式 /project-mcp HTTP 工具与审批闭环
	bind := mustDataMap(t, callRPC(t, svc, MethodProjectMCPBind, map[string]any{
		"projectId":    "rp_iter2",
		"connectionId": connID,
		"remoteRoot":   remoteRoot,
		"sessionHint":  "sess-1",
	}))
	mcpURL := bind["url"].(string)
	mcpToken := bind["token"].(string)

	// tools/list 应返回 4 个工具
	httpResp, listTools := postMCP(t, mcpURL, mcpToken, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list",
	})
	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("tools/list status=%d", httpResp.StatusCode)
	}
	toolList := listTools["result"].(map[string]any)["tools"].([]any)
	if len(toolList) != 4 {
		t.Fatalf("expected 4 MCP tools, got %d", len(toolList))
	}

	// read_file
	_, readMCP := postMCP(t, mcpURL, mcpToken, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": "read_file", "arguments": map[string]any{"path": "notes.md"}},
	})
	readTextJSON := readMCP["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	var readPayload map[string]any
	_ = json.Unmarshal([]byte(readTextJSON), &readPayload)
	curVersion := readPayload["version"].(string)

	// edit_file（批准路径）
	editDone := make(chan map[string]any, 1)
	go func() {
		_, parsed := postMCP(t, mcpURL, mcpToken, map[string]any{
			"jsonrpc": "2.0", "id": 3, "method": "tools/call",
			"params": map[string]any{
				"name": "edit_file",
				"arguments": map[string]any{
					"path": "notes.md", "version": curVersion, "old_text": "retry = 2", "new_text": "retry = 3",
				},
			},
		})
		editDone <- parsed
	}()
	ev := <-eventsCh
	if ev.Event != EventProjectApprovalRequest {
		t.Fatalf("expected %s event, got %s", EventProjectApprovalRequest, ev.Event)
	}
	evParams := ev.Params.(map[string]any)
	_ = mustDataMap(t, callRPC(t, svc, MethodProjectApprovalReply, map[string]any{
		"requestId": evParams["requestId"].(string),
		"approved":  true,
	}))
	editRes := <-editDone
	if editRes["result"].(map[string]any)["isError"] == true {
		t.Fatalf("expected edit_file ok, got %+v", editRes)
	}
	afterBytes, _ := os.ReadFile(notePath)
	if string(afterBytes) != "retry = 3\n" {
		t.Fatalf("expected edited file 'retry = 3\\n', got %q", string(afterBytes))
	}

	// write_file（拒绝路径：远端文件不得创建）
	writeDone := make(chan map[string]any, 1)
	go func() {
		_, parsed := postMCP(t, mcpURL, mcpToken, map[string]any{
			"jsonrpc": "2.0", "id": 4, "method": "tools/call",
			"params": map[string]any{
				"name": "write_file",
				"arguments": map[string]any{"path": "DENIED.md", "content": "nope"},
			},
		})
		writeDone <- parsed
	}()
	ev2 := <-eventsCh
	ev2Params := ev2.Params.(map[string]any)
	_ = mustDataMap(t, callRPC(t, svc, MethodProjectApprovalReply, map[string]any{
		"requestId": ev2Params["requestId"].(string),
		"approved":  false,
	}))
	writeRes := <-writeDone
	if writeRes["result"].(map[string]any)["isError"] != true {
		t.Fatalf("expected denied write_file to return isError=true, got %+v", writeRes)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "DENIED.md")); !os.IsNotExist(err) {
		t.Fatalf("DENIED.md must not exist after denial")
	}

	// 更新连接后旧 token 立即失效（401）
	_ = mustDataMap(t, callRPC(t, svc, MethodProjectConnectionUpdate, map[string]any{
		"id":      connID,
		"name":    "Iter2 Box Updated",
		"host":    "127.0.0.1",
		"port":    srv.hostPort,
		"user":    "dev",
		"hostKey": srv.authorizedHostKey(),
	}))
	unauthResp, _ := postMCP(t, mcpURL, mcpToken, map[string]any{
		"jsonrpc": "2.0", "id": 5, "method": "tools/list",
	})
	if unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after connection update, got %d", unauthResp.StatusCode)
	}
}

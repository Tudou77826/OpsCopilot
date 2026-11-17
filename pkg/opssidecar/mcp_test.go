package opssidecar

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/agenttransport/mcp"
)

// reservePort 取一个可用回环端口（测试内固定给 sidecar 桥用，避免与
// 本机运行中的真实 sidecar 冲突）。
func reservePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("取端口失败: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("释放端口失败: %v", err)
	}
	return fmt.Sprintf("%d", port)
}

// callTool 直接走网关分发（不经 socket），返回工具结果或错误。
func callTool(t *testing.T, f *fixture, name string, args map[string]any) (json.RawMessage, error) {
	t.Helper()
	gateway := &mcpGateway{svc: f.svc, token: "test-token"}
	rawArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("编码工具参数失败: %v", err)
	}
	params, _ := json.Marshal(map[string]any{
		"name":      name,
		"arguments": json.RawMessage(rawArgs), // RawMessage 保原文，避免被编码成 base64
	})
	raw, err := gateway.handleMessage(mustJSON(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      "t1",
		"method":  "tools/call",
		"params":  json.RawMessage(params), // 同上，保原文
	}))
	if err != nil {
		return nil, err
	}
	var reply struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &reply) != nil {
		t.Fatalf("工具应答解析失败: %s", raw)
	}
	if reply.Error != nil {
		t.Fatalf("JSON-RPC 层错误: %s（原始应答 %s）", reply.Error.Message, raw)
	}
	if len(reply.Result.Content) == 0 {
		t.Fatalf("无内容应答: %s", raw)
	}
	if reply.Result.IsError {
		return nil, fmt.Errorf("%s", reply.Result.Content[0].Text)
	}
	return json.RawMessage(reply.Result.Content[0].Text), nil
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}
	return raw
}

func newBridgeFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv(bridgePortEnv, reservePort(t))
	return newFixture(t)
}

func TestMCPInitializeAndToolsList(t *testing.T) {
	f := newBridgeFixture(t)
	gateway := &mcpGateway{svc: f.svc, token: "tok"}

	raw, err := gateway.handleMessage(mustJSON(t, map[string]any{
		"jsonrpc": "2.0", "id": "1", "method": "initialize",
		"params": map[string]any{"protocolVersion": "2024-11-05"},
	}))
	if err != nil {
		t.Fatalf("initialize 失败: %v", err)
	}
	if !strings.Contains(string(raw), "ops-sidecar") {
		t.Fatalf("initialize 应答缺 serverInfo: %s", raw)
	}

	raw, err = gateway.handleMessage(mustJSON(t, map[string]any{
		"jsonrpc": "2.0", "id": "2", "method": "tools/list",
	}))
	if err != nil {
		t.Fatalf("tools/list 失败: %v", err)
	}
	for _, tool := range []string{"remote_exec", "remote_file_upload", "knowledge_search", "terminal_observe", "terminal_write"} {
		if !strings.Contains(string(raw), tool) {
			t.Fatalf("tools/list 缺少 %s: %s", tool, raw)
		}
	}
}

func TestMCPConnectionsListAndExecDeny(t *testing.T) {
	f := newBridgeFixture(t)

	servers, err := callTool(t, f, "connections_list", map[string]any{})
	if err != nil {
		t.Fatalf("connections_list 失败: %v", err)
	}
	if !strings.Contains(string(servers), "test-server") {
		t.Fatalf("服务器清单缺 test-server: %s", servers)
	}

	// 白名单拒绝：命令不在线上白名单时，结构化错误直达工具应答（不碰网络）
	if _, err := callTool(t, f, "remote_exec", map[string]any{
		"server": "test-server", "command": "definitely-not-allowed-cmd",
	}); err == nil {
		t.Fatal("非白名单命令应被拒绝")
	}
	// 审计文件已落盘
	if _, err := os.Stat(filepath.Join(f.dataDir, auditFileName)); err != nil {
		t.Fatalf("审计文件未生成: %v", err)
	}
}

func TestMCPTransferTools(t *testing.T) {
	f := newBridgeFixture(t)
	localDir := filepath.Join(f.dataDir, "..", "local")
	payload := []byte("mcp-transfer-payload")
	src := filepath.Join(localDir, "mcp-src.bin")
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatalf("准备源文件失败: %v", err)
	}

	if _, err := callTool(t, f, "remote_file_upload", map[string]any{
		"server": "test-server", "local_path": src, "remote_path": "/data/mcp.bin", "mkdir": true,
	}); err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(f.sftpRoot, "data", "mcp.bin"))
	if err != nil || string(got) != string(payload) {
		t.Fatalf("远端内容不一致: %v", err)
	}

	dst := filepath.Join(localDir, "mcp-dst.bin")
	if _, err := callTool(t, f, "remote_file_download", map[string]any{
		"server": "test-server", "remote_path": "/data/mcp.bin", "local_path": dst,
	}); err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	got, err = os.ReadFile(dst)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("下载内容不一致: %v", err)
	}
}

func TestMCPKnowledgeToolsWithoutDir(t *testing.T) {
	f := newBridgeFixture(t)
	if _, err := callTool(t, f, "knowledge_search", map[string]any{"query": "磁盘告警"}); err == nil {
		t.Fatal("知识库目录缺失时应报可操作错误")
	}
}

func TestMCPProxyForwardsViaServiceFile(t *testing.T) {
	f := newBridgeFixture(t)
	// fixture 的 Service 已写好发现文件；代理按文件直连转发
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		"",
	}, "\n")
	output := &strings.Builder{}
	code := RunMCPProxy(strings.NewReader(input), output, f.dataDir)
	if code != 0 {
		t.Fatalf("代理退出码 = %d", code)
	}
	var replies []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("代理输出不是合法 JSON 行: %v (%s)", err, line)
		}
		replies = append(replies, payload)
	}
	if len(replies) != 2 {
		t.Fatalf("应答数 = %d, want 2（通知无应答）: %s", len(replies), output.String())
	}
	if !strings.Contains(output.String(), "ops-sidecar") || !strings.Contains(output.String(), "remote_exec") {
		t.Fatalf("代理应答内容不完整: %s", output.String())
	}
}

func TestTerminalWriteApprovalFlow(t *testing.T) {
	f := newBridgeFixture(t)
	// manual 档位（默认）：terminal_write 挂起等待审批
	openData, rpcErr := f.call(t, MethodTerminalOpen, map[string]any{"serverId": f.serverID})
	if rpcErr != nil {
		t.Fatalf("打开终端失败: %+v", rpcErr)
	}
	sessionID, _ := openData["sessionId"].(string)

	done := make(chan error, 1)
	go func() {
		_, err := f.svc.terminalWriteForAgent(sessionID, "echo approved\r")
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if _, rpcErr := f.call(t, MethodTerminalApproveHandoff, map[string]any{
		"sessionId": sessionID, "approved": true,
	}); rpcErr != nil {
		t.Fatalf("审批失败: %+v", rpcErr)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("批准后的写入失败: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("写入未在批准后返回")
	}

	// 拒绝路径
	rejected := make(chan error, 1)
	go func() {
		_, err := f.svc.terminalWriteForAgent(sessionID, "echo rejected\r")
		rejected <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if _, rpcErr := f.call(t, MethodTerminalApproveHandoff, map[string]any{
		"sessionId": sessionID, "approved": false,
	}); rpcErr != nil {
		t.Fatalf("拒绝裁决失败: %+v", rpcErr)
	}
	select {
	case err := <-rejected:
		if err == nil || !strings.Contains(err.Error(), "拒绝") {
			t.Fatalf("拒绝应返回明确错误: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("拒绝后写入未返回")
	}

	// auto 档位：直接放行
	if err := f.svc.setApprovalMode("auto"); err != nil {
		t.Fatalf("切换审批档位失败: %v", err)
	}
	if _, err := f.svc.terminalWriteForAgent(sessionID, "echo auto\r"); err != nil {
		t.Fatalf("auto 档位写入失败: %v", err)
	}
}

func TestTerminalObserveTool(t *testing.T) {
	f := newBridgeFixture(t)
	openData, rpcErr := f.call(t, MethodTerminalOpen, map[string]any{"serverId": f.serverID})
	if rpcErr != nil {
		t.Fatalf("打开终端失败: %+v", rpcErr)
	}
	sessionID, _ := openData["sessionId"].(string)

	// fakessh 横幅经控制器 Feed 进入观察环
	deadline := time.Now().Add(3 * time.Second)
	var text string
	for time.Now().Before(deadline) {
		result, err := callTool(t, f, "terminal_observe", map[string]any{"session_id": sessionID})
		if err == nil && strings.Contains(string(result), fakesshBanner) {
			text = string(result)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(text, fakesshBanner) {
		t.Fatalf("观察环未收到横幅: %s", text)
	}
}

func TestIdleCheckRequiresQuietState(t *testing.T) {
	f := newBridgeFixture(t)
	// 刚触碰过：不满足空闲
	if f.svc.idleCheck() {
		t.Fatal("刚活动过不应判定空闲")
	}
	// 推老时间戳：无连接无会话无传输 → 空闲
	f.svc.lastTouch.Store(time.Now().Add(-2 * idleExitAfter).Unix())
	if !f.svc.idleCheck() {
		t.Fatal("超时且无占用应判定空闲")
	}
	// 有活跃终端时阻塞退出
	openData, rpcErr := f.call(t, MethodTerminalOpen, map[string]any{"serverId": f.serverID})
	if rpcErr != nil {
		t.Fatalf("打开终端失败: %+v", rpcErr)
	}
	if f.svc.idleCheck() {
		t.Fatal("存在活跃终端时不应判定空闲")
	}
	sessionID, _ := openData["sessionId"].(string)
	if _, rpcErr := f.call(t, MethodTerminalClose, map[string]any{"sessionId": sessionID}); rpcErr != nil {
		t.Fatalf("关闭终端失败: %+v", rpcErr)
	}
}

// 代理与 OpsCopilot 既有 stdio 桥使用同一信封协议（编译期引用防漂移）。
var _ = mcp.StdioAddressEnv

package shellsidecar

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"opscopilot/pkg/remote"
)

// fakeConnFull 实现 remote.Connection：stdin/stdout 走 io.Pipe，供测试注入
// 输出、捕获键入。Resize/Close 记录调用。
type fakeConnFull struct {
	stdinW  *io.PipeWriter
	stdoutW *io.PipeWriter
	stdinR  *io.PipeReader
	stdoutR *io.PipeReader
	mu      sync.Mutex
	resized [][2]int
	closed  bool
}

func newFakeConnFull() *fakeConnFull {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	return &fakeConnFull{stdinW: stdinW, stdoutW: stdoutW, stdinR: stdinR, stdoutR: stdoutR}
}

func (f *fakeConnFull) StartShell(cols, rows int) (io.WriteCloser, io.Reader, error) {
	return f.stdinW, f.stdoutR, nil
}
func (f *fakeConnFull) Resize(cols, rows int) error {
	f.mu.Lock()
	f.resized = append(f.resized, [2]int{cols, rows})
	f.mu.Unlock()
	return nil
}
func (f *fakeConnFull) Run(ctx context.Context, cmd string) (string, error) { return "", nil }
func (f *fakeConnFull) Healthy() bool                                       { return true }
func (f *fakeConnFull) Protocol() string                                    { return "ssh" }
func (f *fakeConnFull) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	_ = f.stdinW.Close()
	// 关 stdout 让泵退出
	_ = f.stdoutW.Close()
	return nil
}

type notification struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func newTestService(t *testing.T) (*TerminalService, *fakeConnFull, *[]notification) {
	t.Helper()
	fake := newFakeConnFull()
	svc := NewTerminalService("test")
	svc.dialer = func(*remote.ConnectConfig) (remote.Connection, error) { return fake, nil }
	var mu sync.Mutex
	notes := &[]notification{}
	svc.SetNotify(func(method string, params any) {
		data, _ := json.Marshal(params)
		mu.Lock()
		*notes = append(*notes, notification{Method: method, Params: data})
		mu.Unlock()
	})
	t.Cleanup(svc.Shutdown)
	return svc, fake, notes
}

func waitNotes(t *testing.T, notes *[]notification, mu *sync.Mutex, n int) []notification {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		out := append([]notification(nil), *notes...)
		mu.Unlock()
		if len(out) >= n {
			return out
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]notification(nil), *notes...)
}

func TestOpenTerminalPumpsOutputAndInput(t *testing.T) {
	var mu sync.Mutex
	fake := newFakeConnFull()
	svc := NewTerminalService("test")
	svc.dialer = func(*remote.ConnectConfig) (remote.Connection, error) { return fake, nil }
	notes := &[]notification{}
	svc.SetNotify(func(method string, params any) {
		data, _ := json.Marshal(params)
		mu.Lock()
		*notes = append(*notes, notification{Method: method, Params: data})
		mu.Unlock()
	})
	t.Cleanup(svc.Shutdown)

	connID, err := svc.Connect(remote.ConnectConfig{Host: "h", User: "u"})
	if err != nil {
		t.Fatal(err)
	}
	termID, err := svc.OpenTerminal(connID, 120, 40)
	if err != nil {
		t.Fatal(err)
	}

	att, err := svc.Attach(termID)
	if err != nil {
		t.Fatal(err)
	}
	defer att.Detach()

	// 输出注入 → 订阅者收到
	fake.stdoutW.Write([]byte("hello pty"))
	select {
	case got := <-att.Ch:
		if string(got) != "hello pty" {
			t.Fatalf("got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting output")
	}

	// 键入 → fake stdin
	go func() {
		_ = svc.WriteInput(termID, []byte("ls\r"))
	}()
	buf := make([]byte, 3)
	if _, err := io.ReadFull(fake.stdinR, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ls\r" {
		t.Fatalf("stdin got %q", buf)
	}

	// 主动关闭 → terminal/exited 通知 + 订阅通道关闭
	if err := svc.CloseTerminal(termID); err != nil {
		t.Fatal(err)
	}
	if _, open := <-att.Ch; open {
		t.Fatal("subscriber channel should be closed after terminal exit")
	}
	got := waitNotes(t, notes, &mu, 1)
	found := false
	for _, n := range got {
		if n.Method == "terminal/exited" && strings.Contains(string(n.Params), termID) {
			found = true
		}
	}
	if !found {
		t.Fatalf("terminal/exited notification missing: %+v", got)
	}
	// 退出后 Attach 失败
	if _, err := svc.Attach(termID); err == nil {
		t.Fatal("attach after exit should fail")
	}
}

func TestReplayBufferKeepsTailForReattach(t *testing.T) {
	fake := newFakeConnFull()
	svc := NewTerminalService("test")
	svc.dialer = func(*remote.ConnectConfig) (remote.Connection, error) { return fake, nil }
	t.Cleanup(svc.Shutdown)

	connID, _ := svc.Connect(remote.ConnectConfig{Host: "h"})
	termID, err := svc.OpenTerminal(connID, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	big := bytes.Repeat([]byte("x"), replayBufferSize+4096)
	if _, err := fake.stdoutW.Write(big); err != nil {
		t.Fatal(err)
	}
	// 等泵消费
	time.Sleep(150 * time.Millisecond)
	att, err := svc.Attach(termID)
	if err != nil {
		t.Fatal(err)
	}
	defer att.Detach()
	if len(att.Replay) != replayBufferSize {
		t.Fatalf("replay size = %d, want %d", len(att.Replay), replayBufferSize)
	}
	if !bytes.Equal(att.Replay, big[len(big)-replayBufferSize:]) {
		t.Fatal("replay is not the tail of the stream")
	}
}

func TestDisconnectClosesAllTerminalsAndNotifies(t *testing.T) {
	var mu sync.Mutex
	fake := newFakeConnFull()
	svc := NewTerminalService("test")
	svc.dialer = func(*remote.ConnectConfig) (remote.Connection, error) { return fake, nil }
	notes := &[]notification{}
	svc.SetNotify(func(method string, params any) {
		data, _ := json.Marshal(params)
		mu.Lock()
		*notes = append(*notes, notification{Method: method, Params: data})
		mu.Unlock()
	})
	t.Cleanup(svc.Shutdown)

	connID, _ := svc.Connect(remote.ConnectConfig{Host: "h"})
	term1, _ := svc.OpenTerminal(connID, 80, 24)
	term2, _ := svc.OpenTerminal(connID, 80, 24)
	att1, _ := svc.Attach(term1)
	att2, _ := svc.Attach(term2)

	if err := svc.Disconnect(connID); err != nil {
		t.Fatal(err)
	}
	// 两个订阅通道都应关闭
	for _, att := range []*Attachment{att1, att2} {
		select {
		case _, open := <-att.Ch:
			if open {
				t.Fatal("channel should be closed")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting channel close")
		}
	}
	got := waitNotes(t, notes, &mu, 2)
	exited := 0
	for _, n := range got {
		if n.Method == "terminal/exited" {
			exited++
		}
	}
	if exited != 2 {
		t.Fatalf("expected 2 exited notifications, got %d: %+v", exited, got)
	}
	// 连接已关：fake stdout 写入应失败
	if _, err := fake.stdoutW.Write([]byte("x")); err == nil {
		t.Fatal("fake connection should be closed after disconnect")
	}
}

func TestResizeReachesConnection(t *testing.T) {
	fake := newFakeConnFull()
	svc := NewTerminalService("test")
	svc.dialer = func(*remote.ConnectConfig) (remote.Connection, error) { return fake, nil }
	t.Cleanup(svc.Shutdown)

	connID, _ := svc.Connect(remote.ConnectConfig{Host: "h"})
	termID, _ := svc.OpenTerminal(connID, 80, 24)
	if err := svc.Resize(termID, 132, 43); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.resized) != 1 || fake.resized[0] != [2]int{132, 43} {
		t.Fatalf("resize not propagated: %v", fake.resized)
	}
}

// RPC 层：stdio 行协议往返。
func TestServeControlRoundTrip(t *testing.T) {
	fake := newFakeConnFull()
	svc := NewTerminalService("test")
	svc.dialer = func(*remote.ConnectConfig) (remote.Connection, error) { return fake, nil }
	t.Cleanup(svc.Shutdown)

	stdinR, stdinW := io.Pipe()
	outBuf := &syncBuffer{}
	writer := &RPCWriter{Out: outBuf}
	api := &ControlAPI{Service: svc, Version: "test", Token: "tok", wsBase: "ws://127.0.0.1:1"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ServeControl(ctx, stdinR, writer, api)

	send := func(id int, method string, params any) {
		data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		stdinW.Write(append(data, '\n'))
	}

	// initialize
	send(1, "initialize", nil)
	resp := waitResponse(t, outBuf, 1)
	init := resp.Result.(map[string]any)
	if init["protocol"].(float64) != protocolVersion || init["token"] != "tok" || init["wsBase"] != "ws://127.0.0.1:1" {
		t.Fatalf("initialize result unexpected: %+v", init)
	}

	// connect → openTerminal → resize
	send(2, "shell.connect", map[string]any{"config": map[string]any{"host": "h", "user": "u"}})
	resp = waitResponse(t, outBuf, 2)
	connID := resp.Result.(map[string]any)["connectionId"].(string)
	if !strings.HasPrefix(connID, "conn-") {
		t.Fatalf("connectionId = %q", connID)
	}

	send(3, "shell.openTerminal", map[string]any{"connectionId": connID, "cols": 100, "rows": 30})
	resp = waitResponse(t, outBuf, 3)
	termID := resp.Result.(map[string]any)["terminalId"].(string)

	send(4, "shell.resize", map[string]any{"terminalId": termID, "cols": 120, "rows": 40})
	waitResponse(t, outBuf, 4)
	fake.mu.Lock()
	if len(fake.resized) != 1 {
		t.Fatalf("resize not dispatched")
	}
	fake.mu.Unlock()

	// 未知方法 → 错误应答
	send(5, "shell.nope", nil)
	resp = waitResponse(t, outBuf, 5)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected -32601, got %+v", resp)
	}
}

type rpcLine struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func waitResponse(t *testing.T, buf *syncBuffer, id int) rpcLine {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	prefix := []byte(fmt.Sprintf(`"id":%d`, id))
	for time.Now().Before(deadline) {
		for _, line := range buf.lines() {
			if bytes.Contains(line, prefix) && !bytes.Contains(line, []byte(`"method"`)) {
				var parsed rpcLine
				if err := json.Unmarshal(line, &parsed); err != nil {
					t.Fatal(err)
				}
				var gotID int
				_ = json.Unmarshal(parsed.ID, &gotID)
				if gotID == id {
					return parsed
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting response id=%d; got %s", id, buf.String())
	return rpcLine{}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) lines() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	data := append([]byte(nil), b.buf.Bytes()...)
	var out [][]byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) > 0 {
			out = append(out, line)
		}
	}
	return out
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// S3：连接配置持久化——save/list/delete 往返 + Upsert 去重 + 重启恢复。
func TestConfigServicePersistence(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewConfigService(dir)
	if err != nil {
		t.Fatal(err)
	}
	id1, err := svc.Save(SaveInput{Name: "prod", Host: "10.0.0.1", Port: 22, User: "root", Password: "pw1"})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := svc.Save(SaveInput{Name: "staging", Host: "10.0.0.2", Port: 22, User: "root"})
	if err != nil {
		t.Fatal(err)
	}
	// 同 endpoint 再存：Upsert 更新而非新增
	same, err := svc.Save(SaveInput{Name: "prod-renamed", Host: "10.0.0.1", Port: 22, User: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if same != id1 {
		t.Fatalf("upsert should reuse id, got %s vs %s", same, id1)
	}
	sessions, err := svc.List()
	if err != nil || len(sessions) != 2 {
		t.Fatalf("expect 2 saved sessions, got %d err=%v", len(sessions), err)
	}
	// 重启恢复
	reopened, err := NewConfigService(dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := reopened.List()
	if err != nil || len(again) != 2 {
		t.Fatalf("persistence lost after reopen: %d err=%v", len(again), err)
	}
	// rename + delete
	if err := reopened.Rename(id2, "staging-2"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Delete(id1); err != nil {
		t.Fatal(err)
	}
	final, _ := reopened.List()
	if len(final) != 1 || final[0].ID != id2 || final[0].Name != "staging-2" {
		t.Fatalf("after delete/rename unexpected tree: %+v", final)
	}
}

// S3：RPC 配置方法与 --data-dir 未启用时的报错路径。
func TestConfigRPCMethods(t *testing.T) {
	svc := NewTerminalService("test")
	t.Cleanup(svc.Shutdown)
	api := &ControlAPI{Service: svc, Version: "test", Token: "tok"}
	stdinR, stdinW := io.Pipe()
	outBuf := &syncBuffer{}
	go ServeControl(context.Background(), stdinR, &RPCWriter{Out: outBuf}, api)

	send := func(id int, method string, params any) {
		data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		stdinW.Write(append(data, '\n'))
	}
	// 未启用 Configs：明确报错而不是 panic
	send(1, "shell.configs.list", nil)
	resp := waitResponse(t, outBuf, 1)
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "配置服务未启用") {
		t.Fatalf("expected configs-not-enabled error, got %+v", resp)
	}

	dir := t.TempDir()
	configs, err := NewConfigService(dir)
	if err != nil {
		t.Fatal(err)
	}
	api.Configs = configs
	send(2, "shell.configs.save", map[string]any{"name": "web1", "host": "10.1.1.1", "user": "ops", "password": "x"})
	resp = waitResponse(t, outBuf, 2)
	if resp.Error != nil {
		t.Fatalf("save failed: %+v", resp.Error)
	}
	send(3, "shell.configs.list", nil)
	resp = waitResponse(t, outBuf, 3)
	sessions := resp.Result.(map[string]any)["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("expect 1 saved, got %d", len(sessions))
	}
}

package opscli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/core/ops"
	"opscopilot/pkg/core/security"
	"opscopilot/pkg/remote"
)

type cliResponse struct {
	Success bool            `json:"success"`
	Status  string          `json:"status"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

type cliSSHFixture struct {
	listener  net.Listener
	config    *remote.ConnectConfig
	root      string
	closed    chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	conns     []net.Conn
	execs     atomic.Int32
}

func newCLISSHFixture(t *testing.T, root string) *cliSSHFixture {
	t.Helper()
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if meta.User() != "fixture" || string(password) != "dummy-password" {
			return nil, errors.New("fixture authentication failed")
		}
		return nil, nil
	}}
	serverConfig.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	var p int
	_, _ = fmt.Sscanf(port, "%d", &p)
	f := &cliSSHFixture{listener: ln, root: root, closed: make(chan struct{}), config: &remote.ConnectConfig{Host: "127.0.0.1", Port: p, User: "fixture", Password: "dummy-password"}}
	t.Cleanup(f.Close)
	go func() {
		for {
			nc, e := ln.Accept()
			if e != nil {
				return
			}
			f.mu.Lock()
			f.conns = append(f.conns, nc)
			f.mu.Unlock()
			go f.handle(nc, serverConfig)
		}
	}()
	return f
}

func (f *cliSSHFixture) Close() {
	f.closeOnce.Do(func() {
		close(f.closed)
		_ = f.listener.Close()
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, c := range f.conns {
			_ = c.Close()
		}
	})
}

func (f *cliSSHFixture) handle(raw net.Conn, config *ssh.ServerConfig) {
	defer raw.Close()
	conn, chans, reqs, err := ssh.NewServerConn(raw, config)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for chReq := range chans {
		if chReq.ChannelType() != "session" {
			_ = chReq.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		ch, requests, e := chReq.Accept()
		if e != nil {
			continue
		}
		go f.session(ch, requests)
	}
}

func (f *cliSSHFixture) session(ch ssh.Channel, requests <-chan *ssh.Request) {
	defer ch.Close()
	for req := range requests {
		switch req.Type {
		case "subsystem":
			var p struct{ Name string }
			_ = ssh.Unmarshal(req.Payload, &p)
			if p.Name != "sftp" {
				_ = req.Reply(false, nil)
				return
			}
			_ = req.Reply(true, nil)
			s, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(f.root))
			if err == nil {
				_ = s.Serve()
			}
			return
		case "exec":
			f.execs.Add(1)
			var p struct{ Command string }
			_ = ssh.Unmarshal(req.Payload, &p)
			if strings.Contains(p.Command, "unacknowledged") {
				return
			}
			_ = req.Reply(true, nil)
			cmd := p.Command
			if strings.Contains(cmd, "sleep") || strings.Contains(cmd, "long-running") {
				<-f.closed
				return
			}
			if strings.Contains(cmd, "large-output") {
				_, _ = ch.Write([]byte(strings.Repeat("L", 30000)))
			} else if strings.Contains(cmd, "does-not-exist") {
				_, _ = ch.Stderr().Write([]byte("fixture stderr"))
				_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{7}))
				return
			} else {
				_, _ = ch.Write([]byte("fixture stdout"))
				_, _ = ch.Stderr().Write([]byte("fixture stderr"))
			}
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{0}))
			return
		default:
			_ = req.Reply(false, nil)
		}
	}
}

func buildCLIBinary(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("OPSCOPILOT_TEST_CLI_BINARY"); path != "" {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("OPSCOPILOT_TEST_CLI_BINARY: %v", err)
		}
		return path
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Clean(filepath.Join(root, "..", ".."))
	dir := t.TempDir()
	name := "opscopilot-cli"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", path, "./cmd/opscopilot-cli")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	return path
}

func runCLI(t *testing.T, binary, root, keyFile, whitelist, filePolicy string, args []string, input string) (int, cliResponse, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "OPSCOPILOT_SERVER_ROOT="+root, "OPSCOPILOT_SERVER_KEY_FILE="+keyFile, "OPSCOPILOT_WHITELIST_PATH="+whitelist, "OPSCOPILOT_FILE_ACCESS_PATH="+filePolicy)
	cmd.Stdin = strings.NewReader(input)
	var out, diag bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diag
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	var response cliResponse
	if e := json.Unmarshal(out.Bytes(), &response); e != nil {
		t.Fatalf("CLI output is not JSON (code=%d): %q stderr=%q", code, out.String(), diag.String())
	}
	return code, response, out.String(), diag.String()
}

func assertNoSecretOnDisk(t *testing.T, root, secret string) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return err
		}
		b, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Contains(b, []byte(secret)) {
			t.Errorf("secret appears in plaintext file %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan Session root: %v", err)
	}
}

func writeDefaultCLIPolicies(t *testing.T, whitelist, filePolicy string) {
	t.Helper()
	for path, value := range map[string]any{
		whitelist:  security.DefaultWhitelistConfig(),
		filePolicy: security.DefaultFileAccessConfig(),
	} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal default policy: %v", err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatalf("write policy %s: %v", path, err)
		}
	}
}

func TestCLIProcessLifecycleSSHAndSFTP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("CLI SFTP fixture uses POSIX absolute paths")
	}
	binary := buildCLIBinary(t)
	root := t.TempDir()
	fixture := newCLISSHFixture(t, filepath.Join(t.TempDir(), "remote"))
	keyFile := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyFile, []byte("01234567890123456789012345678901"), 0600); err != nil {
		t.Fatal(err)
	}
	whitelist, filePolicy := filepath.Join(t.TempDir(), "white.json"), filepath.Join(t.TempDir(), "file.json")
	writeDefaultCLIPolicies(t, whitelist, filePolicy)
	code, resp, _, diag := runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"session", "init", "--session", "sess", "--environment", "prod"}, "")
	if code != 0 || !resp.Success {
		t.Fatalf("session init: code=%d resp=%+v", code, resp)
	}
	nodeJSON, _ := json.Marshal(map[string]any{"id": "node", "name": "fixture", "kind": "ssh", "ssh": fixture.config})
	code, resp, out, diag := runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"connection", "put", "--session", "sess", "--input", "-"}, string(nodeJSON))
	if code != 0 || !resp.Success || strings.Contains(out+diag, "dummy-password") {
		t.Fatalf("connection put leaked or failed: code=%d resp=%+v out=%q diag=%q", code, resp, out, diag)
	}
	assertNoSecretOnDisk(t, root, "dummy-password")
	badConfig := *fixture.config
	badConfig.Password = "wrong-password"
	badNode, _ := json.Marshal(map[string]any{"id": "bad", "kind": "ssh", "ssh": &badConfig})
	code, _, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"connection", "put", "--session", "sess", "--input", "-"}, string(badNode))
	if code != 0 {
		t.Fatalf("bad node put failed: code=%d", code)
	}
	code, resp, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"exec", "--session", "sess", "--node", "bad", "--command", "ls -la"}, "")
	if code == 0 || resp.Success || resp.Status != "authentication_failed" {
		t.Fatalf("bad credentials unexpectedly succeeded: code=%d resp=%+v", code, resp)
	}

	for i := 0; i < 2; i++ {
		code, resp, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"exec", "--session", "sess", "--node", "node", "--command", "ls -la"}, "")
		if code != 0 || !resp.Success {
			t.Fatalf("exec %d: code=%d resp=%+v", i, code, resp)
		}
	}
	code, resp, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"exec", "--session", "sess", "--node", "node", "--command", "cat large-output"}, "")
	var large ops.OnceResult
	if err := json.Unmarshal(resp.Data, &large); err != nil {
		t.Fatal(err)
	}
	if code != 0 || !resp.Success || !large.Truncated || len(large.Stdout) >= 30000 || large.StdoutBytes != 30000 {
		t.Fatalf("large exec: code=%d resp=%s", code, resp.Data)
	}
	code, resp, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"exec", "--session", "sess", "--node", "node", "--command", "cat does-not-exist"}, "")
	if code == 0 || resp.Success || resp.Status == "success" {
		t.Fatalf("non-zero exec unexpectedly succeeded: code=%d resp=%+v", code, resp)
	}
	var failed ops.OnceResult
	if err := json.Unmarshal(resp.Data, &failed); err != nil {
		t.Fatal(err)
	}
	if failed.ExitCode == nil || *failed.ExitCode != 7 || failed.Stderr != "fixture stderr" || !failed.CompletionKnown {
		t.Fatalf("lost real failure details: %+v", failed)
	}

	if err := os.MkdirAll(filepath.Join(fixture.root, "srv"), 0700); err != nil {
		t.Fatal(err)
	}
	remoteFile := filepath.Join(fixture.root, "srv", "fixture.txt")
	if err := os.WriteFile(remoteFile, []byte("sftp-fixture-content"), 0600); err != nil {
		t.Fatal(err)
	}
	code, resp, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"file", "download", "--session", "sess", "--node", "node", "--remote", remoteFile, "--local", "downloaded.txt"}, "")
	if code != 0 || !resp.Success {
		t.Fatalf("download: code=%d resp=%+v", code, resp)
	}
	got, err := os.ReadFile(filepath.Join(root, "sessions", "sess", "work", "downloaded.txt"))
	if err != nil || string(got) != "sftp-fixture-content" {
		t.Fatalf("downloaded file = %q err=%v", got, err)
	}
	code, resp, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"file", "download", "--session", "sess", "--node", "node", "--remote", remoteFile, "--local", "../escape.txt"}, "")
	if code == 0 || resp.Success {
		t.Fatalf("path escape unexpectedly succeeded: code=%d resp=%+v", code, resp)
	}
	code, resp, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"file", "download", "--session", "sess", "--node", "node", "--remote", remoteFile, "--local", "too-small.txt", "--max-bytes", "4"}, "")
	if code == 0 || resp.Success {
		t.Fatalf("oversized download unexpectedly succeeded: code=%d resp=%+v", code, resp)
	}
	if got := fixture.execs.Load(); got != 4 {
		t.Fatalf("fixture exec count = %d, want 4 successful/attempted command requests", got)
	}

	code, resp, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"session", "status", "--session", "sess"}, "")
	if code != 0 || !resp.Success {
		t.Fatalf("session status after process pause: code=%d resp=%+v", code, resp)
	}
	code, resp, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"session", "close", "--session", "sess"}, "")
	if code != 0 || !resp.Success {
		t.Fatalf("session close: code=%d resp=%+v", code, resp)
	}
	if _, err := os.Stat(filepath.Join(root, "sessions", "sess", "work")); !os.IsNotExist(err) {
		t.Fatalf("work directory remains: %v", err)
	}
	code, resp, _, _ = runCLI(t, binary, root, keyFile, whitelist, filePolicy, []string{"session", "init", "--session", "sess", "--environment", "prod"}, "")
	if code == 0 || resp.Success || resp.Status != "session_closed" {
		t.Fatalf("closed Session revived: code=%d resp=%+v", code, resp)
	}
}

func TestCLIProcessCloseCancelsLongCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("CLI process integration fixture targets POSIX SSH")
	}
	binary := buildCLIBinary(t)
	root := t.TempDir()
	fixture := newCLISSHFixture(t, filepath.Join(t.TempDir(), "remote"))
	keyFile := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyFile, []byte("01234567890123456789012345678901"), 0600); err != nil {
		t.Fatal(err)
	}
	white, files := filepath.Join(t.TempDir(), "white.json"), filepath.Join(t.TempDir(), "files.json")
	writeDefaultCLIPolicies(t, white, files)
	_, _, _, _ = runCLI(t, binary, root, keyFile, white, files, []string{"session", "init", "--session", "long", "--environment", "prod"}, "")
	nodeJSON, _ := json.Marshal(map[string]any{"id": "node", "kind": "ssh", "ssh": fixture.config})
	_, _, _, _ = runCLI(t, binary, root, keyFile, white, files, []string{"connection", "put", "--session", "long", "--input", "-"}, string(nodeJSON))
	type commandResult struct {
		code     int
		response cliResponse
	}
	commandDone := make(chan commandResult, 1)
	go func() {
		code, result, _, _ := runCLI(t, binary, root, keyFile, white, files, []string{"exec", "--session", "long", "--node", "node", "--command", "cat long-running", "--timeout-sec", "30"}, "")
		commandDone <- commandResult{code, result}
	}()
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for fixture.execs.Load() == 0 {
		select {
		case got := <-commandDone:
			t.Fatalf("command ended before reaching server: code=%d status=%s error=%s", got.code, got.response.Status, got.response.Error)
		case <-waitCtx.Done():
			t.Fatal("command never reached SSH server")
		case <-time.After(10 * time.Millisecond):
		}
	}
	code, resp, _, _ := runCLI(t, binary, root, keyFile, white, files, []string{"session", "close", "--session", "long"}, "")
	if code != 0 || !resp.Success {
		t.Fatalf("close while command running: code=%d resp=%+v", code, resp)
	}
	select {
	case result := <-commandDone:
		if result.code == 0 || result.response.Status != "cancelled" {
			t.Fatalf("close did not cancel active command: code=%d status=%s error=%s", result.code, result.response.Status, result.response.Error)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("long command process did not finish after close")
	}
}

func TestCLIProcessLostExecAcknowledgementIsUnknownNotSafeToRetry(t *testing.T) {
	binary := buildCLIBinary(t)
	root := t.TempDir()
	fixture := newCLISSHFixture(t, t.TempDir())
	key := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(key, []byte("01234567890123456789012345678901"), 0600); err != nil {
		t.Fatal(err)
	}
	white, files := filepath.Join(t.TempDir(), "white.json"), filepath.Join(t.TempDir(), "files.json")
	writeDefaultCLIPolicies(t, white, files)
	code, result, _, _ := runCLI(t, binary, root, key, white, files, []string{"session", "init", "--session", "unknown", "--environment", "test"}, "")
	if code != 0 || !result.Success {
		t.Fatal("init failed")
	}
	input, _ := json.Marshal(map[string]any{"id": "node", "kind": "ssh", "ssh": fixture.config})
	code, result, _, _ = runCLI(t, binary, root, key, white, files, []string{"connection", "put", "--session", "unknown", "--input", "-"}, string(input))
	if code != 0 || !result.Success {
		t.Fatal("put failed")
	}
	code, result, _, _ = runCLI(t, binary, root, key, white, files, []string{"exec", "--session", "unknown", "--node", "node", "--command", "cat unacknowledged"}, "")
	if code == 0 || result.Status != "unknown" || fixture.execs.Load() != 1 {
		t.Fatalf("lost ACK incorrectly claims ordinary failure: code=%d status=%s executed=%d", code, result.Status, fixture.execs.Load())
	}
}

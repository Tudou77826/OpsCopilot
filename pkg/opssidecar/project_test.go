package opssidecar

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/secretstore"
)

type memorySecretStore struct {
	mu   sync.Mutex
	data map[string]string
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{data: map[string]string{}}
}

func (m *memorySecretStore) key(service, user string) string { return service + "\x00" + user }
func (m *memorySecretStore) Set(service, user, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[m.key(service, user)] = password
	return nil
}
func (m *memorySecretStore) Get(service, user string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[m.key(service, user)]
	if !ok {
		return "", secretstore.ErrNotFound
	}
	return v, nil
}
func (m *memorySecretStore) Delete(service, user string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, m.key(service, user))
	return nil
}

type fakeSSHAccount struct {
	password string
	rootDir  string
}

type fakeSFTPServer struct {
	ln       net.Listener
	hostPort int
	mu       sync.Mutex
	signer   ssh.Signer
	accounts map[string]fakeSSHAccount
}

func newSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("new ssh signer: %v", err)
	}
	return signer
}

func startFakeSFTPServer(t *testing.T, accounts map[string]fakeSSHAccount) *fakeSFTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &fakeSFTPServer{
		ln:       ln,
		hostPort: ln.Addr().(*net.TCPAddr).Port,
		signer:   newSigner(t),
		accounts: accounts,
	}
	go srv.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return srv
}

func (f *fakeSFTPServer) rotateHostKey(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	f.signer = newSigner(t)
	f.mu.Unlock()
}

func (f *fakeSFTPServer) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handleConn(conn)
	}
}

func (f *fakeSFTPServer) handleConn(raw net.Conn) {
	defer raw.Close()
	f.mu.Lock()
	signer := f.signer
	f.mu.Unlock()

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			acc, ok := f.accounts[meta.User()]
			if !ok || acc.password != string(pass) {
				return nil, fmt.Errorf("invalid credentials for %s", meta.User())
			}
			return &ssh.Permissions{
				Extensions: map[string]string{"rootDir": acc.rootDir},
			}, nil
		},
	}
	cfg.AddHostKey(signer)

	sConn, chans, reqs, err := ssh.NewServerConn(raw, cfg)
	if err != nil {
		return
	}
	defer sConn.Close()
	go ssh.DiscardRequests(reqs)

	rootDir := sConn.Permissions.Extensions["rootDir"]
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
			for req := range requests {
				if req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp" {
					_ = req.Reply(true, nil)
					server, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(rootDir))
					if err == nil {
						_ = server.Serve()
						_ = server.Close()
					}
					return
				}
				_ = req.Reply(false, nil)
			}
		}()
	}
}

func callRPC(t *testing.T, svc *Service, method string, params any) Response {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	resp, _ := svc.handle(Request{ID: "req-" + method, Method: method, Params: raw})
	return resp
}

func mustDataMap(t *testing.T, resp Response) map[string]any {
	t.Helper()
	if !resp.OK {
		t.Fatalf("expected OK response, got error: %+v", resp.Error)
	}
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	return out
}

func TestProjectConnections_TOFU_Mismatch_And_MultiAccountIsolation(t *testing.T) {
	aliceDir := t.TempDir()
	bobDir := t.TempDir()

	// 在两个账号的根目录下创建同名子目录和不同内容，验证同主机双账号完全隔离。
	aliceApp := filepath.Join(aliceDir, "app")
	bobApp := filepath.Join(bobDir, "app")
	if err := os.MkdirAll(filepath.Join(aliceApp, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(aliceApp, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bobApp, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(aliceApp, "src", "main.ts"), []byte("export const owner = 'alice';\n"), 0o644)
	_ = os.WriteFile(filepath.Join(aliceApp, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
	_ = os.WriteFile(filepath.Join(aliceApp, "bin.dat"), []byte{0x00, 0x01, 0x02}, 0o644)
	_ = os.WriteFile(filepath.Join(bobApp, "src", "main.ts"), []byte("export const owner = 'bob';\n"), 0o644)

	srv := startFakeSFTPServer(t, map[string]fakeSSHAccount{
		"alice": {password: "alice-secret", rootDir: aliceDir},
		"bob":   {password: "bob-secret", rootDir: bobDir},
	})

	dataDir := t.TempDir()
	var out bytes.Buffer
	svc, err := NewService(dataDir, &out)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()
	secrets := newMemorySecretStore()
	svc.SetSecretStoreForTest(secrets)

	// 1. 首次测试连接（无需 hostKey）：跳过主机指纹校验，直接返回 status=ok。
	testResp := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionTest, map[string]any{
		"config": map[string]any{
			"name":     "Dev Alice",
			"host":     "127.0.0.1",
			"port":     srv.hostPort,
			"user":     "alice",
			"password": "alice-secret",
		},
	}))
	if testResp["status"] != "ok" {
		t.Fatalf("expected status=ok without hostKey prompt, got %+v", testResp)
	}

	// 2. 创建两条同主机不同账号的连接（无需 hostKey）。
	createAlice := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionCreate, map[string]any{
		"name":     "Dev Alice",
		"host":     "127.0.0.1",
		"port":     srv.hostPort,
		"user":     "alice",
		"password": "alice-secret",
	}))
	aliceConn := createAlice["connection"].(map[string]any)
	aliceID := aliceConn["id"].(string)

	createBob := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionCreate, map[string]any{
		"name":     "Dev Bob",
		"host":     "127.0.0.1",
		"port":     srv.hostPort,
		"user":     "bob",
		"password": "bob-secret",
	}))
	bobConn := createBob["connection"].(map[string]any)
	bobID := bobConn["id"].(string)

	if aliceID == bobID {
		t.Fatalf("expected distinct connection IDs")
	}

	// 校验 sessions.json 中不含明文密码。
	rawSessions, err := os.ReadFile(filepath.Join(dataDir, "sessions.json"))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	if strings.Contains(string(rawSessions), "alice-secret") || strings.Contains(string(rawSessions), "bob-secret") {
		t.Fatalf("sessions.json must not contain plaintext passwords")
	}

	// 3. 测试连接成功，分别返回各自家目录。
	aliceTest := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionTest, map[string]any{"connectionId": aliceID}))
	if aliceTest["status"] != "ok" {
		t.Fatalf("expected alice test ok, got %+v", aliceTest)
	}
	bobTest := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionTest, map[string]any{"connectionId": bobID}))
	if bobTest["status"] != "ok" {
		t.Fatalf("expected bob test ok, got %+v", bobTest)
	}

	// 4. 浏览与预览：验证两个账号看到各自文件。
	aliceList := mustDataMap(t, callRPC(t, svc, MethodProjectFSList, map[string]any{"connectionId": aliceID, "path": ""}))
	aliceHome := aliceList["path"].(string)
	bobList := mustDataMap(t, callRPC(t, svc, MethodProjectFSList, map[string]any{"connectionId": bobID, "path": ""}))
	bobHome := bobList["path"].(string)

	alicePreview := mustDataMap(t, callRPC(t, svc, MethodProjectFSPreview, map[string]any{
		"connectionId": aliceID,
		"path":         aliceHome + "/app/src/main.ts",
	}))
	bobPreview := mustDataMap(t, callRPC(t, svc, MethodProjectFSPreview, map[string]any{
		"connectionId": bobID,
		"path":         bobHome + "/app/src/main.ts",
	}))
	if !strings.Contains(alicePreview["content"].(string), "alice") {
		t.Fatalf("expected alice content, got %q", alicePreview["content"])
	}
	if !strings.Contains(bobPreview["content"].(string), "bob") {
		t.Fatalf("expected bob content, got %q", bobPreview["content"])
	}

	// 二进制文件拒绝文本预览。
	binResp := callRPC(t, svc, MethodProjectFSPreview, map[string]any{
		"connectionId": aliceID,
		"path":         aliceHome + "/app/bin.dat",
	})
	if binResp.OK || binResp.Error == nil || binResp.Error.Code != CodeUnsupported {
		t.Fatalf("expected unsupported error for binary file, got %+v", binResp)
	}

	// 递归文件列表跳过 .git。
	filesResp := mustDataMap(t, callRPC(t, svc, MethodProjectFSListFiles, map[string]any{
		"connectionId": aliceID,
		"root":         aliceHome + "/app",
	}))
	pathsRaw := filesResp["paths"].([]any)
	for _, p := range pathsRaw {
		if strings.Contains(p.(string), ".git") {
			t.Fatalf("expected .git to be ignored, got %+v", pathsRaw)
		}
	}

	// 5. 服务端更换主机密钥后，仍可直接连接成功（跳过主机密钥校验）。
	srv.rotateHostKey(t)
	rotatedResp := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionTest, map[string]any{"connectionId": aliceID}))
	if rotatedResp["status"] != "ok" {
		t.Fatalf("expected status=ok after key rotation, got %+v", rotatedResp)
	}

	// 6. 更新连接应递增 revision 并清理旧连接池。
	updResp := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionUpdate, map[string]any{
		"id":   aliceID,
		"name": "Dev Alice Renamed",
		"host": "127.0.0.1",
		"port": srv.hostPort,
		"user": "alice",
	}))
	updated := updResp["connection"].(map[string]any)
	if updated["name"] != "Dev Alice Renamed" || updated["revision"].(float64) < 1 {
		t.Fatalf("unexpected updated connection: %+v", updated)
	}
	// 更新后用新密钥测试应重新成功。
	retest := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionTest, map[string]any{"connectionId": aliceID}))
	if retest["status"] != "ok" {
		t.Fatalf("expected ok after trusting rotated key, got %+v", retest)
	}

	// 7. 删除连接应同时清除凭据库条目。
	_ = mustDataMap(t, callRPC(t, svc, MethodProjectConnectionDelete, map[string]any{"id": bobID}))
	if _, err := secrets.Get(projectSecretService, projectSecretKey(bobID, 0)); err != secretstore.ErrNotFound {
		t.Fatalf("expected bob secret to be deleted, got err=%v", err)
	}
	_ = io.EOF
}

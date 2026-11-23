package opssidecar

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func startKeyAuthSSHServer(t *testing.T, rootDir string, allowedPub ssh.PublicKey) (int, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hostSigner := newSigner(t)
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				cfg := &ssh.ServerConfig{
					PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
						if meta.User() == "keyuser" && bytes.Equal(key.Marshal(), allowedPub.Marshal()) {
							return &ssh.Permissions{}, nil
						}
						return nil, fmt.Errorf("unauthorized key")
					},
				}
				cfg.AddHostKey(hostSigner)
				sConn, chans, reqs, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					return
				}
				defer sConn.Close()
				go ssh.DiscardRequests(reqs)
				for nc := range chans {
					if nc.ChannelType() != "session" {
						_ = nc.Reject(ssh.UnknownChannelType, "unsupported")
						continue
					}
					ch, reqs, err := nc.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer ch.Close()
						for req := range reqs {
							if req.Type == "subsystem" && len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp" {
								_ = req.Reply(true, nil)
								srv, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(rootDir))
								if err == nil {
									_ = srv.Serve()
									_ = srv.Close()
								}
								return
							}
							_ = req.Reply(false, nil)
						}
					}()
				}
			}(raw)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hostSigner.PublicKey())))
}

func TestProjectIter3_KeyAuth_And_TerminalPrepareAskpass(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	keyDir := t.TempDir()
	// 1. 生成明文私钥与带口令加密私钥
	plainBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	plainKeyPath := filepath.Join(keyDir, "id_ed25519")
	if err := os.WriteFile(plainKeyPath, pem.EncodeToMemory(plainBlock), 0o600); err != nil {
		t.Fatal(err)
	}

	encBlock, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte("my-key-pass"))
	if err != nil {
		t.Fatal(err)
	}
	encKeyPath := filepath.Join(keyDir, "id_ed25519_enc")
	if err := os.WriteFile(encKeyPath, pem.EncodeToMemory(encBlock), 0o600); err != nil {
		t.Fatal(err)
	}

	rootDir := t.TempDir()
	port, hostKey := startKeyAuthSSHServer(t, rootDir, sshPub)

	var out bytes.Buffer
	svc, err := NewService(t.TempDir(), &out)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	svc.SetSecretStoreForTest(newMemorySecretStore())

	// 明文私钥认证通过
	plainConn := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionCreate, map[string]any{
		"name":           "Plain Key Box",
		"host":           "127.0.0.1",
		"port":           port,
		"user":           "keyuser",
		"authType":       "key",
		"privateKeyPath": plainKeyPath,
		"hostKey":        hostKey,
	}))["connection"].(map[string]any)
	testPlain := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionTest, map[string]any{
		"connectionId": plainConn["id"].(string),
	}))
	if testPlain["status"] != "ok" {
		t.Fatalf("expected plain key auth ok, got %+v", testPlain)
	}

	// 加密私钥 + 口令认证通过
	encConn := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionCreate, map[string]any{
		"name":           "Encrypted Key Box",
		"host":           "127.0.0.1",
		"port":           port,
		"user":           "keyuser",
		"authType":       "key",
		"privateKeyPath": encKeyPath,
		"password":       "my-key-pass",
		"hostKey":        hostKey,
	}))["connection"].(map[string]any)
	encID := encConn["id"].(string)
	testEnc := mustDataMap(t, callRPC(t, svc, MethodProjectConnectionTest, map[string]any{
		"connectionId": encID,
	}))
	if testEnc["status"] != "ok" {
		t.Fatalf("expected encrypted key auth ok, got %+v", testEnc)
	}

	// project/terminal-prepare：生成 known_hosts + 一次性 askpass 票据
	prep := mustDataMap(t, callRPC(t, svc, MethodProjectTerminalPrepare, map[string]any{
		"connectionId": encID,
		"remoteRoot":   "/srv/app",
	}))
	khPath := prep["knownHostsPath"].(string)
	khBytes, err := os.ReadFile(khPath)
	if err != nil || !strings.Contains(string(khBytes), hostKey) {
		t.Fatalf("expected pinned hostKey in %s, got err=%v content=%q", khPath, err, string(khBytes))
	}
	envMap := prep["env"].(map[string]any)
	askURL := envMap["AAW_ASKPASS_URL"].(string)
	askTok := envMap["AAW_ASKPASS_TOKEN"].(string)
	if envMap["SSH_ASKPASS_REQUIRE"] != "force" {
		t.Fatalf("expected SSH_ASKPASS_REQUIRE=force, got %+v", envMap)
	}

	// 首次兑换返回口令
	req1, _ := http.NewRequest(http.MethodPost, askURL, nil)
	req1.Header.Set("Authorization", "Bearer "+askTok)
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK || string(body1) != "my-key-pass" {
		t.Fatalf("expected 200 + secret, got %d %q", resp1.StatusCode, string(body1))
	}

	// 二次重放必须返回 401（取走即焚）
	req2, _ := http.NewRequest(http.MethodPost, askURL, nil)
	req2.Header.Set("Authorization", "Bearer "+askTok)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 on replay, got %d", resp2.StatusCode)
	}
}

package shellsidecar

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/sshclient"
)

func TestHostKeyProbeDoesNotAuthenticateAndPinRejectsBeforePassword(t *testing.T) {
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(private)
	var authCalls atomic.Int32
	server := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { authCalls.Add(1); return nil, nil }}
	server.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				transport, _, _, err := ssh.NewServerConn(conn, server)
				if err == nil {
					transport.Close()
				}
			}()
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	key, err := ProbeHostKey(context.Background(), "127.0.0.1", port)
	if err != nil {
		t.Fatal(err)
	}
	if authCalls.Load() != 0 {
		t.Fatal("probe attempted authentication")
	}
	if key["fingerprint"] != ssh.FingerprintSHA256(signer.PublicKey()) {
		t.Fatal("incorrect fingerprint")
	}
	_, otherPrivate, _ := ed25519.GenerateKey(rand.Reader)
	otherSigner, _ := ssh.NewSignerFromKey(otherPrivate)
	config := &remote.ConnectConfig{Host: "127.0.0.1", Port: port, User: "test", Password: "sensitive-test-password", HostKey: string(ssh.MarshalAuthorizedKey(otherSigner.PublicKey()))}
	if client, err := sshclient.NewClient(config); err == nil {
		client.Close()
		t.Fatal("changed host key accepted")
	}
	if authCalls.Load() != 0 {
		t.Fatal("password was sent before host-key rejection")
	}
	config.HostKey = key["key"]
	client, err := sshclient.NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if authCalls.Load() != 1 {
		t.Fatal("confirmed host did not authenticate")
	}
}

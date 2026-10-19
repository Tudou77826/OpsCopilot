package sshclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/connectiondiag"
)

// This fixture only forwards to one test-owned loopback SSH endpoint. All other
// destinations are rejected. It never executes the fallback command it receives.
func diagnosticSSH(t *testing.T, login, forwardTo string) *ConnectConfig {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(private)
	server := &ssh.ServerConfig{ServerVersion: "SSH-2.0-OpsCopilot-Diagnostic-Fixture", PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if meta.User() != login || string(password) != "fixture-"+login+"-secret" {
			return nil, fmt.Errorf("authentication rejected")
		}
		return nil, nil
	}}
	server.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	connections := map[net.Conn]bool{}
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		defer mu.Unlock()
		for conn := range connections {
			conn.Close()
		}
	})
	go func() {
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			connections[raw] = true
			mu.Unlock()
			go func() {
				defer raw.Close()
				defer func() { mu.Lock(); delete(connections, raw); mu.Unlock() }()
				conn, channels, requests, err := ssh.NewServerConn(raw, server)
				if err != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(requests)
				for request := range channels {
					if request.ChannelType() == "session" {
						channel, requests, err := request.Accept()
						if err == nil {
							go func() {
								defer channel.Close()
								for req := range requests {
									_ = req.Reply(false, nil)
								}
							}()
						}
						continue
					}
					var destination struct {
						Host       string
						Port       uint32
						OriginHost string
						OriginPort uint32
					}
					if request.ChannelType() != "direct-tcpip" || ssh.Unmarshal(request.ExtraData(), &destination) != nil || net.JoinHostPort(destination.Host, fmt.Sprint(destination.Port)) != forwardTo {
						_ = request.Reject(ssh.Prohibited, "fixture destination denied")
						continue
					}
					remote, err := net.Dial("tcp", forwardTo)
					if err != nil {
						_ = request.Reject(ssh.ConnectionFailed, "fixture connect failed")
						continue
					}
					channel, requests, err := request.Accept()
					if err != nil {
						remote.Close()
						continue
					}
					go ssh.DiscardRequests(requests)
					go func() { _, _ = io.Copy(channel, remote); channel.Close(); remote.Close() }()
					go func() { _, _ = io.Copy(remote, channel); channel.Close(); remote.Close() }()
				}
			}()
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	n, _ := strconv.Atoi(port)
	return &ConnectConfig{Host: host, Port: n, User: login, Password: "fixture-" + login + "-secret"}
}

func TestDiagnosticSSHMultiHopAndFinalRejection(t *testing.T) {
	final := diagnosticSSH(t, "fixture55", "")
	jump := diagnosticSSH(t, "fixture51", net.JoinHostPort(final.Host, fmt.Sprint(final.Port)))
	final.Bastion = jump
	ctx, trace := connectiondiag.New(context.Background())
	client, err := NewClientWithTrace(ctx, final)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.SSHClient().Dial("tcp", "127.0.0.1:32063")
	if err == nil {
		t.Fatal("fixture accepted a database channel")
	}
	RecordChannelFailure(ctx, "ssh.database.channel", err)
	report := trace.Finish()
	for _, expected := range []string{"fixture51", "fixture55", "ssh.hop.channel.ok", "ssh.database.channel.rejected", "原因码=1", "SHA256:", "OpsCopilot-Diagnostic-Fixture", "ssh.auth.password"} {
		if !strings.Contains(report, expected) {
			t.Fatalf("missing %s in trace: %s", expected, report)
		}
	}
	if trace.Has("ssh.hop.exec.start") || strings.Contains(report, jump.Password) || strings.Contains(report, final.Password) {
		t.Fatal("unnecessary fallback or credential leak")
	}
}

func TestDiagnosticSSHReportsExistingFallbackSeparately(t *testing.T) {
	jump := diagnosticSSH(t, "fixture51", "")
	ctx, trace := connectiondiag.New(context.Background())
	_, err := NewClientWithTrace(ctx, &ConnectConfig{Host: "127.0.0.1", Port: 32063, User: "fixture55", Password: "other-fixture-secret", Bastion: jump})
	if err == nil || !trace.Has("ssh.hop.channel.rejected") || !trace.Has("ssh.hop.exec.start") || !trace.Has("ssh.hop.exec.failed") || trace.Has("ssh.database.channel.rejected") {
		t.Fatalf("jump failure was not distinguished from final database rejection: %v\n%s", err, trace.Finish())
	}
}

func TestDiagnosticSSHHandshakeCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		peer, err := listener.Accept()
		if err != nil {
			return
		}
		defer peer.Close()
		io.Copy(io.Discard, peer)
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	n, _ := strconv.Atoi(port)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = NewClientWithTrace(ctx, &ConnectConfig{Host: host, Port: n, User: "fixture", Password: "fixture-only"})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatalf("setup cancellation: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pending handshake socket was not released")
	}
}

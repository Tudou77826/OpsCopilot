package terminalhandoff

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/sshclient"
)

type transcript struct {
	mu      sync.Mutex
	data    []byte
	changed chan struct{}
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func newTranscript() *transcript { return &transcript{changed: make(chan struct{})} }
func (v *transcript) append(p []byte) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.data = append(v.data, p...)
	close(v.changed)
	v.changed = make(chan struct{})
}
func (v *transcript) size() int    { v.mu.Lock(); defer v.mu.Unlock(); return len(v.data) }
func (v *transcript) text() string { v.mu.Lock(); defer v.mu.Unlock(); return string(v.data) }
func (v *transcript) wait(ctx context.Context, after int, needle string) error {
	for {
		v.mu.Lock()
		found := bytes.Contains(v.data[after:], []byte(needle))
		ch := v.changed
		v.mu.Unlock()
		if found {
			return nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return fmt.Errorf("waiting for %q: %w", needle, ctx.Err())
		}
	}
}

type labServer struct {
	listener net.Listener
	config   *ssh.ServerConfig
	mu       sync.Mutex
	peers    map[net.Conn]bool
	wg       sync.WaitGroup
	channels atomic.Int32
	execs    atomic.Int32
	shell    string
	helper   string
	lib      string
	distro   string
	client   *sshclient.Client
}

func startLab(t *testing.T, shell string) (*Controller, *transcript, *labServer) {
	return startLabUsing(t, shell, func(stdin io.WriteCloser) (*Controller, error) { return New(stdin) })
}

func startLabUsing(t *testing.T, shell string, attach func(io.WriteCloser) (*Controller, error)) (*Controller, *transcript, *labServer) {
	t.Helper()
	if runtime.GOOS != "windows" || os.Getenv("OPSCOPILOT_HANDOFF_FIXTURE") == "" {
		t.Skip("opt-in real PTY lab: set OPSCOPILOT_HANDOFF_FIXTURE and OPSCOPILOT_HANDOFF_SHELLS on Windows/WSL")
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(m ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		if m.User() == "lab" && string(p) == "isolated-fixture" {
			return nil, nil
		}
		return nil, errors.New("fixture authentication rejected")
	}}
	cfg.AddHostKey(signer)
	address := os.Getenv("OPSCOPILOT_HANDOFF_ADDR")
	if address == "" {
		address = "127.0.0.1:0"
	}
	host, _, addressErr := net.SplitHostPort(address)
	if addressErr != nil || host != "127.0.0.1" {
		t.Fatal("lab endpoint must bind to IPv4 loopback")
	}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	s := &labServer{listener: ln, config: cfg, peers: map[net.Conn]bool{}, helper: os.Getenv("OPSCOPILOT_HANDOFF_FIXTURE"), lib: os.Getenv("OPSCOPILOT_HANDOFF_LIB"), shell: shell, distro: os.Getenv("OPSCOPILOT_HANDOFF_DISTRO")}
	if s.distro == "" {
		s.distro = "docker-desktop"
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			peer, err := ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.peers[peer] = true
			s.wg.Add(1)
			s.mu.Unlock()
			go func() {
				defer s.wg.Done()
				defer func() { peer.Close(); s.mu.Lock(); delete(s.peers, peer); s.mu.Unlock() }()
				s.serve(peer)
			}()
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		s.mu.Lock()
		for peer := range s.peers {
			peer.Close()
		}
		s.mu.Unlock()
		done := make(chan struct{})
		go func() { s.wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("lab fixture failed to shut down")
		}
	})
	client, err := sshclient.NewClient(&sshclient.ConnectConfig{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, User: "lab", Password: "isolated-fixture", HostKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	s.client = client
	stdin, stdout, err := client.StartShell(180, 30)
	if err != nil {
		t.Fatal(err)
	}
	c, err := attach(stdin)
	if err != nil {
		t.Fatal(err)
	}
	v := newTranscript()
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer c.Closed()
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				v.append(c.Feed(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		client.Close()
		select {
		case <-readerDone:
		case <-time.After(3 * time.Second):
			t.Error("PTY reader did not terminate")
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := v.wait(ctx, 0, "LAB> "); err != nil {
		t.Fatalf("%v; output=%q", err, v.text())
	}
	return c, v, s
}

func (s *labServer) serve(peer net.Conn) {
	conn, chans, reqs, err := ssh.NewServerConn(peer, s.config)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for next := range chans {
		if next.ChannelType() != "session" {
			next.Reject(ssh.UnknownChannelType, "fixture supports session only")
			continue
		}
		ch, reqs, err := next.Accept()
		if err != nil {
			continue
		}
		s.channels.Add(1)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer ch.Close()
			for req := range reqs {
				switch req.Type {
				case "pty-req", "window-change", "env":
					req.Reply(true, nil)
				case "exec":
					s.execs.Add(1)
					req.Reply(false, nil)
					return
				case "shell":
					req.Reply(true, nil)
					// Real UIs keep sending resize requests after Shell starts.
					// Drain them while the process is running; otherwise the SSH
					// request queue eventually blocks the shared channel reader.
					s.wg.Add(1)
					go func() {
						defer s.wg.Done()
						for request := range reqs {
							request.Reply(request.Type == "window-change", nil)
						}
					}()
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					cmd := exec.CommandContext(ctx, "wsl.exe", "-d", s.distro, "-e", "env", "LD_LIBRARY_PATH="+s.lib, s.helper, s.shell)
					cmd.Env = append(os.Environ(), "WSL_UTF8=1")
					input, err := cmd.StdinPipe()
					if err != nil {
						return
					}
					cmd.Stdout = ch
					cmd.Stderr = ch.Stderr()
					if err := cmd.Start(); err != nil {
						fmt.Fprintln(ch.Stderr(), err)
						return
					}
					inputDone := make(chan struct{})
					go func() {
						var source io.Reader = ch
						if output := os.Getenv("OPSCOPILOT_HANDOFF_DESKTOP"); output != "" {
							if trace, err := os.OpenFile(output+".input", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err == nil {
								defer trace.Close()
								source = io.TeeReader(ch, trace)
							}
						}
						_, _ = io.Copy(input, source)
						input.Close()
						cancel()
						close(inputDone)
					}()
					_ = cmd.Wait()
					ch.Close()
					<-inputDone
					return
				default:
					req.Reply(false, nil)
				}
			}
		}()
	}
}

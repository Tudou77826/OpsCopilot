package filetransfer

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type testSSHServerOptions struct {
	RootDir    string
	EnableSFTP bool
	EnableSCP  bool
}

type testSSHServer struct {
	t      *testing.T
	ln     net.Listener
	cfg    *ssh.ServerConfig
	opts   testSSHServerOptions
	wg     sync.WaitGroup
	closed chan struct{}
}

func newTestSSHServer(t *testing.T, opts testSSHServerOptions) *testSSHServer {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "u" && string(pass) == "p" {
				return nil, nil
			}
			return nil, errors.New("unauthorized")
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	s := &testSSHServer{
		t:      t,
		ln:     ln,
		cfg:    cfg,
		opts:   opts,
		closed: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.serve()
	return s
}

func (s *testSSHServer) Addr() string {
	return s.ln.Addr().String()
}

func (s *testSSHServer) ClientConfig() *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            "u",
		Auth:            []ssh.AuthMethod{ssh.Password("p")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
}

func (s *testSSHServer) Close() {
	_ = s.ln.Close()
	close(s.closed)
	s.wg.Wait()
}

func (s *testSSHServer) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
				return
			}
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConn(c)
		}(conn)
	}
}

func (s *testSSHServer) handleConn(nc net.Conn) {
	defer nc.Close()

	sc, chans, reqs, err := ssh.NewServerConn(nc, s.cfg)
	if err != nil {
		return
	}
	defer sc.Close()

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		ch, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		s.wg.Add(1)
		go func(ch ssh.Channel, in <-chan *ssh.Request) {
			defer s.wg.Done()
			defer ch.Close()
			s.handleSession(ch, in)
		}(ch, requests)
	}
}

func (s *testSSHServer) handleSession(ch ssh.Channel, requests <-chan *ssh.Request) {
	for req := range requests {
		switch req.Type {
		case "subsystem":
			var payload struct {
				Name string
			}
			_ = ssh.Unmarshal(req.Payload, &payload)
			if payload.Name == "sftp" && s.opts.EnableSFTP {
				_ = req.Reply(true, nil)
				srv, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(s.opts.RootDir))
				if err != nil {
					return
				}
				_ = srv.Serve()
				return
			}
			_ = req.Reply(false, nil)
			return
		case "exec":
			var payload struct {
				Command string
			}
			_ = ssh.Unmarshal(req.Payload, &payload)
			cmd := strings.TrimSpace(payload.Command)

			if s.opts.EnableSCP && strings.HasPrefix(cmd, "scp -t ") {
				_ = req.Reply(true, nil)
				scpSink(ch, s.opts.RootDir, strings.TrimSpace(strings.TrimPrefix(cmd, "scp -t ")))
				_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
				return
			}
			if s.opts.EnableSCP && strings.HasPrefix(cmd, "scp -f ") {
				_ = req.Reply(true, nil)
				scpSource(ch, s.opts.RootDir, strings.TrimSpace(strings.TrimPrefix(cmd, "scp -f ")))
				_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
				return
			}

			if s.opts.EnableSCP && strings.Contains(cmd, "command -v scp") {
				_ = req.Reply(true, nil)
				_, _ = ch.Write([]byte("1\n"))
				_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
				return
			}

			_ = req.Reply(true, nil)
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 127}))
			return
		default:
			_ = req.Reply(false, nil)
		}
	}
}

func scpSink(ch ssh.Channel, rootDir, target string) {
	br := bufio.NewReader(ch)
	bw := bufio.NewWriter(ch)

	writeAck := func() {
		_ = bw.WriteByte(0)
		_ = bw.Flush()
	}
	writeErr := func(msg string) {
		_, _ = bw.WriteString("\x01" + msg + "\n")
		_ = bw.Flush()
	}

	writeAck()

	line, err := br.ReadString('\n')
	if err != nil {
		writeErr("read header failed")
		return
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "C") {
		writeErr("bad header")
		return
	}
	parts := strings.SplitN(line[1:], " ", 3)
	if len(parts) != 3 {
		writeErr("bad header parts")
		return
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		writeErr("bad size")
		return
	}
	name := parts[2]

	writeAck()

	b := make([]byte, size)
	if _, err := io.ReadFull(br, b); err != nil {
		writeErr("read data failed")
		return
	}
	_, _ = br.ReadByte()

	dstDir := strings.Trim(target, "'")
	if dstDir == "" {
		dstDir = "."
	}
	dst := filepath.Join(rootDir, filepath.FromSlash(dstDir), filepath.FromSlash(name))
	_ = os.MkdirAll(filepath.Dir(dst), 0755)
	_ = os.WriteFile(dst, b, 0644)

	writeAck()
}

func scpSource(ch ssh.Channel, rootDir, target string) {
	br := bufio.NewReader(ch)
	bw := bufio.NewWriter(ch)

	readAck := func() error {
		b, err := br.ReadByte()
		if err != nil {
			return err
		}
		if b == 0 {
			return nil
		}
		if b == 1 || b == 2 {
			msg, _ := br.ReadString('\n')
			return fmt.Errorf("%s", strings.TrimSpace(msg))
		}
		return fmt.Errorf("bad ack")
	}

	writeErr := func(msg string) {
		_, _ = bw.WriteString("\x01" + msg + "\n")
		_ = bw.Flush()
	}

	if err := readAck(); err != nil {
		return
	}

	p := strings.Trim(target, "'")
	src := filepath.Join(rootDir, filepath.FromSlash(p))
	b, err := os.ReadFile(src)
	if err != nil {
		writeErr("not found")
		return
	}
	name := filepath.Base(src)
	header := fmt.Sprintf("C0644 %d %s\n", len(b), name)
	_, _ = bw.WriteString(header)
	_ = bw.Flush()

	if err := readAck(); err != nil {
		return
	}

	_, _ = bw.Write(b)
	_ = bw.WriteByte(0)
	_ = bw.Flush()

	_ = readAck()
}

func writeTestFile(t *testing.T, root, p string, content []byte) {
	t.Helper()
	ap := filepath.Join(root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(ap), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ap, content, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readAll(r io.Reader) []byte {
	b, _ := io.ReadAll(r)
	return b
}

func equalBytes(a, b []byte) bool {
	return bytes.Equal(a, b)
}

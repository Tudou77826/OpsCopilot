// Package sshtest provides loopback-only SSH fixtures. It is only imported by tests.
package sshtest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/remote"
)

type CommandOptions struct {
	ForwardFailure ssh.RejectionReason
	MissingPython  bool
	Stall          bool
	Preamble       string
}

type CommandServer struct {
	Config   *remote.ConnectConfig
	Active   atomic.Int32
	Forwards atomic.Int32
	Commands atomic.Int32
}

func NewCommandServer(t *testing.T, options CommandOptions) *CommandServer {
	t.Helper()
	python := os.Getenv("OPSCOPILOT_TEST_PYTHON")
	if python == "" {
		name := "python3"
		if runtime.GOOS == "windows" {
			name = "python"
		}
		var err error
		python, err = exec.LookPath(name)
		if err != nil {
			t.Skip("Python 3 required for real command relay tests")
		}
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(key)
	config := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	s := &CommandServer{Config: &remote.ConnectConfig{Host: host, Port: portNumber, User: "fixture", Password: "fixture-owned-password"}}
	lifetime, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	owned := map[net.Conn]bool{}
	t.Cleanup(func() {
		listener.Close()
		cancel()
		mu.Lock()
		for conn := range owned {
			conn.Close()
		}
		mu.Unlock()
		s.WaitIdle(t)
	})
	go func() {
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			owned[raw] = true
			mu.Unlock()
			go func() {
				defer raw.Close()
				defer func() { mu.Lock(); delete(owned, raw); mu.Unlock() }()
				conn, channels, requests, err := ssh.NewServerConn(raw, config)
				if err != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(requests)
				for request := range channels {
					if request.ChannelType() != "session" {
						s.Forwards.Add(1)
						reason := options.ForwardFailure
						if reason == 0 {
							reason = ssh.Prohibited
						}
						_ = request.Reject(reason, "fixture rejects standard forwarding")
						continue
					}
					channel, requests, err := request.Accept()
					if err != nil {
						continue
					}
					go s.run(lifetime, python, channel, requests, options)
				}
			}()
		}
	}()
	return s
}

func (s *CommandServer) run(ctx context.Context, python string, channel ssh.Channel, requests <-chan *ssh.Request, options CommandOptions) {
	defer channel.Close()
	for request := range requests {
		if request.Type != "exec" {
			_ = request.Reply(false, nil)
			continue
		}
		s.Commands.Add(1)
		var payload struct{ Command string }
		if ssh.Unmarshal(request.Payload, &payload) != nil {
			_ = request.Reply(false, nil)
			return
		}
		// Parse the actual production POSIX shell quoting using Python's shlex.
		parser := exec.CommandContext(ctx, python, "-c", "import json,shlex,sys;print(json.dumps(shlex.split(sys.argv[1])))", payload.Command)
		out, err := parser.Output()
		var args []string
		if err != nil || json.Unmarshal(out, &args) != nil || len(args) != 9 || args[0] != "exec" || args[1] != "python3" || args[2] != "-u" || args[3] != "-c" {
			_ = request.Reply(false, nil)
			return
		}
		if options.MissingPython {
			_ = request.Reply(true, nil)
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{127}))
			return
		}
		processArgs := args[2:]
		if options.Stall {
			processArgs = []string{"-u", "-c", "import time;time.sleep(60)"}
		}
		cmd := exec.CommandContext(ctx, python, processArgs...)
		input, err := cmd.StdinPipe()
		if err != nil {
			_ = request.Reply(false, nil)
			return
		}
		output, err := cmd.StdoutPipe()
		if err != nil {
			_ = request.Reply(false, nil)
			return
		}
		errorOutput, err := cmd.StderrPipe()
		if err != nil {
			_ = request.Reply(false, nil)
			return
		}
		if options.Preamble != "" {
			_, _ = io.WriteString(channel, options.Preamble)
		}
		if err = cmd.Start(); err != nil {
			_ = request.Reply(false, nil)
			return
		}
		s.Active.Add(1)
		defer s.Active.Add(-1)
		_ = request.Reply(true, nil)
		var streams sync.WaitGroup
		streams.Add(2)
		go func() { defer streams.Done(); _, _ = io.Copy(channel, output) }()
		go func() { defer streams.Done(); _, _ = io.Copy(channel.Stderr(), errorOutput) }()
		go func() { _, _ = io.Copy(input, channel); input.Close() }()
		go func() {
			for req := range requests {
				if req.Type == "signal" {
					_ = cmd.Process.Kill()
				}
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
			}
			_ = cmd.Process.Kill()
		}()
		// Like sshd, EOF of both process output descriptors closes the channel's
		// outbound direction even while the process can still read stdin.
		streams.Wait()
		_ = channel.CloseWrite()
		err = cmd.Wait()
		code := uint32(0)
		if err != nil {
			code = 1
		}
		_ = channel.CloseWrite()
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{code}))
		return
	}
}

func (s *CommandServer) WaitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for s.Active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.Active.Load() != 0 {
		t.Errorf("fixture relay process remains active: %s", fmt.Sprint(s.Active.Load()))
	}
}

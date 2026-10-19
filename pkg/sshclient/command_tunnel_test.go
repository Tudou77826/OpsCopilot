package sshclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"opscopilot/internal/sshtest"
	"opscopilot/pkg/connectiondiag"
)

func relayClient(t *testing.T, options sshtest.CommandOptions) (*Client, *sshtest.CommandServer) {
	t.Helper()
	server := sshtest.NewCommandServer(t, options)
	client, err := NewClient(server.Config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client, server
}

func TestCommandTunnelBinarySmallPacketsAndHalfClose(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	greeting := []byte{0, 255, 13, 10, 26, 128}
	go func() {
		peer, err := listener.Accept()
		if err != nil {
			return
		}
		defer peer.Close()
		_, _ = peer.Write(greeting)
		_, _ = io.Copy(peer, peer)
		_, _ = peer.Write([]byte("TAIL"))
	}()
	client, server := relayClient(t, sshtest.CommandOptions{Preamble: "fixture shell banner\n"})
	if _, err = client.SSHClient().Dial("tcp", listener.Addr().String()); err == nil {
		t.Fatal("fixture allowed standard forwarding")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := client.DialCommandContext(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	result := make(chan error, 1)
	go func() {
		first := make([]byte, len(greeting))
		if _, err := io.ReadFull(conn, first); err != nil {
			result <- err
			return
		}
		if !bytes.Equal(first, greeting) {
			result <- errors.New("startup frame corrupted binary greeting")
			return
		}
		if _, err := conn.Write([]byte{255}); err != nil {
			result <- err
			return
		}
		if _, err := io.ReadFull(conn, first[:1]); err != nil || first[0] != 255 {
			result <- errors.New("single-byte request stalled or changed")
			return
		}
		payload := bytes.Repeat([]byte{0, 255, 13, 10, 26, 128, 77}, 600000)
		written := make(chan error, 1)
		go func() { _, err := io.Copy(conn, bytes.NewReader(payload)); written <- err }()
		got := make([]byte, len(payload))
		if _, err := io.ReadFull(conn, got); err != nil {
			result <- err
			return
		}
		if err := <-written; err != nil {
			result <- err
			return
		}
		if !bytes.Equal(got, payload) {
			result <- errors.New("large binary payload changed")
			return
		}
		if err := conn.(interface{ CloseWrite() error }).CloseWrite(); err != nil {
			result <- err
			return
		}
		tail, err := io.ReadAll(conn)
		if err != nil {
			result <- err
			return
		}
		if string(tail) != "TAIL" {
			result <- errors.New("half-close lost trailing response")
			return
		}
		result <- nil
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		conn.Close()
		t.Fatal("byte relay timed out")
	}
	conn.Close()
	server.WaitIdle(t)
	if server.Forwards.Load() != 1 || server.Commands.Load() != 1 {
		t.Fatal("command relay unexpectedly retried or used forwarding")
	}
}

func TestCommandTunnelStartupFailuresAndCancellation(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		options  sshtest.CommandOptions
		expected string
		timeout  time.Duration
	}{
		{"refused", sshtest.CommandOptions{}, "ConnectionRefusedError", 3 * time.Second},
		{"missing-python", sshtest.CommandOptions{MissingPython: true}, "127", 3 * time.Second},
		{"no-ready", sshtest.CommandOptions{Stall: true}, "超时", 300 * time.Millisecond},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			client, server := relayClient(t, scenario.options)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			target := listener.Addr().String()
			listener.Close()
			ctx, cancel := context.WithTimeout(context.Background(), scenario.timeout)
			defer cancel()
			_, err = client.DialCommandContext(ctx, target)
			if err == nil || !strings.Contains(err.Error(), scenario.expected) {
				t.Fatalf("wrong startup result: %v", err)
			}
			server.WaitIdle(t)
		})
	}
}

func TestCommandTunnelCloseStopsIdleProcess(t *testing.T) {
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			defer conn.Close()
			_, _ = io.Copy(io.Discard, conn)
		}
	}()
	client, server := relayClient(t, sshtest.CommandOptions{})
	conn, err := client.DialCommandContext(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if server.Active.Load() != 1 {
		t.Fatal("no actual relay process")
	}
	conn.Close()
	conn.Close()
	server.WaitIdle(t)
}

func TestCommandTunnelPeerHalfCloseStillAcceptsClientWrites(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	got := make(chan string, 1)
	go func() {
		peer, err := listener.Accept()
		if err != nil {
			return
		}
		defer peer.Close()
		_, _ = peer.Write([]byte("GREETING"))
		_ = peer.(*net.TCPConn).CloseWrite()
		data, _ := io.ReadAll(peer)
		got <- string(data)
	}()
	client, server := relayClient(t, sshtest.CommandOptions{})
	conn, err := client.DialCommandContext(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	finished := make(chan error, 1)
	go func() {
		data, err := io.ReadAll(conn)
		if err != nil {
			finished <- err
			return
		}
		if string(data) != "GREETING" {
			finished <- errors.New("peer half-close lost output")
			return
		}
		_, err = conn.Write([]byte("AFTER_EOF"))
		if err == nil {
			err = conn.(interface{ CloseWrite() error }).CloseWrite()
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		conn.Close()
		t.Fatal("peer half-close stalled")
	}
	select {
	case data := <-got:
		if data != "AFTER_EOF" {
			t.Fatal("peer half-close discarded client writes")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("peer did not receive final client writes")
	}
	conn.Close()
	server.WaitIdle(t)
}

func TestAutomaticDatabaseTransport(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	t.Run("policy rejection automatically uses same target", func(t *testing.T) {
		client, server := relayClient(t, sshtest.CommandOptions{})
		ctx, trace := connectiondiag.New(context.Background())
		conn, err := client.DialDatabaseContext(ctx, listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if _, err = conn.Write([]byte("ping")); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 4)
		if _, err = io.ReadFull(conn, got); err != nil || string(got) != "ping" {
			t.Fatalf("relay: %q %v", got, err)
		}
		if server.Forwards.Load() != 1 || server.Commands.Load() != 1 || !trace.Has("ssh.database.auto.compatibility") || !trace.Has("ssh.database.command.ok") {
			t.Fatal("automatic fallback not confirmed")
		}
	})
	t.Run("connection failure does not execute commands", func(t *testing.T) {
		client, server := relayClient(t, sshtest.CommandOptions{ForwardFailure: ssh.ConnectionFailed})
		_, err := client.DialDatabaseContext(context.Background(), listener.Addr().String())
		var rejected *ssh.OpenChannelError
		if !errors.As(err, &rejected) || rejected.Reason != ssh.ConnectionFailed || server.Commands.Load() != 0 {
			t.Fatalf("unexpected fallback: %v", err)
		}
	})
	t.Run("standard success never starts command transport", func(t *testing.T) {
		config := diagnosticSSH(t, "forward", listener.Addr().String())
		client, err := NewClient(config)
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		ctx, trace := connectiondiag.New(context.Background())
		conn, err := client.DialDatabaseContext(ctx, listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		conn.Close()
		if !trace.Has("ssh.database.channel.ok") || trace.Has("ssh.command.setup") {
			t.Fatal("unnecessary command transport")
		}
	})
}

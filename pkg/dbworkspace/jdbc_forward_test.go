package dbworkspace

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJDBCForwardPreservesTargetFailure(t *testing.T) {
	for _, message := range []string{"ssh: rejected: administratively prohibited", "ssh: rejected: connect failed (Connection refused)"} {
		t.Run(message, func(t *testing.T) {
			cause := errors.New(message)
			targetSeen := make(chan string, 1)
			tunnel, err := jdbcForward(func(_ context.Context, target string) (net.Conn, error) {
				targetSeen <- target
				return nil, cause
			}, "127.0.0.1:1611")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tunnel.stop)
			client, err := net.DialTimeout("tcp", tunnel.address, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			_ = client.SetDeadline(time.Now().Add(2 * time.Second))
			_, _ = client.Write([]byte("handshake"))
			if _, err = client.Read(make([]byte, 1)); err == nil {
				t.Fatal("failed target did not close the local connection")
			}
			select {
			case target := <-targetSeen:
				if target != "127.0.0.1:1611" {
					t.Fatalf("unexpected target: %s", target)
				}
			case <-time.After(time.Second):
				t.Fatal("forwarder did not attempt the database target")
			}
			driverErr := errors.New("Failed to read data from socket: Connection reset")
			detail := jdbcOpenError(driverErr, tunnel)
			if !errors.Is(detail, cause) || !errors.Is(detail, driverErr) || !strings.Contains(detail.Error(), "SSH 隧道连接数据库目标失败") {
				t.Fatalf("lost the SSH or JDBC cause: %v", detail)
			}
		})
	}
}

func TestJDBCForwardFailureDiagnostics(t *testing.T) {
	s, driver := jdbcTestService(t)
	s.Gateway = func(context.Context, string, string) (DialContext, func() error, error) {
		return func(context.Context, string) (net.Conn, error) {
			return nil, errors.New("ssh: rejected: administratively prohibited")
		}, func() error { return nil }, nil
	}
	password := "forward-diagnostic-secret"
	_, err := s.Test(context.Background(), ProfileInput{Profile: Profile{
		Engine: "jdbc", DriverID: driver.ID, Host: "127.0.0.1", Port: 1611,
		User: "forward-diagnostic-reader", GatewayID: "test-owned-gateway",
	}, Password: &password})
	if err == nil {
		t.Fatal("rejected tunnel unexpectedly connected")
	}
	for _, expected := range []string{"SSH 隧道连接数据库目标失败", "administratively prohibited", "JDBC 连接失败", "SQLState"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("missing diagnostic %s: %v", expected, err)
		}
	}
	if strings.Contains(err.Error(), password) || strings.Contains(err.Error(), "forward-diagnostic-reader") {
		t.Fatal("diagnostic leaked connection credentials")
	}
}

func TestJDBCForwardRoundTripAndCleanup(t *testing.T) {
	peerDone := make(chan struct{})
	tunnel, err := jdbcForward(func(context.Context, string) (net.Conn, error) {
		channel, peer := net.Pipe()
		go func() { defer close(peerDone); defer peer.Close(); _, _ = io.Copy(peer, peer) }()
		return channel, nil
	}, "db.internal:1611")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tunnel.stop)
	client, err := net.DialTimeout("tcp", tunnel.address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = client.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 5)
	if _, err = io.ReadFull(client, response); err != nil || string(response) != "hello" {
		t.Fatalf("forwarded payload: %q, %v", response, err)
	}
	snapshot := tunnel.snapshot()
	deadline := time.Now().Add(time.Second)
	for (snapshot.Sent != 5 || snapshot.Received != 5) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		snapshot = tunnel.snapshot()
	}
	if snapshot.Accepted != 1 || snapshot.Opened != 1 || snapshot.Sent != 5 || snapshot.Received != 5 {
		t.Fatalf("missing forwarded connection or byte counts: %#v", snapshot)
	}
	driverErr := errors.New("database authentication failed")
	if detail := jdbcOpenError(driverErr, tunnel); !errors.Is(detail, driverErr) || strings.Contains(detail.Error(), "SSH 隧道连接数据库目标失败") {
		t.Fatalf("misreported database failure as an SSH failure: %v", detail)
	}
	tunnel.stop()
	tunnel.stop()
	select {
	case <-peerDone:
	case <-time.After(time.Second):
		t.Fatal("disconnect did not close the remote channel")
	}
	if _, err = client.Read(make([]byte, 1)); err == nil {
		t.Fatal("disconnect did not close the local connection")
	}
	if c, err := net.DialTimeout("tcp", tunnel.address, time.Second); err == nil {
		c.Close()
		t.Fatal("temporary port remained open")
	}
}

func TestJDBCForwardShutdownIsNotADialFailure(t *testing.T) {
	opened, finished := make(chan struct{}), make(chan struct{})
	tunnel, err := jdbcForward(func(ctx context.Context, _ string) (net.Conn, error) {
		close(opened)
		<-ctx.Done()
		defer close(finished)
		return nil, ctx.Err()
	}, "db.internal:1611")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tunnel.stop)
	client, err := net.DialTimeout("tcp", tunnel.address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	select {
	case <-opened:
	case <-time.After(time.Second):
		t.Fatal("target dial not started")
	}
	tunnel.stop()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel target dial")
	}
	// The read completes after the forwarder's deferred local close.
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = client.Read(make([]byte, 1))
	if err := tunnel.snapshot().DialFailure; err != nil {
		t.Fatalf("normal disconnect reported as a tunnel failure: %v", err)
	}
}

func TestJDBCForwardRemoteEOFIsNotDialFailure(t *testing.T) {
	tunnel, err := jdbcForward(func(context.Context, string) (net.Conn, error) {
		channel, peer := net.Pipe()
		peer.Close()
		return channel, nil
	}, "127.0.0.1:32063")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tunnel.stop)
	client, err := net.DialTimeout("tcp", tunnel.address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err = client.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected closed remote channel")
	}
	s := tunnel.snapshot()
	if s.Opened != 1 || s.FirstEnd != "目标 → JDBC" || s.FirstError != nil || s.DialFailure != nil {
		t.Fatalf("remote EOF misclassified: %#v", s)
	}
	detail := jdbcOpenError(errors.New("Connection reset"), tunnel).Error()
	for _, expected := range []string{"连接诊断 v2", "127.0.0.1:32063", "已通过 SSH 建立到目标的 TCP 通道", "目标 → JDBC 0 字节", "读到 EOF"} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("missing %s: %s", expected, detail)
		}
	}
}

func TestJDBCForwardWithoutLocalClientDoesNotClaimConnectivity(t *testing.T) {
	tunnel, err := jdbcForward(func(context.Context, string) (net.Conn, error) {
		t.Error("unexpected dial")
		return nil, errors.New("unexpected dial")
	}, "db.internal:32063")
	if err != nil {
		t.Fatal(err)
	}
	defer tunnel.stop()
	detail := jdbcOpenError(errors.New("driver rejected URL"), tunnel).Error()
	if !strings.Contains(detail, "尚未观察到 JDBC 接入本机转发端口") {
		t.Fatal(detail)
	}
	if detail := jdbcOpenError(errors.New("Connection reset"), nil).Error(); !strings.Contains(detail, "连接方式：直接连接") || strings.Contains(detail, "连接方式：SSH") {
		t.Fatal(detail)
	}
}

func TestJDBCConnectedThenResetDiagnostics(t *testing.T) {
	root := os.Getenv("OPSCOPILOT_TEST_JDBC_RUNTIME")
	if root == "" {
		t.Skip("set JDBC runtime path for real socket-reset diagnostics")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	classes := t.TempDir()
	if out, err := exec.CommandContext(ctx, "javac", "--release", "8", "-d", classes, filepath.Join("testdata", "SocketResetDriver.java")).CombinedOutput(); err != nil {
		t.Fatalf("compile fixture: %v\n%s", err, out)
	}
	jarPath := filepath.Join(t.TempDir(), "socket-reset-fixture.jar")
	if out, err := exec.CommandContext(ctx, "jar", "--create", "--file", jarPath, "-C", classes, ".").CombinedOutput(); err != nil {
		t.Fatalf("package fixture: %v\n%s", err, out)
	}
	server, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	peerDone := make(chan error, 1)
	go func() {
		conn, err := server.Accept()
		if err != nil {
			peerDone <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err = io.ReadFull(conn, make([]byte, 5)); err != nil {
			peerDone <- err
			return
		}
		peerDone <- conn.(*net.TCPConn).SetLinger(0)
	}()
	s := New(filepath.Join(t.TempDir(), "profiles.json"), &memorySecrets{values: map[string]string{}})
	s.JDBCRuntimeDir = root
	t.Cleanup(s.Close)
	d, err := s.ImportJDBC(ctx, JDBCImport{Name: "Reset probe", ClassName: "fixtures.SocketResetDriver", URLTemplate: "jdbc:reset-probe://{host}:{port}", Dialect: "generic", Paths: []string{jarPath}})
	if err != nil {
		t.Fatal(err)
	}
	s.Gateway = func(context.Context, string, string) (DialContext, func() error, error) {
		return func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", server.Addr().String())
		}, func() error { return nil }, nil
	}
	password := "socket-reset-diagnostic-secret"
	_, err = s.Test(ctx, ProfileInput{Profile: Profile{Engine: "jdbc", DriverID: d.ID, Host: "127.0.0.1", Port: 32063, User: "socket-reset-reader", GatewayID: "owned-test-gateway"}, Password: &password})
	if err == nil {
		t.Fatal("reset peer unexpectedly connected")
	}
	detail := err.Error()
	for _, expected := range []string{"连接诊断 v2", "127.0.0.1:32063", "已通过 SSH 建立到目标的 TCP 通道", "fixtures.SocketResetDriver", "SQLState 08001", "目标 → JDBC", "转发读写错误", "Failed to read data from socket"} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("missing %s in real Java reset diagnostic: %s", expected, detail)
		}
	}
	if strings.Contains(detail, "SSH 隧道连接数据库目标失败") || strings.Contains(detail, password) || strings.Contains(detail, "socket-reset-reader") || strings.Contains(detail, "HELLO") {
		t.Fatalf("misclassification or sensitive payload in diagnostic: %s", detail)
	}
	select {
	case peerErr := <-peerDone:
		if peerErr != nil {
			t.Fatal(peerErr)
		}
	case <-time.After(time.Second):
		t.Fatal("test peer did not finish")
	}
}

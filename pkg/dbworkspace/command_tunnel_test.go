package dbworkspace

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"opscopilot/internal/sshtest"
	"opscopilot/pkg/connectiondiag"
	"opscopilot/pkg/sshclient"
)

func TestGatewayModeValidationAndPersistence(t *testing.T) {
	s := newNativeTestService(t.TempDir()+"/profiles.json", &memorySecrets{values: map[string]string{}})
	p, err := s.Save(ProfileInput{Profile: Profile{Engine: "mysql", Host: "127.0.0.1", User: "fixture", GatewayID: "final55", GatewayMode: GatewayModeCommand}})
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := s.List()
	if err != nil || len(profiles) != 1 || profiles[0].GatewayMode != GatewayModeCommand {
		t.Fatal("command mode was not persisted")
	}
	p.GatewayID = ""
	if _, err = s.Save(ProfileInput{Profile: p}); err == nil {
		t.Fatal("command mode allowed without SSH")
	}
	p.GatewayID = "final55"
	p.GatewayMode = "automatic-fallback"
	if _, err = s.Save(ProfileInput{Profile: p}); err == nil {
		t.Fatal("unknown gateway mode allowed")
	}
}

func TestJDBCCommandTunnelIntegration(t *testing.T) {
	for _, mode := range []string{GatewayModeCommand, GatewayModeAuto} {
		t.Run(mode, func(t *testing.T) { testJDBCCommandTunnel(t, mode) })
	}
}
func testJDBCCommandTunnel(t *testing.T, gatewayMode string) {
	port, err := strconv.Atoi(os.Getenv("OPSCOPILOT_TEST_MYSQL_PORT"))
	if err != nil {
		t.Skip("isolated MySQL endpoint required")
	}
	s, driver := jdbcTestService(t)
	server := sshtest.NewCommandServer(t, sshtest.CommandOptions{})
	s.Gateway = func(ctx context.Context, _, mode string) (DialContext, func() error, error) {
		client, err := sshclient.NewClientWithTrace(ctx, server.Config)
		if err != nil {
			return nil, nil, err
		}
		return func(dialCtx context.Context, target string) (net.Conn, error) {
			if mode == GatewayModeCommand {
				conn, err := client.DialCommandContext(connectiondiag.WithTrace(dialCtx, connectiondiag.From(ctx)), target)
				if err == nil {
					connectiondiag.Event(ctx, "ssh.database.command.ok", "fixture remote TCP confirmed")
				}
				return conn, err
			}
			if mode == GatewayModeAuto {
				return client.DialDatabaseContext(connectiondiag.WithTrace(dialCtx, connectiondiag.From(ctx)), target)
			}
			return client.SSHClient().DialContext(dialCtx, "tcp", target)
		}, client.Close, nil
	}
	password := os.Getenv("OPSCOPILOT_TEST_MYSQL_PASSWORD")
	p, err := s.Save(ProfileInput{Profile: Profile{Engine: "jdbc", DriverID: driver.ID, Host: "127.0.0.1", Port: port, User: "root", GatewayID: "final55", GatewayMode: gatewayMode, JDBCAllowPublicKeyRetrieval: true}, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	version, err := s.Test(context.Background(), ProfileInput{Profile: p})
	if err != nil || !strings.Contains(version, "MySQL") {
		t.Fatalf("real command/JDBC connection test failed: %s %v", version, err)
	}
	server.WaitIdle(t)
	info, err := s.Connect(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	run := func(sql string) QueryResult {
		t.Helper()
		out, err := s.Run(QueryRequest{SessionID: info.ID, RequestID: uuid.NewString(), SQL: sql, Limit: 100, TimeoutSeconds: 10})
		if err != nil || out.Error != "" {
			t.Fatalf("command/JDBC query failed: %v %s", err, out.Error)
		}
		return out
	}
	schema := "ops_command_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	run("CREATE DATABASE " + quoteIdentifier(schema))
	defer func() {
		db, closeDB, err := (MySQL{}).Open(context.Background(), Profile{Host: "127.0.0.1", Port: port, User: "root"}, password, nil)
		if err != nil {
			t.Error("cleanup connection failed")
			return
		}
		defer closeDB()
		defer db.Close()
		_, err = db.Exec("DROP DATABASE IF EXISTS " + quoteIdentifier(schema))
		if err != nil {
			t.Errorf("cleanup owned schema: %v", err)
		}
	}()
	run("CREATE TABLE " + qualified(schema, "items") + " (id BIGINT PRIMARY KEY, note TEXT); INSERT INTO " + qualified(schema, "items") + " VALUES (9007199254740993,'中文; 🦉')")
	result := run("SELECT id,note,REPEAT('界',80000),NULL FROM " + qualified(schema, "items"))
	row := result.Sets[0].Rows[0]
	if *row[0] != "9007199254740993" || *row[1] != "中文; 🦉" || len([]rune(*row[2])) != 80000 || row[3] != nil {
		t.Fatal("JDBC command channel corrupted result values")
	}
	tables, err := s.Tables(info.ID, schema)
	if err != nil || len(tables) != 1 || tables[0].Name != "items" {
		t.Fatalf("command metadata: %v %v", tables, err)
	}
	structure, err := s.Structure(TableRequest{SessionID: info.ID, Database: schema, Table: "items"})
	if err != nil || len(structure.Columns.Rows) != 2 {
		t.Fatalf("command structure: %v", err)
	}
	requestID := uuid.NewString()
	done := make(chan QueryResult, 1)
	go func() {
		out, _ := s.Run(QueryRequest{SessionID: info.ID, RequestID: requestID, SQL: "SELECT SLEEP(5)", TimeoutSeconds: 10})
		done <- out
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !s.Cancel(requestID) {
		if time.Now().After(deadline) {
			t.Fatal("command query cancellation not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case out := <-done:
		if !out.Cancelled || !out.Disconnected {
			t.Fatalf("cancellation did not close command channel: %+v", out)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("command cancellation stalled")
	}
	server.WaitIdle(t)
	info, err = s.Connect(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	run("SELECT 1")
	_ = s.Disconnect(info.ID)
	server.WaitIdle(t)
	if gatewayMode == GatewayModeAuto && server.Forwards.Load() == 0 {
		t.Fatal("automatic mode did not try standard forwarding")
	}
	if gatewayMode == GatewayModeCommand && server.Forwards.Load() != 0 {
		t.Fatal("explicit command mode attempted standard forwarding")
	}
	p.GatewayMode = GatewayModeForward
	_, err = s.Test(context.Background(), ProfileInput{Profile: p})
	if err == nil || !strings.Contains(err.Error(), "administratively prohibited") {
		t.Fatal(fmt.Sprintf("fixture did not reproduce forbidden forwarding: %v", err))
	}
	server.WaitIdle(t)
}

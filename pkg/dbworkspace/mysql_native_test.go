package dbworkspace

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"net"
	"opscopilot/pkg/secretstore"
	"time"
)

// Native transport is test-only; production connections require imported JDBC.
func newNativeTestService(path string, secrets secretstore.SecretStore) *Service {
	s := New(path, secrets)
	s.connectors["mysql"] = MySQL{}
	return s
}

func (MySQL) Open(ctx context.Context, p Profile, password string, dial DialContext) (*sql.DB, func(), error) {
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd = p.User, password
	cfg.Net, cfg.Addr, cfg.DBName = "tcp", net.JoinHostPort(p.Host, fmt.Sprint(p.Port)), p.Database
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 10*time.Second, 125*time.Second, 10*time.Second
	cfg.MaxAllowedPacket = 16 << 20
	cfg.Collation = "utf8mb4_general_ci"
	if p.TLS == "required" {
		cfg.TLSConfig = "true"
	}
	cleanup := func() {}
	if dial != nil {
		// SSH forwarded channels do not implement socket deadlines. The driver
		// watches each operation's context and closes the channel on cancellation;
		// keep service-level deadlines instead of calling unsupported channel APIs.
		cfg.ReadTimeout, cfg.WriteTimeout = 0, 0
		cfg.Net = "opsdb-" + uuid.NewString()
		mysql.RegisterDialContext(cfg.Net, mysql.DialContextFunc(dial))
		cleanup = func() { mysql.DeregisterDialContext(cfg.Net) }
	}
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("MySQL 连接配置无效")
	}
	db := sql.OpenDB(connector)
	// One pinned query connection plus a bounded control connection for KILL QUERY.
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		cleanup()
		return nil, nil, fmt.Errorf("MySQL 连接失败: %w", err)
	}
	return db, cleanup, nil
}

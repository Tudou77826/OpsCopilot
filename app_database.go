package main

import (
	"context"
	"fmt"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"net"
	"path/filepath"
	"strings"
	"time"

	"opscopilot/pkg/connectiondiag"
	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/dbworkspace"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/sshclient"
)

func (a *App) databaseService() *dbworkspace.Service {
	a.databaseMu.Lock()
	defer a.databaseMu.Unlock()
	if a.database == nil {
		dir := "."
		if a.configMgr != nil {
			dir = a.configMgr.Directory()
		}
		secrets := a.secretStore
		if secrets == nil {
			secrets = secretstore.NewKeyringStore()
		}
		a.database = dbworkspace.New(filepath.Join(dir, "database-connections.json"), secrets)
		a.database.Gateway = a.openDatabaseGateway
		a.database.BuildVersion = Version
		a.database.ResolveJDBCRuntime = resolveDesktopJDBCRuntime
	}
	return a.database
}

func (a *App) DatabaseProfiles() ([]dbworkspace.Profile, error) { return a.databaseService().List() }
func (a *App) DatabaseGroups() ([]string, error)                { return a.databaseService().Groups() }
func (a *App) DatabaseCreateGroup(name string) error            { return a.databaseService().CreateGroup(name) }
func (a *App) DatabaseRenameGroup(oldName, newName string) error {
	return a.databaseService().RenameGroup(oldName, newName)
}
func (a *App) DatabaseDeleteGroup(name string) error { return a.databaseService().DeleteGroup(name) }
func (a *App) DatabaseSave(in dbworkspace.ProfileInput) (dbworkspace.Profile, error) {
	return a.databaseService().Save(in)
}
func (a *App) DatabaseTest(in dbworkspace.ProfileInput) (string, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.databaseService().Test(ctx, in)
}
func (a *App) DatabaseDelete(id string, revision int) error {
	return a.databaseService().Delete(id, revision)
}
func (a *App) DatabaseConnect(id string) (dbworkspace.SessionInfo, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.databaseService().Connect(ctx, id)
}
func (a *App) DatabaseDisconnect(id string) error          { return a.databaseService().Disconnect(id) }
func (a *App) DatabaseSchemas(id string) ([]string, error) { return a.databaseService().Schemas(id) }
func (a *App) DatabaseTables(id, schema string) ([]dbworkspace.Table, error) {
	return a.databaseService().Tables(id, schema)
}
func (a *App) DatabaseStructure(req dbworkspace.TableRequest) (dbworkspace.Structure, error) {
	return a.databaseService().Structure(req)
}
func (a *App) DatabaseGenerateSQL(req dbworkspace.SQLTemplateRequest) (string, error) {
	return a.databaseService().GenerateSQL(req)
}
func (a *App) DatabaseBrowse(req dbworkspace.TableRequest) (dbworkspace.QueryResult, error) {
	return a.databaseService().Browse(req)
}
func (a *App) DatabaseQuery(req dbworkspace.QueryRequest) (dbworkspace.QueryResult, error) {
	return a.databaseService().Run(req)
}
func (a *App) DatabaseCancel(id string) bool { return a.databaseService().Cancel(id) }

func (a *App) DatabaseJDBCStatus() (dbworkspace.JDBCStatus, error) {
	return a.databaseService().JDBCStatus()
}
func (a *App) DatabaseImportJDBC(in dbworkspace.JDBCImport) (dbworkspace.JDBCDriver, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return a.databaseService().ImportJDBC(ctx, in)
}
func (a *App) DatabaseDeleteJDBC(id string) error { return a.databaseService().DeleteJDBC(id) }
func (a *App) DatabaseChooseJDBCJars() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择厂商 JDBC 驱动及依赖 JAR", Filters: []runtime.FileFilter{{DisplayName: "Java 驱动 (*.jar)", Pattern: "*.jar"}}})
}

type DatabaseGateway struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
}

func (a *App) DatabaseGateways() []DatabaseGateway {
	out := []DatabaseGateway{}
	if a.savedSessionMgr == nil {
		return out
	}
	var walk func([]*connectionstore.Node)
	walk = func(nodes []*connectionstore.Node) {
		for _, n := range nodes {
			if n.Config != nil && n.Config.NormalizedProtocol() == remote.ProtocolSSH {
				out = append(out, DatabaseGateway{n.ID, n.Name, n.Config.Host})
			}
			walk(n.Children)
		}
	}
	walk(a.savedSessionMgr.Snapshot())
	return out
}

func (a *App) openDatabaseGateway(ctx context.Context, id, mode string) (dbworkspace.DialContext, func() error, error) {
	if mode != "" && mode != dbworkspace.GatewayModeAuto && mode != dbworkspace.GatewayModeForward && mode != dbworkspace.GatewayModeCommand {
		return nil, nil, fmt.Errorf("不支持的 SSH 通道模式")
	}
	if a.savedSessionMgr == nil {
		return nil, nil, fmt.Errorf("没有可用的 SSH 连接")
	}
	var cfg *remote.ConnectConfig
	var walk func([]*connectionstore.Node)
	walk = func(nodes []*connectionstore.Node) {
		for _, n := range nodes {
			if n.ID == id {
				cfg = n.Config
			}
			walk(n.Children)
		}
	}
	walk(a.savedSessionMgr.Snapshot())
	if cfg == nil || cfg.NormalizedProtocol() != remote.ProtocolSSH {
		return nil, nil, fmt.Errorf("关联的 SSH 连接不存在，请编辑数据库连接")
	}
	// Register every hop's credentials before emitting any route/error evidence.
	var registerSecrets func(*remote.ConnectConfig)
	registerSecrets = func(c *remote.ConnectConfig) {
		if c != nil {
			connectiondiag.AddSecrets(ctx, c.Password, c.RootPassword)
			registerSecrets(c.Bastion)
		}
	}
	registerSecrets(cfg)
	var credentials func(*remote.ConnectConfig) error
	credentials = func(c *remote.ConnectConfig) error {
		if c == nil {
			return nil
		}
		if c.Password == "" && a.secretStore != nil {
			c.Password, _ = a.secretStore.Get("OpsCopilot-SSH", c.Host+":"+c.User)
			if c.Password == "" {
				c.Password, _ = a.secretStore.Get("opscopilot", c.Host+"_"+c.User)
			}
		}
		if c.Password == "" {
			return fmt.Errorf("SSH 凭据不可用，请先在会话管理中连接该节点")
		}
		connectiondiag.AddSecrets(ctx, c.Password, c.RootPassword)
		return credentials(c.Bastion)
	}
	if err := credentials(cfg); err != nil {
		return nil, nil, err
	}
	var route []string
	for node := cfg; node != nil; node = node.Bastion {
		route = append([]string{net.JoinHostPort(node.Host, fmt.Sprint(node.Port))}, route...)
	}
	connectiondiag.Event(ctx, "ssh.route", "实际配置路径 本机 → %s；SSH账号见各节点事件；后台重新认证，不复用已打开终端或 su/sudo 身份", strings.Join(route, " → "))
	connectiondiag.Fact(ctx, "SSH实际路径", "本机 → "+strings.Join(route, " → "))
	client, err := sshclient.NewClientWithTrace(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	if err = ctx.Err(); err != nil {
		client.Close()
		return nil, nil, err
	}
	return func(dialCtx context.Context, addr string) (net.Conn, error) {
		started := time.Now()
		if mode == dbworkspace.GatewayModeCommand {
			connectiondiag.Event(ctx, "ssh.database.command.start", "最终SSH=%s SSH账号=%q 数据库目标=%s 类型=exec/python3；使用显式选择的兼容模式，不尝试 direct-tcpip", net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port)), cfg.User, addr)
			commandCtx := connectiondiag.WithTrace(dialCtx, connectiondiag.From(ctx))
			conn, err := client.DialCommandContext(commandCtx, addr)
			if err != nil {
				connectiondiag.Event(ctx, "ssh.database.command.failed", "%v", err)
			} else {
				connectiondiag.Event(ctx, "ssh.database.command.ok", "数据库目标=%s 远端TCP已确认 耗时=%dms", addr, time.Since(started).Milliseconds())
			}
			return conn, err
		}
		return client.DialDatabaseContext(connectiondiag.WithTrace(dialCtx, connectiondiag.From(ctx)), addr)
	}, client.Close, nil
}

func (a *App) shutdownDatabase(context.Context) {
	a.databaseMu.Lock()
	svc := a.database
	a.databaseMu.Unlock()
	if svc != nil {
		svc.Close()
	}
}

package dbworkspace

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"opscopilot/pkg/connectiondiag"
)

// JDBC implements database/sql's connection boundary so the workspace retains
// its existing session gates, result limits and cancellation policy.
type JDBC struct {
	service    *Service
	definition JDBCDriver
}

func (JDBC) Disconnected(c *sql.Conn) bool {
	closed := true
	_ = c.Raw(func(conn any) error {
		select {
		case <-conn.(*jdbcConn).process.done:
		default:
			closed = false
		}
		return nil
	})
	return closed
}

func (j JDBC) Open(ctx context.Context, p Profile, password string, dial DialContext) (*sql.DB, func(), error) {
	connectiondiag.Event(ctx, "jdbc.driver.config", "驱动类=%s 版本=%s 方言=%s URL模板=%s", j.definition.ClassName, j.definition.Version, j.definition.Dialect, j.definition.URLTemplate)
	connectiondiag.Fact(ctx, "JDBC驱动类", j.definition.ClassName)
	connectiondiag.Fact(ctx, "JDBC驱动版本", j.definition.Version)
	connectiondiag.Fact(ctx, "JDBC方言", j.definition.Dialect)
	for _, jar := range j.definition.Jars {
		connectiondiag.Event(ctx, "jdbc.jar", "文件=%s SHA256=%s", jar.Name, jar.SHA256)
	}
	connectiondiag.Event(ctx, "jdbc.target.original", "数据库端点=%s", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))
	cleanup := func() {}
	var tunnel *jdbcForwarder
	if dial != nil {
		if p.TLS == "required" {
			return nil, nil, fmt.Errorf("JDBC 经 SSH 的 TLS 主机名验证尚未支持，请使用直连 TLS 或受信任 SSH 通道")
		}
		forwarder, err := jdbcForwardWithTrace(ctx, dial, net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))
		if err != nil {
			return nil, nil, err
		}
		tunnel = forwarder
		cleanup = forwarder.stop
		p.Host, _, _ = net.SplitHostPort(forwarder.address)
		_, port, _ := net.SplitHostPort(forwarder.address)
		p.Port, _ = strconv.Atoi(port)
		connectiondiag.Event(ctx, "jdbc.loopback.ready", "JDBC实际接入=%s → SSH数据库目标=%s；本机监听成功不代表远端可达", forwarder.address, forwarder.target)
	}
	process, err := j.service.startJDBC(ctx, j.definition)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	if err = process.load(ctx, j.service.driverDir(), j.definition, nil); err != nil {
		process.close()
		cleanup()
		return nil, nil, err
	}
	connectiondiag.Event(ctx, "jdbc.driver.loaded", "驱动 JAR 校验、加载及 URL 模板检查完成")
	connectiondiag.Event(ctx, "jdbc.url.effective", "实际JDBC URL=%s；SSH时已替换为本机回环端口。不会记录数据库账号或密码。", jdbcURL(j.definition, p))
	connectiondiag.Fact(ctx, "JDBC实际URL", jdbcURL(j.definition, p))
	connectiondiag.Event(ctx, "jdbc.properties", "DriverManager登录超时=10s TLS配置=%s allowPublicKeyRetrieval=%t；generic方言仅传user/password，不将TLS或只读配置转换为厂商属性", p.TLS, p.JDBCAllowPublicKeyRetrieval)
	properties := "仅传 user/password（值不记录）；其他厂商属性由驱动默认值决定"
	if j.definition.Dialect == "mysql" {
		properties = "user/password（值不记录）；固定超时、安全与TLS属性，具体值见时间线"
	}
	connectiondiag.Fact(ctx, "JDBC连接属性策略", properties)
	if j.definition.Dialect == "mysql" {
		connectiondiag.Event(ctx, "jdbc.properties.mysql", "connectTimeout=10000 socketTimeout=125000 allowMultiQueries=false allowLoadLocalInfile=false allowUrlInLocalInfile=false sslMode按TLS配置设置；此处仅记录属性，不发送额外请求")
	}
	if err = process.call(ctx, map[string]any{"op": "open", "url": jdbcURL(j.definition, p), "user": p.User, "password": password, "tls": p.TLS, "allowPublicKeyRetrieval": strconv.FormatBool(p.JDBCAllowPublicKeyRetrieval)}, nil); err != nil {
		// Snapshot before our own cleanup closes sockets, otherwise an intentional
		// shutdown can be mistaken for the original connection failure.
		detail := jdbcOpenError(fmt.Errorf("驱动 %s v%s: %w", j.definition.ClassName, j.definition.Version, err), tunnel)
		connectiondiag.Event(ctx, "jdbc.open.failed", "%v", detail)
		cleanup()
		process.close()
		return nil, nil, detail
	}
	connectiondiag.Event(ctx, "jdbc.open.ok", "Java 建立数据库连接并完成首次版本读取")
	c := &jdbcConn{process: process}
	db := sql.OpenDB(&jdbcSQLConnector{conn: c})
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, func() { process.close(); cleanup() }, nil
}

type jdbcSQLConnector struct {
	conn *jdbcConn
	mu   sync.Mutex
	used bool
}

func (c *jdbcSQLConnector) Connect(context.Context) (driver.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.used {
		return nil, fmt.Errorf("JDBC 连接已关闭，请重新连接")
	}
	c.used = true
	return c.conn, nil
}
func (c *jdbcSQLConnector) Driver() driver.Driver { return jdbcSQLDriver{} }

type jdbcSQLDriver struct{}

func (jdbcSQLDriver) Open(string) (driver.Conn, error) {
	return nil, fmt.Errorf("JDBC 必须通过连接器打开")
}

type jdbcConn struct{ process *jdbcProcess }

func (c *jdbcConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *jdbcConn) Close() error                        { c.process.close(); return nil }
func (c *jdbcConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *jdbcConn) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	if o.Isolation != 0 {
		return nil, fmt.Errorf("暂不支持自定义事务隔离级别")
	}
	if err := c.process.call(ctx, map[string]any{"op": "begin", "readOnly": o.ReadOnly}, nil); err != nil {
		return nil, err
	}
	return jdbcTx{c}, nil
}

type jdbcTx struct{ c *jdbcConn }

func (t jdbcTx) Commit() error   { return t.finish("commit") }
func (t jdbcTx) Rollback() error { return t.finish("rollback") }
func (t jdbcTx) finish(op string) error {
	ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	return t.c.process.call(ctx, map[string]any{"op": op}, nil)
}
func (c *jdbcConn) query(ctx context.Context, query string, args []driver.NamedValue) (ResultSet, error) {
	values := make([]any, len(args))
	for i, a := range args {
		values[i] = a.Value
	}
	timeout := 120
	if deadline, ok := ctx.Deadline(); ok {
		timeout = int(time.Until(deadline).Seconds()) + 1
	}
	limit := 10000
	if n, ok := ctx.Value(jdbcRowLimitKey{}).(int); ok {
		limit = n + 1
	}
	var out ResultSet
	err := c.process.call(ctx, map[string]any{"op": "query", "sql": query, "args": values, "timeout": timeout, "limit": limit}, &out)
	return out, err
}
func (c *jdbcConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	r, err := c.query(ctx, q, args)
	if err != nil {
		return nil, err
	}
	return &jdbcRows{result: r}, nil
}
func (c *jdbcConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	r, err := c.query(ctx, q, args)
	return driver.RowsAffected(r.AffectedRows), err
}

type jdbcRows struct {
	result ResultSet
	index  int
}

func (r *jdbcRows) Columns() []string {
	out := make([]string, len(r.result.Columns))
	for i, c := range r.result.Columns {
		out[i] = c.Name
	}
	return out
}
func (r *jdbcRows) ColumnTypeDatabaseTypeName(i int) string { return r.result.Columns[i].Type }
func (r *jdbcRows) Close() error                            { return nil }
func (r *jdbcRows) Next(out []driver.Value) error {
	if r.index >= len(r.result.Rows) {
		if r.result.Truncated {
			return fmt.Errorf("JDBC 结果达到大小上限，请缩小查询范围")
		}
		return io.EOF
	}
	for i, v := range r.result.Rows[r.index] {
		if v == nil {
			out[i] = nil
		} else {
			out[i] = *v
		}
	}
	r.index++
	return nil
}

type jdbcRowLimitKey struct{}

func jdbcCall(ctx context.Context, c *sql.Conn, request map[string]any, out any) error {
	return c.Raw(func(conn any) error { return conn.(*jdbcConn).process.call(ctx, request, out) })
}
func (j JDBC) Version(ctx context.Context, c *sql.Conn) (string, error) {
	var out struct {
		Version string `json:"version"`
	}
	err := jdbcCall(ctx, c, map[string]any{"op": "version"}, &out)
	return out.Version, err
}
func (j JDBC) SelectDatabase(ctx context.Context, c *sql.Conn, database string) error {
	return jdbcCall(ctx, c, map[string]any{"op": "database", "database": database}, nil)
}
func (j JDBC) Schemas(ctx context.Context, c *sql.Conn) ([]string, error) {
	var r ResultSet
	err := jdbcCall(ctx, c, map[string]any{"op": "schemas"}, &r)
	if r.Truncated {
		return nil, fmt.Errorf("数据库列表过大")
	}
	out := []string{}
	for _, row := range r.Rows {
		if len(row) > 0 && row[0] != nil {
			out = append(out, *row[0])
		}
	}
	return out, err
}
func (j JDBC) Tables(ctx context.Context, c *sql.Conn, schema string) ([]Table, error) {
	var r ResultSet
	err := jdbcCall(ctx, c, map[string]any{"op": "tables", "database": schema}, &r)
	if r.Truncated {
		return nil, fmt.Errorf("表列表过大")
	}
	out := []Table{}
	for _, row := range r.Rows {
		t := Table{}
		for i, col := range r.Columns {
			if i >= len(row) || row[i] == nil {
				continue
			}
			switch strings.ToUpper(col.Name) {
			case "TABLE_NAME":
				t.Name = *row[i]
			case "TABLE_TYPE":
				t.Type = *row[i]
			case "REMARKS":
				t.Comment = *row[i]
			}
		}
		out = append(out, t)
	}
	return out, err
}
func (j JDBC) Structure(ctx context.Context, c *sql.Conn, schema, table string) (Structure, error) {
	if j.definition.Dialect == "mysql" {
		return (MySQL{}).Structure(ctx, c, schema, table)
	}
	var out Structure
	err := jdbcCall(ctx, c, map[string]any{"op": "columns", "database": schema, "table": table}, &out.Columns)
	if err == nil {
		err = jdbcCall(ctx, c, map[string]any{"op": "indexes", "database": schema, "table": table}, &out.Indexes)
	}
	out.Columns = jdbcProject(out.Columns, []string{"COLUMN_NAME", "TYPE_NAME", "IS_NULLABLE", "COLUMN_DEF", "", "IS_AUTOINCREMENT", "REMARKS"}, []string{"字段", "类型", "可空", "默认值", "键", "自增", "注释"})
	out.Indexes = jdbcProject(out.Indexes, []string{"INDEX_NAME", "COLUMN_NAME", "ORDINAL_POSITION", "NON_UNIQUE", "TYPE"}, []string{"索引", "字段", "顺序", "非唯一", "类型"})
	out.DDL = "此驱动尚未提供 DDL 方言适配"
	return out, err
}

// Keep the frontend's column contract independent of DatabaseMetaData's vendor columns/order.
func jdbcProject(source ResultSet, keys, labels []string) ResultSet {
	positions := map[string]int{}
	for i, col := range source.Columns {
		positions[strings.ToUpper(col.Name)] = i
	}
	out := ResultSet{Columns: []Column{}, Rows: [][]*string{}, Truncated: source.Truncated}
	for _, label := range labels {
		out.Columns = append(out.Columns, Column{Name: label, Type: "TEXT"})
	}
	for _, row := range source.Rows {
		next := make([]*string, len(keys))
		for i, key := range keys {
			if index, ok := positions[key]; ok && index < len(row) {
				next[i] = row[index]
			}
		}
		out.Rows = append(out.Rows, next)
	}
	return out
}
func (j JDBC) BrowseSQL(schema, table string, offset, limit int) string {
	if j.definition.Dialect == "mysql" {
		return (MySQL{}).BrowseSQL(schema, table, offset, limit)
	}
	return fmt.Sprintf("SELECT * FROM %s.%s OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", jdbcQuote(schema), jdbcQuote(table), offset, limit+1)
}
func jdbcQuote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func (j JDBC) GenerateSQL(ctx context.Context, c *sql.Conn, r SQLTemplateRequest) (string, error) {
	if j.definition.Dialect == "mysql" {
		return (MySQL{}).GenerateSQL(ctx, c, r)
	}
	if r.Kind == "name" {
		if r.Column != "" {
			return jdbcQuote(r.Column), nil
		}
		return jdbcQuote(r.Table), nil
	}
	name := jdbcQuote(r.Database) + "." + jdbcQuote(r.Table)
	if r.Kind == "qualified-name" {
		return name, nil
	}
	if r.Kind == "select" {
		return "SELECT * FROM " + name + ";", nil
	}
	return "", fmt.Errorf("此驱动尚未适配写入 SQL 模板")
}

// jdbcOpenError retains both the SSH channel failure and the JDBC driver cause.
// A local socket reset alone does not establish that the database was reached.
func jdbcOpenError(err error, tunnel *jdbcForwarder) error {
	if tunnel == nil {
		return fmt.Errorf("JDBC 连接失败 [连接诊断 v2]\n连接方式：直接连接\n%w", err)
	}
	s := tunnel.snapshot()
	stage := "尚未观察到 JDBC 接入本机转发端口"
	if s.Accepted > 0 {
		stage = "JDBC 已接入本机转发端口，尚未确认远端 TCP 通道建立"
	}
	if s.Opened > 0 {
		stage = "已通过 SSH 建立到目标的 TCP 通道；不代表数据库握手或认证成功"
	}
	detail := fmt.Sprintf("JDBC 连接失败 [连接诊断 v2]\n连接方式：SSH 隧道\nSSH 节点视角的数据库目标：%s\n通道状态：%s\n连接尝试：本机接入 %d 次，远端 TCP 建立 %d 次\n累计转发：JDBC → 目标 %d 字节；目标 → JDBC %d 字节",
		tunnel.target, stage, s.Accepted, s.Opened, s.Sent, s.Received)
	if s.FirstEnd != "" {
		detail += "\n首先结束的转发方向：" + s.FirstEnd
		if s.FirstError == nil {
			detail += "（读到 EOF）"
		} else {
			detail += "（读写异常）"
		}
	}
	jdbcErr := fmt.Errorf("%s\n%w", detail, err)
	if s.FirstError != nil {
		jdbcErr = errors.Join(fmt.Errorf("转发读写错误（%s）: %w", s.FirstEnd, s.FirstError), jdbcErr)
	}
	if s.DialFailure != nil {
		return errors.Join(fmt.Errorf("SSH 隧道连接数据库目标失败: %w", s.DialFailure), jdbcErr)
	}
	return jdbcErr
}

type jdbcForwarder struct {
	address  string
	target   string
	stop     func()
	snapshot func() jdbcForwardSnapshot
}

type jdbcForwardSnapshot struct {
	Accepted, Opened int
	Sent, Received   int64
	DialFailure      error
	FirstEnd         string
	FirstError       error
}

// Count only successful writes, never retain or log database wire contents.
type jdbcCountWriter struct {
	w io.Writer
	n *atomic.Int64
}

func (w jdbcCountWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	w.n.Add(int64(n))
	return n, err
}

// Only listens on loopback; owned channels are closed with the JDBC session.
func jdbcForward(dial DialContext, target string) (*jdbcForwarder, error) {
	return jdbcForwardWithTrace(context.Background(), dial, target)
}

func jdbcForwardWithTrace(traceCtx context.Context, dial DialContext, target string) (*jdbcForwarder, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	var state jdbcForwardSnapshot
	var sent, received atomic.Int64
	end := func(direction string, err error) {
		mu.Lock()
		defer mu.Unlock()
		if ctx.Err() == nil && state.FirstEnd == "" {
			state.FirstEnd, state.FirstError = direction, err
			connectiondiag.Event(traceCtx, "jdbc.forward.end", "首个结束方向=%s 错误=%v JDBC→目标=%d字节 目标→JDBC=%d字节；不记录载荷", direction, err, sent.Load(), received.Load())
		}
	}
	owned := map[net.Conn]bool{}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			listener.Close()
			mu.Lock()
			for conn := range owned {
				conn.Close()
			}
			mu.Unlock()
		})
	}
	go func() {
		for {
			local, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			if ctx.Err() != nil {
				mu.Unlock()
				local.Close()
				return
			}
			owned[local] = true
			state.Accepted++
			connectiondiag.Event(traceCtx, "jdbc.loopback.accepted", "本机接入次数=%d；开始请求远端通道", state.Accepted)
			mu.Unlock()
			go func() {
				defer func() { local.Close(); mu.Lock(); delete(owned, local); mu.Unlock() }()
				opening, done := context.WithTimeout(ctx, 10*time.Second)
				remote, err := dial(opening, target)
				done()
				if err != nil {
					mu.Lock()
					// Shutdown is intentional, not a new connection failure.
					if ctx.Err() == nil {
						state.DialFailure = err
						connectiondiag.Event(traceCtx, "jdbc.forward.dial.failed", "目标=%s 错误=%v；本机连接将关闭，驱动可能随后报告 reset/EOF", target, err)
					}
					mu.Unlock()
					return
				}
				defer remote.Close()
				mu.Lock()
				if ctx.Err() != nil {
					mu.Unlock()
					return
				}
				owned[remote] = true
				state.Opened++
				connectiondiag.Event(traceCtx, "jdbc.forward.dial.ok", "远端TCP建立次数=%d 目标=%s", state.Opened, target)
				mu.Unlock()
				defer func() { mu.Lock(); delete(owned, remote); mu.Unlock() }()
				go func() {
					_, err := io.Copy(jdbcCountWriter{local, &received}, remote)
					end("目标 → JDBC", err)
					local.Close()
				}()
				_, err = io.Copy(jdbcCountWriter{remote, &sent}, local)
				end("JDBC → 目标", err)
			}()
		}
	}()
	return &jdbcForwarder{address: listener.Addr().String(), target: target, stop: stop, snapshot: func() jdbcForwardSnapshot {
		mu.Lock()
		defer mu.Unlock()
		s := state
		s.Sent, s.Received = sent.Load(), received.Load()
		return s
	}}, nil
}

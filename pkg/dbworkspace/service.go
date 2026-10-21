package dbworkspace

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"opscopilot/pkg/connectiondiag"
	"opscopilot/pkg/secretstore"
)

type session struct {
	owner     string
	info      SessionInfo
	db        *sql.DB
	conn      *sql.Conn
	connector Connector
	gate      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	cleanup   func()
	once      sync.Once
	serverID  string
}

func (v *session) close() {
	v.once.Do(func() { v.cancel(); v.conn.Close(); v.db.Close(); v.cleanup() })
}

type Service struct {
	policyMu   sync.Mutex
	path       string
	secrets    secretstore.SecretStore
	connectors map[string]Connector
	Gateway    GatewayOpener
	mu         sync.Mutex
	sessions   map[string]*session
	requests   map[string]context.CancelFunc
	closed     bool
	opening    int
	// JDBCRuntimeDir is supplied by the host/test; production defaults beside the executable.
	JDBCRuntimeDir string
	BuildVersion   string
	jdbcProcesses  int
}

func New(path string, secrets secretstore.SecretStore) *Service {
	return &Service{path: path, secrets: secrets, connectors: map[string]Connector{}, sessions: map[string]*session{}, requests: map[string]context.CancelFunc{}}
}

func (s *Service) Connect(ctx context.Context, id string) (info SessionInfo, err error) {
	return s.ConnectRevision(ctx, id, 0)
}

// ConnectRevision fixes the approved saved configuration before dialing. A
// concurrent edit cannot redirect a confirmation to a different endpoint.
func (s *Service) ConnectRevision(ctx context.Context, id string, revision int) (info SessionInfo, err error) {
	if err := s.reserveOpening(); err != nil {
		return SessionInfo{}, err
	}
	defer s.releaseOpening()
	p, password, err := s.profile(id)
	if err != nil {
		return SessionInfo{}, err
	}
	if revision > 0 && p.Revision != revision {
		return SessionInfo{}, errors.New("数据库连接配置已改变，请重新确认后连接")
	}
	ctx, diagnostic := s.beginDiagnostic(ctx, "连接", p, password)
	defer func() { err = s.finishDiagnostic(ctx, diagnostic, err) }()
	adapter, err := s.adapter(p)
	if err != nil {
		return SessionInfo{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	db, release, err := s.open(ctx, adapter, p, password)
	if err != nil {
		return SessionInfo{}, err
	}
	c, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		release()
		return SessionInfo{}, err
	}
	info = SessionInfo{ID: uuid.NewString(), ProfileID: p.ID, Database: p.Database, ReadOnly: p.ReadOnly}
	connectiondiag.Event(ctx, "database.version.start", "开始读取数据库版本")
	if info.Version, err = connectorVersion(ctx, adapter, c); err != nil {
		c.Close()
		db.Close()
		release()
		return SessionInfo{}, err
	}
	connectiondiag.Event(ctx, "database.version.ok", "版本=%s", info.Version)
	lifetime, stop := context.WithCancel(context.Background())
	v := &session{info: info, db: db, conn: c, connector: adapter, gate: make(chan struct{}, 1), ctx: lifetime, cancel: stop, cleanup: release}
	if control, ok := adapter.(QueryCanceller); ok {
		connectiondiag.Event(ctx, "database.control-id.start", "开始读取用于查询取消的数据库会话标识；不记录标识值")
		v.serverID, err = control.ConnectionID(ctx, c)
		if err != nil {
			connectiondiag.Event(ctx, "database.control-id.failed", "%v", err)
			v.close()
			return SessionInfo{}, err
		}
		connectiondiag.Event(ctx, "database.control-id.ok", "数据库会话标识已读取")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		v.close()
		return SessionInfo{}, fmt.Errorf("数据库服务已关闭")
	}
	s.sessions[info.ID] = v
	return info, nil
}

// Test checks a draft using a short-lived connection, without saving credentials
// or registering a workspace session. A missing password preserves saved credentials.
func (s *Service) Test(ctx context.Context, in ProfileInput) (version string, err error) {
	if err := s.reserveOpening(); err != nil {
		return "", err
	}
	defer s.releaseOpening()
	p := in.Profile
	if err := validateProfile(&p); err != nil {
		return "", err
	}
	password := ""
	if in.Password != nil {
		if len(*in.Password) > 8192 {
			return "", fmt.Errorf("密码过长")
		}
		password = *in.Password
	} else if p.ID != "" {
		_, savedPassword, err := s.profile(p.ID)
		if err != nil {
			return "", err
		}
		password = savedPassword
	}
	ctx, diagnostic := s.beginDiagnostic(ctx, "测试连接", p, password)
	defer func() { err = s.finishDiagnostic(ctx, diagnostic, err) }()
	adapter, err := s.adapter(p)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	db, release, err := s.open(ctx, adapter, p, password)
	if err != nil {
		return "", err
	}
	defer release()
	defer db.Close()
	c, err := db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()
	connectiondiag.Event(ctx, "database.version.start", "开始读取数据库版本")
	version, err = connectorVersion(ctx, adapter, c)
	if err != nil {
		return "", fmt.Errorf("测试数据库连接失败: %w", err)
	}
	connectiondiag.Event(ctx, "database.version.ok", "版本=%s", version)
	return version, nil
}

func (s *Service) reserveOpening() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(s.sessions)+s.opening >= 16 {
		return fmt.Errorf("数据库连接数量已达上限或服务已关闭")
	}
	s.opening++
	return nil
}

func (s *Service) releaseOpening() {
	s.mu.Lock()
	s.opening--
	s.mu.Unlock()
}

func (s *Service) open(ctx context.Context, adapter Connector, p Profile, password string) (*sql.DB, func(), error) {
	var dial DialContext
	closeGateway := func() error { return nil }
	var err error
	if p.GatewayID != "" {
		connectiondiag.Event(ctx, "ssh.gateway.start", "开始打开所选 SSH 节点的独立连接")
		if s.Gateway == nil {
			return nil, nil, fmt.Errorf("当前宿主不支持 SSH 数据库通道")
		}
		dial, closeGateway, err = s.Gateway(ctx, p.GatewayID, p.GatewayMode)
		if err != nil {
			connectiondiag.Event(ctx, "ssh.gateway.failed", "%v", err)
			return nil, nil, err
		}
		connectiondiag.Event(ctx, "ssh.gateway.ok", "SSH 节点连接完成；尚未请求数据库 TCP 通道")
	} else {
		connectiondiag.Event(ctx, "database.direct", "直接从本机访问数据库目标，不使用 SSH")
	}
	connectiondiag.Event(ctx, "database.connector.start", "开始建立数据库连接")
	db, cleanup, err := adapter.Open(ctx, p, password, dial)
	if err != nil {
		closeGateway()
		return nil, nil, err
	}
	return db, func() { cleanup(); closeGateway() }, nil
}

func (s *Service) Disconnect(id string) error {
	return s.DisconnectContext(context.Background(), id)
}
func (s *Service) DisconnectContext(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	s.mu.Lock()
	v := s.sessions[id]
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	owner, _ := ctx.Value(ownershipKey{}).(string)
	if v != nil && v.owner != owner {
		s.mu.Unlock()
		return ErrOwned
	}
	delete(s.sessions, id)
	s.mu.Unlock()
	if v != nil {
		v.close()
	}
	return nil
}

// discardSession is internal cleanup of a broken physical connection, not an
// authorized user action. Cancellation must not leave it registered as usable.
func (s *Service) discardSession(id string, expected *session) {
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	s.mu.Lock()
	if s.sessions[id] != expected {
		s.mu.Unlock()
		return
	}
	delete(s.sessions, id)
	s.mu.Unlock()
	expected.close()
}

func (s *Service) disconnectProfile(id string) {
	s.mu.Lock()
	var found []*session
	for key, v := range s.sessions {
		if v.info.ProfileID == id {
			found = append(found, v)
			delete(s.sessions, key)
		}
	}
	s.mu.Unlock()
	for _, v := range found {
		v.close()
	}
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	all := s.sessions
	s.sessions = map[string]*session{}
	s.mu.Unlock()
	for _, v := range all {
		v.close()
	}
}

func (s *Service) acquire(id string) (*session, func(), error) {
	return s.acquireContext(context.Background(), id)
}
func (s *Service) acquireContext(ctx context.Context, id string) (*session, func(), error) {
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	v := s.sessions[id]
	if v == nil {
		return nil, nil, fmt.Errorf("数据库尚未连接，请重新连接")
	}
	owner, _ := ctx.Value(ownershipKey{}).(string)
	if v.owner != owner {
		return nil, nil, ErrOwned
	}
	select {
	case v.gate <- struct{}{}:
		return v, func() { <-v.gate }, nil
	default:
		return nil, nil, fmt.Errorf("当前连接正在执行操作，请等待或停止查询")
	}
}

func (s *Service) Schemas(id string) ([]string, error) {
	return s.SchemasContext(context.Background(), id)
}
func (s *Service) SchemasContext(parent context.Context, id string) ([]string, error) {
	v, release, err := s.acquireContext(parent, id)
	if err != nil {
		return nil, err
	}
	defer release()
	base, finish := sessionContext(parent, v)
	defer finish()
	ctx, cancel := context.WithTimeout(base, 15*time.Second)
	defer cancel()
	defer s.retireInterruptedSession(ctx, id, v)
	return v.connector.Schemas(ctx, v.conn)
}
func (s *Service) Tables(id, schema string) ([]Table, error) {
	return s.TablesContext(context.Background(), id, schema)
}
func (s *Service) TablesContext(parent context.Context, id, schema string) ([]Table, error) {
	v, release, err := s.acquireContext(parent, id)
	if err != nil {
		return nil, err
	}
	defer release()
	base, finish := sessionContext(parent, v)
	defer finish()
	ctx, cancel := context.WithTimeout(base, 15*time.Second)
	defer cancel()
	defer s.retireInterruptedSession(ctx, id, v)
	return v.connector.Tables(ctx, v.conn, schema)
}
func (s *Service) Structure(req TableRequest) (Structure, error) {
	return s.StructureContext(context.Background(), req)
}
func (s *Service) StructureContext(parent context.Context, req TableRequest) (Structure, error) {
	v, release, err := s.acquireContext(parent, req.SessionID)
	if err != nil {
		return Structure{}, err
	}
	defer release()
	base, finish := sessionContext(parent, v)
	defer finish()
	ctx, cancel := context.WithTimeout(base, 15*time.Second)
	defer cancel()
	defer s.retireInterruptedSession(ctx, req.SessionID, v)
	return v.connector.Structure(ctx, v.conn, req.Database, req.Table)
}

func (s *Service) GenerateSQL(req SQLTemplateRequest) (string, error) {
	v, release, err := s.acquire(req.SessionID)
	if err != nil {
		return "", err
	}
	defer release()
	generator, ok := v.connector.(SQLGenerator)
	if !ok {
		return "", fmt.Errorf("该数据库暂不支持生成 SQL")
	}
	ctx, cancel := context.WithTimeout(v.ctx, 15*time.Second)
	defer cancel()
	defer s.retireInterruptedSession(ctx, req.SessionID, v)
	return generator.GenerateSQL(ctx, v.conn, req)
}

func (s *Service) Cancel(requestID string) bool {
	s.mu.Lock()
	cancel := s.requests[requestID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		return true
	}
	return false
}

func (s *Service) Browse(req TableRequest) (QueryResult, error) {
	if req.Offset < 0 || req.Offset > 10000000 {
		return QueryResult{}, fmt.Errorf("分页位置超出范围")
	}
	if req.Limit < 1 || req.Limit > 1000 {
		req.Limit = 100
	}
	s.mu.Lock()
	v := s.sessions[req.SessionID]
	s.mu.Unlock()
	if v == nil {
		return QueryResult{}, fmt.Errorf("数据库尚未连接")
	}
	if req.RequestID == "" {
		req.RequestID = uuid.NewString()
	}
	return s.Run(QueryRequest{SessionID: req.SessionID, RequestID: req.RequestID, SQL: v.connector.BrowseSQL(req.Database, req.Table, req.Offset, req.Limit), Limit: req.Limit, TimeoutSeconds: 30})
}

func (s *Service) Run(req QueryRequest) (QueryResult, error) {
	return s.RunContext(context.Background(), req)
}
func (s *Service) RunContext(parent context.Context, req QueryRequest) (out QueryResult, returnErr error) {
	if _, err := uuid.Parse(req.RequestID); err != nil {
		return QueryResult{}, fmt.Errorf("查询标识无效")
	}
	v, release, err := s.acquireContext(parent, req.SessionID)
	if err != nil {
		return QueryResult{}, err
	}
	defer release()
	dialect := "mysql"
	if jdbc, ok := v.connector.(JDBC); ok {
		dialect = jdbc.definition.Dialect
	}
	stmts, err := executionSQL(req, dialect)
	if err != nil {
		return QueryResult{}, err
	}
	if v.info.ReadOnly {
		for _, stmt := range stmts {
			if !returnsRowsFor(stmt, dialect) {
				return QueryResult{}, fmt.Errorf("当前连接使用只读执行，请在连接设置中开启写入后重连")
			}
		}
	}
	if req.Limit < 1 || req.Limit > 5000 {
		req.Limit = 1000
	}
	if req.TimeoutSeconds < 1 || req.TimeoutSeconds > 120 {
		req.TimeoutSeconds = 30
	}
	base, finish := sessionContext(parent, v)
	defer finish()
	ctx, cancel := context.WithTimeout(base, time.Duration(req.TimeoutSeconds)*time.Second)
	defer cancel()
	var interrupted atomic.Bool
	var stopOnce sync.Once
	stopQuery := func() {
		stopOnce.Do(func() {
			interrupted.Store(true)
			if control, ok := v.connector.(QueryCanceller); ok && v.serverID != "" {
				killCtx, finish := context.WithTimeout(context.Background(), 2*time.Second)
				_ = control.Cancel(killCtx, v.db, v.serverID)
				finish()
			}
			cancel()
		})
	}
	s.mu.Lock()
	if _, exists := s.requests[req.RequestID]; exists {
		s.mu.Unlock()
		return QueryResult{}, fmt.Errorf("查询标识重复")
	}
	s.requests[req.RequestID] = stopQuery
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.requests, req.RequestID); s.mu.Unlock() }()
	start := time.Now()
	out = QueryResult{Sets: []ResultSet{}}
	defer func() {
		if ctx.Err() != nil || interrupted.Load() {
			stopQuery()
			out.Cancelled = true
			out.Error = "已停止查询并关闭连接；已提交的写入不会撤销，请重连后核实结果"
			out.Disconnected = true
		}
		if errors.Is(err, sql.ErrConnDone) || errors.Is(err, driver.ErrBadConn) {
			out.Disconnected = true
		}
		if health, ok := v.connector.(interface{ Disconnected(*sql.Conn) bool }); ok && health.Disconnected(v.conn) {
			out.Disconnected = true
		}
		if out.Disconnected {
			s.discardSession(req.SessionID, v)
		}
		out.DurationMS = time.Since(start).Milliseconds()
	}()
	if req.Database != "" {
		if len(req.Database) > 64 {
			return out, fmt.Errorf("数据库名称过长")
		}
		if chooser, ok := v.connector.(databaseSelector); ok {
			err = chooser.SelectDatabase(ctx, v.conn, req.Database)
		} else {
			_, err = v.conn.ExecContext(ctx, "USE "+quoteIdentifier(req.Database))
		}
		if err != nil {
			out.Error = fmt.Sprintf("选择数据库失败：%s", err.Error())
			return out, nil
		}
	}
	var tx *sql.Tx
	if v.info.ReadOnly {
		tx, err = v.conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			out.Error = fmt.Sprintf("开始只读事务失败：%s", err.Error())
			return out, nil
		}
		defer tx.Rollback()
	}
	resultBudget := 8 << 20
	for _, stmt := range stmts {
		if resultBudget <= 0 {
			out.Error = "累计查询结果已达到 8 MiB 上限，后续语句未执行"
			break
		}
		var result ResultSet
		if returnsRowsFor(stmt, dialect) {
			var q queryer = v.conn
			if tx != nil {
				q = tx
			}
			result, err = readBoundedResult(ctx, q, stmt, req.Limit, &resultBudget)
		} else {
			var r sql.Result
			r, err = v.conn.ExecContext(ctx, stmt)
			result = ResultSet{Columns: []Column{}, Rows: [][]*string{}}
			if err == nil {
				result.AffectedRows, _ = r.RowsAffected()
			}
		}
		result.Statement = stmt
		if err != nil {
			out.Error = fmt.Sprintf("第 %d 条 SQL 执行失败，后续语句未执行：%s", len(out.Sets)+1, err.Error())
			break
		}
		out.Sets = append(out.Sets, result)
	}
	return out, nil
}

type databaseSelector interface {
	SelectDatabase(context.Context, *sql.Conn, string) error
}
type versionReader interface {
	Version(context.Context, *sql.Conn) (string, error)
}

func connectorVersion(ctx context.Context, adapter Connector, c *sql.Conn) (string, error) {
	if reader, ok := adapter.(versionReader); ok {
		return reader.Version(ctx, c)
	}
	var version string
	err := c.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)
	return version, err
}

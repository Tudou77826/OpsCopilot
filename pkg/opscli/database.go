package opscli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"opscopilot/pkg/dbworkspace"
	"opscopilot/pkg/opssession"
	"opscopilot/pkg/sshclient"
)

const databaseOutputLimit = 32 * 1024

type serverDriverCapability struct {
	DriverID        string `json:"driverId"`
	ReadOnlyAdapted bool   `json:"readOnlyAdapted"`
	Reason          string `json:"reason"`
}

type serverDriverStatus struct {
	dbworkspace.JDBCStatus
	ServerCapabilities []serverDriverCapability `json:"serverCapabilities"`
}

func supportsServerReadOnly(driver dbworkspace.JDBCDriver) bool {
	return driver.Dialect == "mysql"
}

type opsCLIMemorySecrets struct{ values map[string]string }

func (m *opsCLIMemorySecrets) Set(_, user, password string) error {
	m.values[user] = password
	return nil
}
func (m *opsCLIMemorySecrets) Get(_, user string) (string, error) {
	v, ok := m.values[user]
	if !ok {
		return "", errors.New("secret not found")
	}
	return v, nil
}
func (m *opsCLIMemorySecrets) Delete(_, user string) error { delete(m.values, user); return nil }

func driverCommand(ctx context.Context, args []string, input io.Reader) (any, error) {
	if len(args) == 0 {
		return nil, errors.New("driver 需要 list 或 import")
	}
	var path string
	switch args[0] {
	case "list":
		if err := flags("driver list", args[1:], func(*flag.FlagSet) {}); err != nil {
			return nil, err
		}
	case "import":
		if err := flags("driver import", args[1:], func(f *flag.FlagSet) { f.StringVar(&path, "input", "", "JSON 输入") }); err != nil {
			return nil, err
		}
		if err := requireOptions("--input", path); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("未知 driver 操作，请查看 driver 帮助")
	}
	driverDir := strings.TrimSpace(os.Getenv("OPSCOPILOT_JDBC_DRIVER_DIR"))
	if driverDir == "" {
		return nil, errors.New("请配置 OPSCOPILOT_JDBC_DRIVER_DIR")
	}
	s := dbworkspace.New(filepath.Join(driverDir, ".opscopilot-profiles.json"), &opsCLIMemorySecrets{values: map[string]string{}})
	if runtime := strings.TrimSpace(os.Getenv("OPSCOPILOT_JDBC_RUNTIME_DIR")); runtime != "" {
		s.JDBCRuntimeDir = runtime
	}
	s.JDBCDriverDir = driverDir
	defer s.Close()
	switch args[0] {
	case "list":
		status, err := s.JDBCStatus()
		if err != nil {
			return nil, err
		}
		result := serverDriverStatus{JDBCStatus: status, ServerCapabilities: []serverDriverCapability{}}
		for _, driver := range status.Drivers {
			capability := serverDriverCapability{DriverID: driver.ID, ReadOnlyAdapted: supportsServerReadOnly(driver), Reason: "已适配 MySQL 服务端只读事务；连接、权限和目标版本仍需实际验证"}
			if !capability.ReadOnlyAdapted {
				capability.Reason = "尚未适配服务端强制只读事务，database 子命令暂不支持此驱动"
			}
			result.ServerCapabilities = append(result.ServerCapabilities, capability)
		}
		return result, nil
	case "import":
		var in dbworkspace.JDBCImport
		if err := decodeInput(path, input, &in); err != nil {
			return nil, err
		}
		return s.ImportJDBC(ctx, in)
	default:
		return nil, fmt.Errorf("未知 driver 操作: %s", args[0])
	}
}

func queryDatabase(ctx context.Context, store *opssession.Store, session string, node opssession.Node, action, database, table, sqlText string, limit, seconds int) (any, error) {
	if action != "query" && action != "schemas" && action != "tables" && action != "structure" {
		return nil, errors.New("不支持的数据库操作")
	}
	if node.Kind != "database" || node.Database == nil {
		return nil, errors.New("数据库操作需要 database 节点")
	}
	if strings.TrimSpace(node.Database.DriverID) == "" {
		return nil, errors.New("数据库节点缺少 driverID")
	}
	if node.Database.Engine != "" && node.Database.Engine != "jdbc" {
		return nil, errors.New("服务端数据库操作仅支持 JDBC")
	}
	if limit < 1 || limit > 5000 {
		return nil, errors.New("返回行数限制必须为 1–5000")
	}
	if seconds < 1 || seconds > 120 {
		return nil, errors.New("数据库查询超时必须为 1–120 秒")
	}
	if action == "query" && strings.TrimSpace(sqlText) == "" {
		return nil, errors.New("query 需要 --sql")
	}
	if (action == "tables" || action == "structure") && strings.TrimSpace(table) == "" && action == "structure" {
		return nil, errors.New("structure 需要 --table")
	}

	if store == nil {
		return nil, errors.New("数据库操作需要有效的 Session store")
	}
	workDir, err := store.WorkDirContext(ctx, session)
	if err != nil {
		return nil, err
	}
	tempDir, err := os.MkdirTemp(workDir, ".jdbc-run-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	secrets := &opsCLIMemorySecrets{values: map[string]string{}}
	s := dbworkspace.New(filepath.Join(tempDir, "profiles.json"), secrets)
	if runtime := strings.TrimSpace(os.Getenv("OPSCOPILOT_JDBC_RUNTIME_DIR")); runtime != "" {
		s.JDBCRuntimeDir = runtime
	}
	driverDir := strings.TrimSpace(os.Getenv("OPSCOPILOT_JDBC_DRIVER_DIR"))
	if driverDir == "" {
		return nil, errors.New("请配置 OPSCOPILOT_JDBC_DRIVER_DIR")
	}
	s.JDBCDriverDir = driverDir
	defer s.Close()

	p := *node.Database
	registry, err := s.JDBCStatus()
	if err != nil {
		return nil, err
	}
	found := false
	for _, driver := range registry.Drivers {
		if driver.ID != strings.TrimSpace(p.DriverID) {
			continue
		}
		found = true
		if !supportsServerReadOnly(driver) {
			return nil, errors.New("该 JDBC 方言尚未适配服务端强制只读事务，暂不支持自动定位；需要先完成并验证方言适配")
		}
	}
	if !found {
		return nil, errors.New("未找到节点指定的 driverId；请运行 driver list，选择已部署的驱动后用 connection put 更新该节点")
	}
	// The profile belongs only to this invocation.  Save must create a fresh
	// profile instead of treating an external node ID as an update target.
	p.ID = ""
	p.Revision = 0
	p.Engine = "jdbc"
	p.ReadOnly = true
	p.DriverID = strings.TrimSpace(p.DriverID)
	if p.GatewayID == "" {
		p.GatewayID = strings.TrimSpace(node.GatewayID)
	}
	if p.GatewayID != "" {
		if store == nil {
			return nil, errors.New("SSH 网关需要 Session store")
		}
		gateway, e := store.NodeContext(ctx, session, p.GatewayID)
		if e != nil {
			return nil, e
		}
		if gateway.Kind != "ssh" || gateway.SSH == nil {
			return nil, errors.New("数据库网关必须是 SSH 节点")
		}
		s.Gateway = func(openCtx context.Context, gatewayID, _ string) (dbworkspace.DialContext, func() error, error) {
			if gatewayID != p.GatewayID {
				return nil, nil, errors.New("SSH 网关标识不匹配")
			}
			client, e := sshclient.NewClientWithTrace(openCtx, gateway.SSH)
			if e != nil {
				return nil, nil, e
			}
			dial := func(dialCtx context.Context, target string) (net.Conn, error) {
				return client.DialDatabaseContext(dialCtx, target)
			}
			return dial, client.Close, nil
		}
	}
	var password *string
	if node.Password != nil {
		value := *node.Password
		password = &value
	}
	profile, err := s.Save(dbworkspace.ProfileInput{Profile: p, Password: password})
	if err != nil {
		return nil, err
	}
	info, err := s.Connect(ctx, profile.ID)
	if err != nil {
		return nil, err
	}
	defer s.DisconnectContext(context.Background(), info.ID)

	switch action {
	case "query":
		result, runErr := s.RunContext(ctx, dbworkspace.QueryRequest{SessionID: info.ID, RequestID: uuid.NewString(), Database: database, SQL: sqlText, Limit: limit, TimeoutSeconds: seconds, Mode: "statement"})
		result = boundOpsCLIQueryResult(result, nodeSecrets(node))
		if runErr != nil {
			return result, runErr
		}
		if result.Error != "" {
			return result, errors.New(result.Error)
		}
		if result.Cancelled {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			return result, errors.New("查询已取消")
		}
		return result, nil
	case "schemas":
		value, e := s.SchemasContext(ctx, info.ID)
		return boundOpsCLIStrings(value, nodeSecrets(node)), e
	case "tables":
		value, e := s.TablesContext(ctx, info.ID, database)
		return boundOpsCLITables(value, nodeSecrets(node)), e
	default:
		value, e := s.StructureContext(ctx, dbworkspace.TableRequest{SessionID: info.ID, Database: database, Table: table})
		return boundOpsCLIStructure(value, nodeSecrets(node)), e
	}
}

func boundOpsCLIStrings(values []string, secrets []string) any {
	truncated := false
	for i := range values {
		values[i] = boundOpsCLIText(values[i], 1024, &truncated, secrets)
	}
	for len(mustJSON(map[string]any{"items": values, "truncated": truncated})) > databaseOutputLimit && len(values) > 0 {
		keep := len(values) / 2
		if keep == len(values) {
			keep--
		}
		values = values[:keep]
		truncated = true
	}
	return map[string]any{"items": values, "truncated": truncated}
}
func boundOpsCLITables(values []dbworkspace.Table, secrets []string) any {
	truncated := false
	for i := range values {
		values[i].Name = boundOpsCLIText(values[i].Name, 1024, &truncated, secrets)
		values[i].Type = boundOpsCLIText(values[i].Type, 1024, &truncated, secrets)
		values[i].Comment = boundOpsCLIText(values[i].Comment, 2048, &truncated, secrets)
	}
	for len(mustJSON(map[string]any{"items": values, "truncated": truncated})) > databaseOutputLimit && len(values) > 0 {
		keep := len(values) / 2
		if keep == len(values) {
			keep--
		}
		values = values[:keep]
		truncated = true
	}
	return map[string]any{"items": values, "truncated": truncated}
}
func boundOpsCLIStructure(value dbworkspace.Structure, secrets []string) any {
	truncated := false
	value.DDL = boundOpsCLIText(value.DDL, 8192, &truncated, secrets)
	value.Columns = boundOpsCLIResultSet(value.Columns, secrets)
	value.Indexes = boundOpsCLIResultSet(value.Indexes, secrets)
	truncated = truncated || value.Columns.Truncated || value.Indexes.Truncated
	result := map[string]any{"structure": value, "truncated": truncated}
	for len(mustJSON(result)) > databaseOutputLimit {
		if len(value.DDL) > 128 {
			value.DDL = boundOpsCLIText(value.DDL, len(value.DDL)/2, &truncated, secrets)
		} else if len(value.Indexes.Rows) > 0 {
			value.Indexes.Rows = value.Indexes.Rows[:len(value.Indexes.Rows)/2]
			value.Indexes.Truncated = true
		} else if len(value.Columns.Rows) > 0 {
			value.Columns.Rows = value.Columns.Rows[:len(value.Columns.Rows)/2]
			value.Columns.Truncated = true
		} else if len(value.Indexes.Columns) > 0 {
			value.Indexes.Columns = value.Indexes.Columns[:len(value.Indexes.Columns)/2]
			value.Indexes.Truncated = true
			for i := range value.Indexes.Rows {
				if len(value.Indexes.Rows[i]) > len(value.Indexes.Columns) {
					value.Indexes.Rows[i] = value.Indexes.Rows[i][:len(value.Indexes.Columns)]
				}
			}
		} else if len(value.Columns.Columns) > 0 {
			value.Columns.Columns = value.Columns.Columns[:len(value.Columns.Columns)/2]
			value.Columns.Truncated = true
			for i := range value.Columns.Rows {
				if len(value.Columns.Rows[i]) > len(value.Columns.Columns) {
					value.Columns.Rows[i] = value.Columns.Rows[i][:len(value.Columns.Columns)]
				}
			}
		} else {
			value.DDL = "[结果已截断]"
			truncated = true
			result = map[string]any{"structure": value, "truncated": true}
			break
		}
		result = map[string]any{"structure": value, "truncated": truncated || value.Columns.Truncated || value.Indexes.Truncated}
	}
	return result
}
func boundOpsCLIQueryResult(value dbworkspace.QueryResult, secrets []string) dbworkspace.QueryResult {
	errorTruncated := false
	value.Error = boundOpsCLIText(value.Error, 4096, &errorTruncated, secrets)
	if errorTruncated {
		if len(value.Sets) == 0 {
			value.Sets = []dbworkspace.ResultSet{{Truncated: true}}
		} else {
			value.Sets[0].Truncated = true
		}
	}
	for i := range value.Sets {
		value.Sets[i] = boundOpsCLIResultSet(value.Sets[i], secrets)
	}
	for len(mustJSON(value)) > databaseOutputLimit && len(value.Sets) > 0 {
		trimmed := false
		for i := len(value.Sets) - 1; i >= 0; i-- {
			if len(value.Sets[i].Rows) == 0 {
				continue
			}
			keep := len(value.Sets[i].Rows) / 2
			if keep == len(value.Sets[i].Rows) {
				keep--
			}
			value.Sets[i].Rows = value.Sets[i].Rows[:keep]
			value.Sets[i].Truncated = true
			trimmed = true
			break
		}
		if !trimmed {
			break
		}
	}
	for len(mustJSON(value)) > databaseOutputLimit {
		changed := false
		for i := range value.Sets {
			set := &value.Sets[i]
			if len(set.Statement) > 256 {
				set.Statement = boundOpsCLIText(set.Statement, len(set.Statement)/2, &set.Truncated, secrets)
				changed = true
			}
			for j := range set.Rows {
				for k := range set.Rows[j] {
					if set.Rows[j][k] != nil && len(*set.Rows[j][k]) > 512 {
						v := boundOpsCLIText(*set.Rows[j][k], len(*set.Rows[j][k])/2, &set.Truncated, secrets)
						set.Rows[j][k] = &v
						changed = true
					}
				}
			}
			if len(set.Columns) > 0 && !changed {
				keep := len(set.Columns) / 2
				if keep == len(set.Columns) {
					keep--
				}
				set.Columns = set.Columns[:keep]
				set.Truncated = true
				for j := range set.Rows {
					if len(set.Rows[j]) > len(set.Columns) {
						set.Rows[j] = set.Rows[j][:len(set.Columns)]
					}
				}
				changed = true
			}
		}
		if !changed {
			if len(value.Sets) > 1 {
				value.Sets = value.Sets[:len(value.Sets)-1]
				continue
			}
			if len(value.Sets) == 0 {
				value.Sets = []dbworkspace.ResultSet{{Truncated: true}}
			}
			value.Sets[0].Truncated = true
			break
		}
	}
	return value
}
func boundOpsCLIResultSet(value dbworkspace.ResultSet, secrets []string) dbworkspace.ResultSet {
	for i := range value.Columns {
		value.Columns[i].Name = boundOpsCLIText(value.Columns[i].Name, 1024, &value.Truncated, secrets)
		value.Columns[i].Type = boundOpsCLIText(value.Columns[i].Type, 1024, &value.Truncated, secrets)
	}
	value.Statement = boundOpsCLIText(value.Statement, 4096, &value.Truncated, secrets)
	for r := range value.Rows {
		for c := range value.Rows[r] {
			if value.Rows[r][c] != nil {
				v := boundOpsCLIText(*value.Rows[r][c], 4096, &value.Truncated, secrets)
				value.Rows[r][c] = &v
			}
		}
	}
	return value
}

func boundOpsCLIText(value string, limit int, truncated *bool, secrets ...[]string) string {
	for _, list := range secrets {
		value = hideCredentials(value, list)
	}
	if len(value) <= limit {
		if !utf8.ValidString(value) {
			*truncated = true
			return strings.ToValidUTF8(value, "�")
		}
		return value
	}
	*truncated = true
	if limit < 1 {
		return "[结果已截断]"
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return strings.ToValidUTF8(value[:cut], "�") + "\n[结果已截断]"
}
func mustJSON(value any) []byte { b, _ := json.Marshal(value); return b }

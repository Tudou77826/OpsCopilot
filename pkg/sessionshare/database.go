package sessionshare

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GatewaySecrets 描述数据库连接依赖的 SSH 隧道节点（支持多级堡垒机），
// 作为 DatabaseSecretsPayload 的一部分加密存储。
type GatewaySecrets struct {
	Name         string          `json:"name,omitempty"`
	Host         string          `json:"host"`
	Port         int             `json:"port"`
	User         string          `json:"user,omitempty"`
	Password     string          `json:"password,omitempty"`
	RootPassword string          `json:"root_password,omitempty"`
	Bastion      *GatewaySecrets `json:"bastion,omitempty"`
}

// DatabaseSecretsPayload 是需要加密保护的数据库连接凭据。
type DatabaseSecretsPayload struct {
	Password string          `json:"password,omitempty"`
	Gateway  *GatewaySecrets `json:"gateway,omitempty"`
}

// SharedDatabase 是共享仓库中一个数据库连接条目（databases/<owner>.json）。
// 元数据明文存储，凭据与 SSH 隧道信息加密存储在 SecretsEnc 中。
type SharedDatabase struct {
	Owner                       string    `json:"owner"`
	Name                        string    `json:"name"`
	Engine                      string    `json:"engine"` // "mysql" | "jdbc"
	DriverName                  string    `json:"driver_name,omitempty"`
	DriverClassName             string    `json:"driver_class_name,omitempty"`
	DriverDialect               string    `json:"driver_dialect,omitempty"`
	JDBCAllowPublicKeyRetrieval bool      `json:"jdbc_allow_public_key_retrieval,omitempty"`
	Host                        string    `json:"host"`
	Port                        int       `json:"port"`
	User                        string    `json:"user"`
	Database                    string    `json:"database,omitempty"`
	Environment                 string    `json:"environment,omitempty"`
	TLS                         string    `json:"tls,omitempty"`
	ReadOnly                    bool      `json:"read_only,omitempty"`
	HasGateway                  bool      `json:"has_gateway,omitempty"`
	GatewayHost                 string    `json:"gateway_host,omitempty"`
	GatewayMode                 string    `json:"gateway_mode,omitempty"`
	SecretsEnc                  string    `json:"secrets_enc,omitempty"`
	LastLoginAt                 time.Time `json:"last_login_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

func normalizeDBEngine(engine string) string {
	e := strings.ToLower(strings.TrimSpace(engine))
	if e == "" {
		return "jdbc"
	}
	return e
}

// EndpointKey 返回数据库端点的逻辑唯一标识（不含 owner，供跨用户 LWW 合并与本地簿记使用）。
// 包含 GatewayHost 以区分经由不同 SSH 跳板访问的 127.0.0.1:3306。
func (d SharedDatabase) EndpointKey() string {
	return normalizeDBEngine(d.Engine) + "|" +
		strings.TrimSpace(d.DriverClassName) + "|" +
		d.Host + "|" +
		strconv.Itoa(d.Port) + "|" +
		d.User + "|" +
		d.Database + "|" +
		d.GatewayHost
}

// EntryKey 返回条目的唯一标识（含 owner）。
func (d SharedDatabase) EntryKey() string {
	return d.Owner + "|" + d.EndpointKey()
}

// SameDatabaseEndpoint 判断两条共享数据库记录是否指向同一逻辑端点。
func SameDatabaseEndpoint(a, b SharedDatabase) bool {
	return a.EndpointKey() == b.EndpointKey()
}

// DatabaseStore 是数据库连接共享存储的可选扩展接口，由 GitSessionStore 实现。
type DatabaseStore interface {
	UploadDatabase(ctx context.Context, db SharedDatabase) error
	DeleteDatabase(ctx context.Context, endpoint SharedDatabase) error
	DownloadAllDatabases(ctx context.Context) ([]SharedDatabase, error)
}

// MergeSharedDatabases 跨用户合并：同一端点多条记录取 LastLoginAt 最新者，
// 返回结果按 LastLoginAt 降序排序。
func MergeSharedDatabases(all []SharedDatabase) []SharedDatabase {
	byEndpoint := make(map[string]SharedDatabase, len(all))
	for _, d := range all {
		key := d.EndpointKey()
		existing, ok := byEndpoint[key]
		if !ok || d.LastLoginAt.After(existing.LastLoginAt) {
			byEndpoint[key] = d
		}
	}

	merged := make([]SharedDatabase, 0, len(byEndpoint))
	for _, d := range byEndpoint {
		merged = append(merged, d)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].LastLoginAt.After(merged[j].LastLoginAt)
	})
	return merged
}

// LocalDatabaseEntry 是本地数据库共享簿记中的单条记录。
type LocalDatabaseEntry struct {
	SharedDatabase
	LastPushedAt time.Time `json:"last_pushed_at,omitempty"`
}

// Pending 判断该数据库端点是否有未推送的连接记录。
func (e LocalDatabaseEntry) Pending() bool {
	return e.LastLoginAt.After(e.LastPushedAt)
}

// DatabaseLocalState 本地数据库共享簿记（sessionstore-databases-local.json）。
type DatabaseLocalState struct {
	filePath string
	mu       sync.Mutex
	entries  map[string]LocalDatabaseEntry // key: SharedDatabase.EndpointKey()
}

// NewDatabaseLocalState 创建本地数据库共享簿记。
func NewDatabaseLocalState(path string) *DatabaseLocalState {
	return &DatabaseLocalState{
		filePath: path,
		entries:  make(map[string]LocalDatabaseEntry),
	}
}

// Load 从磁盘加载（不存在时初始化空文件）。
func (l *DatabaseLocalState) Load() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := os.ReadFile(l.filePath)
	if os.IsNotExist(err) {
		l.entries = make(map[string]LocalDatabaseEntry)
		return l.saveLocked()
	}
	if err != nil {
		return err
	}

	entries := make(map[string]LocalDatabaseEntry)
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse database local state: %w", err)
	}
	l.entries = entries
	return nil
}

// RecordLogin 记录一次成功的数据库连接。
func (l *DatabaseLocalState) RecordLogin(entry LocalDatabaseEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := entry.EndpointKey()
	if existing, ok := l.entries[key]; ok {
		if entry.LastLoginAt.Before(existing.LastLoginAt) {
			return
		}
		entry.LastPushedAt = existing.LastPushedAt
		if entry.SecretsEnc == "" {
			entry.SecretsEnc = existing.SecretsEnc
		}
	}
	l.entries[key] = entry
	if err := l.saveLocked(); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] session share: write database local state: %v\n", err)
	}
}

// PendingEntries 返回待推送的数据库端点，按连接时间升序。
func (l *DatabaseLocalState) PendingEntries() []LocalDatabaseEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	var pending []LocalDatabaseEntry
	for _, e := range l.entries {
		if e.Pending() {
			pending = append(pending, e)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].LastLoginAt.Before(pending[j].LastLoginAt)
	})
	return pending
}

// MarkPushed 将指定端点的推送进度推进到 loginAt。
func (l *DatabaseLocalState) MarkPushed(endpointKey string, loginAt time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.entries[endpointKey]
	if !ok || e.LastPushedAt.After(loginAt) || e.LastPushedAt.Equal(loginAt) {
		return
	}
	e.LastPushedAt = loginAt
	l.entries[endpointKey] = e
	if err := l.saveLocked(); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] session share: write database local state: %v\n", err)
	}
}

// Remove 删除指定端点的簿记。
func (l *DatabaseLocalState) Remove(endpointKey string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.entries, endpointKey)
	if err := l.saveLocked(); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] session share: write database local state: %v\n", err)
	}
}

func (l *DatabaseLocalState) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(l.filePath), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	data, err := json.MarshalIndent(l.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.filePath, data, 0600)
}

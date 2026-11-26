package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/dbworkspace"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/sessionshare"
)

const sharedDatabaseGroupName = "团队共享"

// SharedDatabaseView 前端可见的共享数据库条目（不含任何明文凭据）。
type SharedDatabaseView struct {
	EntryKey        string `json:"entryKey"`
	Owner           string `json:"owner"`
	Name            string `json:"name"`
	Engine          string `json:"engine"`
	DriverName      string `json:"driverName,omitempty"`
	DriverClassName string `json:"driverClassName,omitempty"`
	DriverDialect   string `json:"driverDialect,omitempty"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	User            string `json:"user"`
	Database        string `json:"database,omitempty"`
	Environment     string `json:"environment,omitempty"`
	TLS             string `json:"tls,omitempty"`
	ReadOnly        bool   `json:"readOnly,omitempty"`
	HasGateway      bool   `json:"hasGateway,omitempty"`
	GatewayHost     string `json:"gatewayHost,omitempty"`
	GatewayMode     string `json:"gatewayMode,omitempty"`
	LastLoginAt     string `json:"lastLoginAt"`
	Own             bool   `json:"own"`
	HasSecrets      bool   `json:"hasSecrets"`
	Decryptable     bool   `json:"decryptable"`
}

// SharedDatabaseImportResult ImportSharedDatabase 的返回结果。
type SharedDatabaseImportResult struct {
	Success         bool                 `json:"success"`
	Message         string               `json:"message,omitempty"`
	Profile         *dbworkspace.Profile `json:"profile,omitempty"`
	Reused          bool                 `json:"reused,omitempty"`
	MissingDriver   bool                 `json:"missingDriver,omitempty"`
	DriverName      string               `json:"driverName,omitempty"`
	DriverClassName string               `json:"driverClassName,omitempty"`
}

func (rt *sessionShareRuntime) dbMergedSnapshot() []sharedDatabaseViewEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return append([]sharedDatabaseViewEntry(nil), rt.dbMerged...)
}

func (rt *sessionShareRuntime) upsertDBMergedView(db sessionshare.SharedDatabase, decryptable bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	replaced := false
	for i, v := range rt.dbMerged {
		if sessionshare.SameDatabaseEndpoint(v.db, db) {
			if !db.LastLoginAt.Before(v.db.LastLoginAt) {
				rt.dbMerged[i] = sharedDatabaseViewEntry{db: db, decryptable: decryptable}
			}
			replaced = true
			break
		}
	}
	if !replaced {
		rt.dbMerged = append(rt.dbMerged, sharedDatabaseViewEntry{db: db, decryptable: decryptable})
	}

	sort.SliceStable(rt.dbMerged, func(i, j int) bool {
		return rt.dbMerged[i].db.LastLoginAt.After(rt.dbMerged[j].db.LastLoginAt)
	})
	rt.status.DatabaseEntryCount = len(rt.dbMerged)
}

// recordSharedDatabaseLogin 在 DatabaseConnect 成功后异步记录并推送该数据库连接。
// 全异步 + recover，任何共享故障不影响数据库主流程。
func (a *App) recordSharedDatabaseLogin(profileID string) {
	rt := a.getSessionShare()
	if rt == nil || rt.dbLocal == nil {
		return
	}
	dbStore, ok := rt.store.(sessionshare.DatabaseStore)
	if !ok {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("session share: recordSharedDatabaseLogin panic", "recover", r)
			}
		}()

		profile, ok := a.findDatabaseProfile(profileID)
		if !ok {
			return
		}

		password, err := a.databaseService().Password(profileID)
		if err != nil {
			slog.Warn("session share: read database password failed", "error", err, "profile", profileID)
			password = ""
		}

		var driverName, driverClassName, driverDialect string
		if profile.Engine == "jdbc" && profile.DriverID != "" {
			if status, err := a.databaseService().JDBCStatus(); err == nil {
				for _, d := range status.Drivers {
					if d.ID == profile.DriverID {
						driverName = d.Name
						driverClassName = d.ClassName
						driverDialect = d.Dialect
						break
					}
				}
			}
		}

		var gatewaySecrets *sessionshare.GatewaySecrets
		var gatewayHost string
		if profile.GatewayID != "" {
			if gwCfg := a.resolveGatewayConfigWithSecrets(profile.GatewayID); gwCfg != nil {
				gatewayHost = gwCfg.Host
				gatewaySecrets = gatewaySecretsFromConfig(gwCfg)
			}
		}

		payload := sessionshare.DatabaseSecretsPayload{
			Password: password,
			Gateway:  gatewaySecrets,
		}
		passphrase := strings.TrimSpace(a.configMgr.Config.SessionShare.SecretKey)
		secretsEnc := ""
		if hasAnyDBSecret(&payload) {
			if passphrase == "" {
				slog.Warn("session share: secret key not configured, sharing database metadata only",
					"host", profile.Host)
			} else {
				enc, err := sessionshare.EncryptDatabaseSecrets(payload, passphrase)
				if err != nil {
					slog.Error("session share: encrypt database secrets failed", "error", err)
					return
				}
				secretsEnc = enc
			}
		}

		now := time.Now()
		dbEntry := sessionshare.SharedDatabase{
			Name:                        profile.Name,
			Engine:                      profile.Engine,
			DriverName:                  driverName,
			DriverClassName:             driverClassName,
			DriverDialect:               driverDialect,
			JDBCAllowPublicKeyRetrieval: profile.JDBCAllowPublicKeyRetrieval,
			Host:                        profile.Host,
			Port:                        profile.Port,
			User:                        profile.User,
			Database:                    profile.Database,
			Environment:                 profile.Environment,
			TLS:                         profile.TLS,
			ReadOnly:                    profile.ReadOnly,
			HasGateway:                  profile.GatewayID != "",
			GatewayHost:                 gatewayHost,
			GatewayMode:                 profile.GatewayMode,
			SecretsEnc:                  secretsEnc,
			LastLoginAt:                 now,
			UpdatedAt:                   now,
		}

		rt.dbLocal.RecordLogin(sessionshare.LocalDatabaseEntry{SharedDatabase: dbEntry})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		dbEntry.Owner = rt.resolveOwner(ctx)
		if err := dbStore.UploadDatabase(ctx, dbEntry); err != nil {
			slog.Warn("session share: upload database after connect failed (will retry on sync)",
				"error", err, "host", profile.Host)
			return
		}
		rt.dbLocal.MarkPushed(dbEntry.EndpointKey(), now)
		rt.upsertDBMergedView(dbEntry, true)
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "session-share:synced", nil)
		}
	}()
}

// syncSharedDatabases 在会话共享同步流程中同步数据库条目（拉取合并 + 补推待推记录）。
func (a *App) syncSharedDatabases(ctx context.Context, rt *sessionShareRuntime, owner, passphrase string) error {
	dbStore, ok := rt.store.(sessionshare.DatabaseStore)
	if !ok {
		return nil
	}

	all, err := dbStore.DownloadAllDatabases(ctx)
	if err != nil {
		return fmt.Errorf("download shared databases: %w", err)
	}

	merged := sessionshare.MergeSharedDatabases(all)
	keyOK := passphrase != ""
	sampleIdx := -1
	for i, d := range merged {
		if d.SecretsEnc != "" {
			sampleIdx = i
			break
		}
	}
	if keyOK && sampleIdx >= 0 {
		if _, err := sessionshare.DecryptDatabaseSecrets(merged[sampleIdx].SecretsEnc, passphrase); err != nil {
			keyOK = false
		}
	}

	views := make([]sharedDatabaseViewEntry, 0, len(merged))
	for _, d := range merged {
		views = append(views, sharedDatabaseViewEntry{
			db:          d,
			decryptable: d.SecretsEnc == "" || keyOK,
		})
	}

	var pending []sessionshare.LocalDatabaseEntry
	if rt.dbLocal != nil {
		pending = rt.dbLocal.PendingEntries()
	}

	rt.mu.Lock()
	rt.dbMerged = views
	rt.status.DatabaseEntryCount = len(views)
	rt.status.PendingCount = len(rt.local.PendingEntries()) + len(pending)
	if !keyOK && sampleIdx >= 0 && rt.keyWarning == "" {
		rt.keyWarning = "共享密钥可能不正确，无法解密共享密码"
	}
	rt.mu.Unlock()

	for _, e := range pending {
		upload := e.SharedDatabase
		upload.Owner = owner
		upload.UpdatedAt = time.Now()
		if err := dbStore.UploadDatabase(ctx, upload); err != nil {
			slog.Warn("session share: flush pending database failed", "error", err, "host", e.Host)
			return fmt.Errorf("upload pending database %s: %w", e.Host, err)
		}
		rt.dbLocal.MarkPushed(e.EndpointKey(), e.LastLoginAt)
		rt.upsertDBMergedView(upload, upload.SecretsEnc == "" || keyOK)
	}
	return nil
}

// --- Wails 绑定 ---

// GetSharedDatabases 返回跨用户合并后的共享数据库列表（不含明文凭据）。
func (a *App) GetSharedDatabases() string {
	rt := a.getSessionShare()
	if rt == nil {
		return `{"enabled":false,"databases":[]}`
	}

	owner := rt.resolveOwner(context.Background())
	merged := rt.dbMergedSnapshot()
	const maxEntries = 50
	if len(merged) > maxEntries {
		merged = merged[:maxEntries]
	}

	dbs := make([]SharedDatabaseView, 0, len(merged))
	for _, v := range merged {
		d := v.db
		dbs = append(dbs, SharedDatabaseView{
			EntryKey:        d.EntryKey(),
			Owner:           d.Owner,
			Name:            d.Name,
			Engine:          d.Engine,
			DriverName:      d.DriverName,
			DriverClassName: d.DriverClassName,
			DriverDialect:   d.DriverDialect,
			Host:            d.Host,
			Port:            d.Port,
			User:            d.User,
			Database:        d.Database,
			Environment:     d.Environment,
			TLS:             d.TLS,
			ReadOnly:        d.ReadOnly,
			HasGateway:      d.HasGateway,
			GatewayHost:     d.GatewayHost,
			GatewayMode:     d.GatewayMode,
			LastLoginAt:     d.LastLoginAt.Format("2006-01-02 15:04:05"),
			Own:             d.Owner == owner,
			HasSecrets:      d.SecretsEnc != "",
			Decryptable:     v.decryptable,
		})
	}

	return marshalJSON(map[string]interface{}{
		"enabled":   true,
		"owner":     owner,
		"databases": dbs,
	})
}

// ImportSharedDatabase 将共享数据库连接解密、匹配本机驱动与 SSH 隧道并收入本地「团队共享」分组。
// 若本地已有相同端点连接，则直接复用返回，避免重复创建。
func (a *App) ImportSharedDatabase(entryKey string) SharedDatabaseImportResult {
	rt := a.getSessionShare()
	if rt == nil {
		return SharedDatabaseImportResult{Success: false, Message: "会话共享未启用"}
	}

	entry, ok := findSharedDatabaseEntry(rt, entryKey)
	if !ok {
		return SharedDatabaseImportResult{Success: false, Message: "共享数据库连接不存在，请刷新后重试"}
	}
	d := entry.db

	secrets, err := a.decryptSharedDatabaseSecrets(d)
	if err != nil {
		return SharedDatabaseImportResult{Success: false, Message: err.Error()}
	}

	// 1. 若为 JDBC 引擎，按驱动类名（回退方言）匹配本机已导入的驱动
	driverID := ""
	if strings.EqualFold(d.Engine, "jdbc") {
		status, err := a.databaseService().JDBCStatus()
		if err != nil {
			return SharedDatabaseImportResult{Success: false, Message: fmt.Sprintf("检查本机 JDBC 驱动失败: %v", err)}
		}
		driverID = matchLocalJDBCDriver(status.Drivers, d.DriverClassName, d.DriverDialect)
		if driverID == "" {
			label := d.DriverName
			if label == "" {
				label = d.DriverClassName
			}
			if label == "" {
				label = d.DriverDialect
			}
			return SharedDatabaseImportResult{
				Success:         false,
				MissingDriver:   true,
				DriverName:      d.DriverName,
				DriverClassName: d.DriverClassName,
				Message:         fmt.Sprintf("本机未导入驱动「%s」，请先在驱动管理中导入后再连接", label),
			}
		}
	}

	// 2. 若依赖 SSH 隧道，自动将隧道配置 Ensure 到本地会话树并绑定新 GatewayID
	gatewayID := ""
	if d.HasGateway {
		if secrets.Gateway == nil {
			return SharedDatabaseImportResult{Success: false, Message: "该共享连接依赖 SSH 隧道，但缺少隧道配置（请检查共享密钥）"}
		}
		if a.savedSessionMgr == nil {
			return SharedDatabaseImportResult{Success: false, Message: "本地会话管理未就绪，无法保存 SSH 隧道"}
		}
		gwCfg := remoteConfigFromGatewaySecrets(secrets.Gateway)
		node, err := a.savedSessionMgr.EnsureConnectionByEndpoint(gwCfg, "")
		if err != nil {
			return SharedDatabaseImportResult{Success: false, Message: fmt.Sprintf("保存关联 SSH 隧道失败: %v", err)}
		}
		gatewayID = node.ID
	}

	// 3. 若本地已有相同端点的数据库连接，直接复用
	existingProfiles, err := a.databaseService().List()
	if err != nil {
		return SharedDatabaseImportResult{Success: false, Message: fmt.Sprintf("读取本地数据库连接失败: %v", err)}
	}
	for _, p := range existingProfiles {
		if sameLocalDatabaseProfile(p, d, driverID, gatewayID) {
			copyProfile := p
			return SharedDatabaseImportResult{Success: true, Profile: &copyProfile, Reused: true}
		}
	}

	// 4. 保存到本地「团队共享」分组
	var pwPtr *string
	if secrets.Password != "" {
		pw := secrets.Password
		pwPtr = &pw
	}
	saved, err := a.databaseService().Save(dbworkspace.ProfileInput{
		Profile: dbworkspace.Profile{
			Name:                        d.Name,
			Environment:                 d.Environment,
			Group:                       sharedDatabaseGroupName,
			Engine:                      d.Engine,
			DriverID:                    driverID,
			JDBCAllowPublicKeyRetrieval: d.JDBCAllowPublicKeyRetrieval,
			Host:                        d.Host,
			Port:                        d.Port,
			User:                        d.User,
			Database:                    d.Database,
			TLS:                         d.TLS,
			ReadOnly:                    d.ReadOnly,
			GatewayID:                   gatewayID,
			GatewayMode:                 d.GatewayMode,
		},
		Password: pwPtr,
	})
	if err != nil {
		return SharedDatabaseImportResult{Success: false, Message: fmt.Sprintf("保存数据库连接失败: %v", err)}
	}

	return SharedDatabaseImportResult{Success: true, Profile: &saved, Reused: false}
}

// RemoveSharedDatabase 删除共享数据库条目（仅允许删除自己的）。
func (a *App) RemoveSharedDatabase(entryKey string) string {
	rt := a.getSessionShare()
	if rt == nil {
		return "会话共享未启用"
	}
	dbStore, ok := rt.store.(sessionshare.DatabaseStore)
	if !ok {
		return "当前共享存储不支持数据库连接"
	}

	entry, ok := findSharedDatabaseEntry(rt, entryKey)
	if !ok {
		return "共享数据库连接不存在"
	}

	ctxOwner := rt.resolveOwner(context.Background())
	if entry.db.Owner != ctxOwner {
		return "只能删除自己共享的条目"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := dbStore.DeleteDatabase(ctx, entry.db); err != nil {
		return fmt.Sprintf("删除共享失败: %v", err)
	}

	if rt.dbLocal != nil {
		rt.dbLocal.Remove(entry.db.EndpointKey())
	}
	rt.mu.Lock()
	kept := rt.dbMerged[:0]
	for _, v := range rt.dbMerged {
		if v.db.EntryKey() != entryKey {
			kept = append(kept, v)
		}
	}
	rt.dbMerged = kept
	rt.status.DatabaseEntryCount = len(rt.dbMerged)
	rt.mu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "session-share:synced", nil)
	}
	return ""
}

// --- 辅助函数 ---

func findSharedDatabaseEntry(rt *sessionShareRuntime, entryKey string) (sharedDatabaseViewEntry, bool) {
	for _, v := range rt.dbMergedSnapshot() {
		if v.db.EntryKey() == entryKey {
			return v, true
		}
	}
	return sharedDatabaseViewEntry{}, false
}

func (a *App) decryptSharedDatabaseSecrets(d sessionshare.SharedDatabase) (sessionshare.DatabaseSecretsPayload, error) {
	if d.SecretsEnc == "" {
		return sessionshare.DatabaseSecretsPayload{}, nil
	}
	passphrase := strings.TrimSpace(a.configMgr.Config.SessionShare.SecretKey)
	if passphrase == "" {
		return sessionshare.DatabaseSecretsPayload{}, fmt.Errorf("未配置共享密钥，无法解密数据库连接密码")
	}
	return sessionshare.DecryptDatabaseSecrets(d.SecretsEnc, passphrase)
}

func (a *App) findDatabaseProfile(id string) (dbworkspace.Profile, bool) {
	list, err := a.databaseService().List()
	if err != nil {
		return dbworkspace.Profile{}, false
	}
	for _, p := range list {
		if p.ID == id {
			return p, true
		}
	}
	return dbworkspace.Profile{}, false
}

func (a *App) resolveGatewayConfigWithSecrets(gatewayID string) *remote.ConnectConfig {
	if a.savedSessionMgr == nil || gatewayID == "" {
		return nil
	}
	var cfg *remote.ConnectConfig
	var walk func([]*connectionstore.Node)
	walk = func(nodes []*connectionstore.Node) {
		for _, n := range nodes {
			if n.ID == gatewayID && n.Config != nil {
				copied := cloneConnectConfig(n.Config)
				if copied.Name == "" {
					copied.Name = n.Name
				}
				cfg = copied
				return
			}
			walk(n.Children)
			if cfg != nil {
				return
			}
		}
	}
	walk(a.savedSessionMgr.Snapshot())
	if cfg == nil {
		return nil
	}
	var fillSecrets func(*remote.ConnectConfig)
	fillSecrets = func(c *remote.ConnectConfig) {
		if c == nil {
			return
		}
		if c.Password == "" && a.secretStore != nil {
			c.Password, _ = a.secretStore.Get("OpsCopilot-SSH", c.Host+":"+c.User)
			if c.Password == "" {
				c.Password, _ = a.secretStore.Get("opscopilot", c.Host+"_"+c.User)
			}
		}
		fillSecrets(c.Bastion)
	}
	fillSecrets(cfg)
	return cfg
}

func cloneConnectConfig(c *remote.ConnectConfig) *remote.ConnectConfig {
	if c == nil {
		return nil
	}
	copied := *c
	copied.Bastion = cloneConnectConfig(c.Bastion)
	return &copied
}

func gatewaySecretsFromConfig(c *remote.ConnectConfig) *sessionshare.GatewaySecrets {
	if c == nil {
		return nil
	}
	port := c.Port
	if port == 0 {
		port = 22
	}
	return &sessionshare.GatewaySecrets{
		Name:         c.Name,
		Host:         c.Host,
		Port:         port,
		User:         c.User,
		Password:     c.Password,
		RootPassword: c.RootPassword,
		Bastion:      gatewaySecretsFromConfig(c.Bastion),
	}
}

func remoteConfigFromGatewaySecrets(g *sessionshare.GatewaySecrets) remote.ConnectConfig {
	port := g.Port
	if port == 0 {
		port = 22
	}
	cfg := remote.ConnectConfig{
		Name:         g.Name,
		Protocol:     remote.ProtocolSSH,
		Host:         g.Host,
		Port:         port,
		User:         g.User,
		Password:     g.Password,
		RootPassword: g.RootPassword,
	}
	if g.Bastion != nil {
		b := remoteConfigFromGatewaySecrets(g.Bastion)
		cfg.Bastion = &b
	}
	return cfg
}

func hasAnyDBSecret(p *sessionshare.DatabaseSecretsPayload) bool {
	if p == nil {
		return false
	}
	if p.Password != "" {
		return true
	}
	return p.Gateway != nil
}

func matchLocalJDBCDriver(drivers []dbworkspace.JDBCDriver, className, dialect string) string {
	className = strings.TrimSpace(className)
	dialect = strings.TrimSpace(dialect)
	if className != "" {
		for _, d := range drivers {
			if strings.EqualFold(strings.TrimSpace(d.ClassName), className) {
				return d.ID
			}
		}
	}
	if dialect != "" {
		for _, d := range drivers {
			if strings.EqualFold(strings.TrimSpace(d.Dialect), dialect) {
				return d.ID
			}
		}
	}
	return ""
}

func sameLocalDatabaseProfile(p dbworkspace.Profile, d sessionshare.SharedDatabase, driverID, gatewayID string) bool {
	if !strings.EqualFold(p.Engine, d.Engine) {
		return false
	}
	if p.Host != d.Host || p.Port != d.Port || p.User != d.User || p.Database != d.Database {
		return false
	}
	if p.GatewayID != gatewayID {
		return false
	}
	if strings.EqualFold(d.Engine, "jdbc") && p.DriverID != driverID {
		return false
	}
	return true
}

// app_sessionshare.go 会话连接信息共享（issue #57）的编排层与 Wails 绑定。
//
// 架构约定（见设计文档）：
//   - app.go 仅持有一个 sessionShareRuntime 指针字段 + ConnectWithID 内
//     单行钩子，本文件承载全部逻辑，避免 app.go 持续膨胀；
//   - pkg/sessionshare 为纯叶子包，本层负责 config/remote 类型与
//     sessionshare 自有类型之间的映射；
//   - 共享故障与连接主流程隔离：钩子全异步、内部 recover，任何异常
//     只降级为"暂不共享"。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"opscopilot/pkg/remote"
	"opscopilot/pkg/sessionshare"
)

// SessionShareStatus 会话共享同步状态快照（前端轮询，结构对齐 PatchSyncStatus）。
type SessionShareStatus struct {
	Enabled         bool   `json:"enabled"`
	Configured      bool   `json:"configured"`
	HasSecretKey    bool   `json:"hasSecretKey"`
	Running         bool   `json:"running"`
	PendingCount    int    `json:"pendingCount"`
	EntryCount      int    `json:"entryCount"`
	Owner           string `json:"owner,omitempty"`
	LastSyncAt      string `json:"lastSyncAt,omitempty"`
	LastSyncSuccess bool   `json:"lastSyncSuccess"`
	LastSyncMessage string `json:"lastSyncMessage,omitempty"`
	RemoteURL       string `json:"remoteURL,omitempty"`
	Branch          string `json:"branch,omitempty"`
}

// SharedSessionView 前端可见的共享会话条目（不含任何明文凭据）。
type SharedSessionView struct {
	EntryKey    string `json:"entryKey"`
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Protocol    string `json:"protocol,omitempty"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	LastLoginAt string `json:"lastLoginAt"`
	Own         bool   `json:"own"`         // 当前用户的条目（可删除共享）
	HasSecrets  bool   `json:"hasSecrets"`  // 是否携带加密凭据
	Decryptable bool   `json:"decryptable"` // 凭据能否用当前密钥解开
}

// sessionShareRuntime 会话共享的运行时状态（App 持有其指针，nil = 未启用）。
type sessionShareRuntime struct {
	store sessionshare.Store
	local *sessionshare.LocalState

	ownerMu sync.RWMutex
	owner   string // 当前用户标识（首次同步时解析）

	mu     sync.RWMutex
	status SessionShareStatus
	merged []sharedViewEntry // 跨用户合并后的最新视图（按最近登录降序）
	// 密钥探测失败的提示文案（同步成功但有密钥问题时展示，空 = 无警告）
	keyWarning string

	syncing atomic.Bool
}

type sharedViewEntry struct {
	session     sessionshare.SharedSession
	decryptable bool
}

// --- 生命周期 ---

func (a *App) getSessionShare() *sessionShareRuntime {
	a.sessionShareMu.RLock()
	defer a.sessionShareMu.RUnlock()
	return a.sessionShare
}

func (a *App) setSessionShare(rt *sessionShareRuntime) {
	a.sessionShareMu.Lock()
	a.sessionShare = rt
	a.sessionShareMu.Unlock()
}

// sessionShareStoreDir 共享仓库的本地 clone 目录（与 patchstore 同级）。
func (a *App) sessionShareStoreDir() string {
	return filepath.Join(filepath.Dir(a.configMgr.Config.Log.Dir), "sessionstore")
}

// initSessionShareStore 根据配置初始化会话共享（startup 与 SaveSettings 调用）。
func (a *App) initSessionShareStore() {
	cfg := a.configMgr.Config.SessionShare
	remoteURL := strings.TrimSpace(cfg.RemoteURL)
	if !cfg.Enabled || remoteURL == "" {
		a.setSessionShare(nil)
		return
	}

	branch := strings.TrimSpace(cfg.Branch)
	if branch == "" {
		branch = "main"
	}

	localDir := a.sessionShareStoreDir()
	// 提前建目录：Owner() 解析 git user.name 需要在该目录下执行 git config
	if err := os.MkdirAll(localDir, 0755); err != nil {
		slog.Error("session share: create local dir failed", "error", err)
	}

	store := sessionshare.NewGitSessionStore(remoteURL, localDir, branch)
	local := sessionshare.NewLocalState(filepath.Join(filepath.Dir(localDir), "sessionstore-local.json"))
	if err := local.Load(); err != nil {
		slog.Warn("session share: load local state failed", "error", err)
	}

	rt := &sessionShareRuntime{
		store: store,
		local: local,
		status: SessionShareStatus{
			Enabled:      true,
			Configured:   true,
			HasSecretKey: strings.TrimSpace(cfg.SecretKey) != "",
			RemoteURL:    remoteURL,
			Branch:       branch,
		},
	}
	a.setSessionShare(rt)

	go func() {
		if err := a.runSessionShareSync(rt); err != nil {
			slog.Error("session share: initial sync failed", "error", err)
		}
	}()
}

// --- 状态维护 ---

func (rt *sessionShareRuntime) updateStatus(update func(*SessionShareStatus)) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	update(&rt.status)
}

func (rt *sessionShareRuntime) statusSnapshot() SessionShareStatus {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.status
}

func (rt *sessionShareRuntime) mergedSnapshot() []sharedViewEntry {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return append([]sharedViewEntry(nil), rt.merged...)
}

func (rt *sessionShareRuntime) syncMessage() string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.keyWarning
}

// resolveOwner 解析当前用户标识（缓存），供条目 Own 判定与删除权限校验。
func (rt *sessionShareRuntime) resolveOwner(ctx context.Context) string {
	rt.ownerMu.RLock()
	if rt.owner != "" {
		defer rt.ownerMu.RUnlock()
		return rt.owner
	}
	rt.ownerMu.RUnlock()

	rt.ownerMu.Lock()
	defer rt.ownerMu.Unlock()
	if rt.owner == "" { // double-check
		if gs, ok := rt.store.(*sessionshare.GitSessionStore); ok {
			rt.owner = gs.Owner(ctx)
		} else {
			rt.owner = "unknown"
		}
	}
	return rt.owner
}

// --- 连接成功钩子（ConnectWithID 单行调用） ---

// recordSharedLogin 记录一次成功登录并异步推送。全异步 + recover，
// 任何共享故障不影响连接主流程。
func (a *App) recordSharedLogin(cfg ConnectConfig) {
	rt := a.getSessionShare()
	if rt == nil {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("session share: recordSharedLogin panic", "recover", r)
			}
		}()

		passphrase := strings.TrimSpace(a.configMgr.Config.SessionShare.SecretKey)
		now := time.Now()

		// 有凭据且配置了密钥才加密；密钥缺失时仅共享元数据并提示
		secrets := secretsPayloadFromConfig(cfg)
		secretsEnc := ""
		if hasAnySecret(secrets) {
			if passphrase == "" {
				slog.Warn("session share: secret key not configured, sharing metadata only",
					"host", cfg.Host)
			} else {
				enc, err := sessionshare.EncryptSecrets(*secrets, passphrase)
				if err != nil {
					slog.Error("session share: encrypt secrets failed", "error", err)
					return
				}
				secretsEnc = enc
			}
		}

		entry := sessionshare.LocalEntry{
			Name:        sessionDisplayName(cfg),
			Protocol:    cfg.Protocol,
			Host:        cfg.Host,
			Port:        cfg.Port,
			User:        cfg.User,
			SecretsEnc:  secretsEnc,
			LastLoginAt: now,
		}
		rt.local.RecordLogin(entry)

		// 异步推送（失败静默，留待下次同步由本地簿记补推）
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		owner := rt.resolveOwner(ctx)
		upload := sessionshare.SharedSession{
			Owner:       owner,
			Name:        entry.Name,
			Protocol:    entry.Protocol,
			Host:        entry.Host,
			Port:        entry.Port,
			User:        entry.User,
			SecretsEnc:  secretsEnc,
			LastLoginAt: now,
			UpdatedAt:   now,
		}
		if err := rt.store.Upload(ctx, upload); err != nil {
			slog.Warn("session share: upload after login failed (will retry on sync)",
				"error", err, "host", cfg.Host)
			return
		}
		rt.local.MarkPushed(entry.Protocol, entry.Host, entry.Port, entry.User, now)
		// 推送成功后就地更新合并视图，让共享面板立即看到本条
		// （merged 正常由同步刷新，而同步只在启动/保存设置/手动重试时触发）
		rt.upsertMergedView(upload, true)
	}()
}

// upsertMergedView 按端点更新内存中的合并视图并维持按最近登录降序。
func (rt *sessionShareRuntime) upsertMergedView(s sessionshare.SharedSession, decryptable bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	replaced := false
	for i, v := range rt.merged {
		if sessionshare.SameEndpoint(v.session, s) {
			// 仅登录时间更新时替换，保持与远端一致的合并语义
			if !s.LastLoginAt.Before(v.session.LastLoginAt) {
				rt.merged[i] = sharedViewEntry{session: s, decryptable: decryptable}
			}
			replaced = true
			break
		}
	}
	if !replaced {
		rt.merged = append(rt.merged, sharedViewEntry{session: s, decryptable: decryptable})
	}

	sort.SliceStable(rt.merged, func(i, j int) bool {
		return rt.merged[i].session.LastLoginAt.After(rt.merged[j].session.LastLoginAt)
	})
	rt.status.EntryCount = len(rt.merged)
}

func secretsPayloadFromConfig(cfg ConnectConfig) *sessionshare.SecretsPayload {
	payload := &sessionshare.SecretsPayload{
		Password:     cfg.Password,
		RootPassword: cfg.RootPassword,
	}
	if cfg.Bastion != nil {
		payload.Bastion = &sessionshare.BastionSecrets{
			Name:     cfg.Bastion.Name,
			Host:     cfg.Bastion.Host,
			Port:     cfg.Bastion.Port,
			User:     cfg.Bastion.User,
			Password: cfg.Bastion.Password,
		}
	}
	return payload
}

func hasAnySecret(p *sessionshare.SecretsPayload) bool {
	return p != nil && (p.Password != "" || p.RootPassword != "" ||
		(p.Bastion != nil && p.Bastion.Password != ""))
}

func sessionDisplayName(cfg ConnectConfig) string {
	if cfg.Name != "" {
		return cfg.Name
	}
	return cfg.Host
}

// --- 同步 ---

func (a *App) runSessionShareSync(rt *sessionShareRuntime) error {
	if !rt.syncing.CompareAndSwap(false, true) {
		return fmt.Errorf("session share sync already running")
	}
	defer rt.syncing.Store(false)

	rt.updateStatus(func(status *SessionShareStatus) {
		status.Running = true
		status.LastSyncMessage = "正在同步..."
	})

	err := a.syncSharedSessions(rt)
	now := time.Now().Format("2006-01-02 15:04:05")
	// syncMessage 必须在进入 updateStatus 前读取：
	// updateStatus 回调内已持有 rt.mu 写锁，回调内再 RLock 会死锁（RWMutex 不可重入）
	keyWarning := rt.syncMessage()
	rt.updateStatus(func(status *SessionShareStatus) {
		status.Running = false
		status.LastSyncAt = now
		status.LastSyncSuccess = err == nil
		if err != nil {
			status.LastSyncMessage = err.Error()
		} else if keyWarning != "" {
			// syncSharedSessions 探测到的更具体提示（如密钥不正确），优先保留
			status.LastSyncMessage = keyWarning
		} else {
			status.LastSyncMessage = "同步完成"
		}
	})

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "session-share:synced", nil)
	}
	return err
}

// syncSharedSessions 拉取全部用户条目 → 跨用户 merge → 试解密标记 →
// 补推本地待推端点。
func (a *App) syncSharedSessions(rt *sessionShareRuntime) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	all, err := rt.store.DownloadAll(ctx)
	if err != nil {
		return fmt.Errorf("download shared sessions: %w", err)
	}

	passphrase := strings.TrimSpace(a.configMgr.Config.SessionShare.SecretKey)
	owner := rt.resolveOwner(ctx)
	merged := sessionshare.MergeSharedSessions(all)

	// 密钥可用性探测：解密一条带凭据的样本。密钥错误时全部标记不可解，
	// 状态里给出明确提示（避免逐条 scrypt 派生的开销）。
	keyOK := passphrase != ""
	sampleIdx := -1
	for i, s := range merged {
		if s.SecretsEnc != "" {
			sampleIdx = i
			break
		}
	}
	if keyOK && sampleIdx >= 0 {
		if _, err := sessionshare.DecryptSecrets(merged[sampleIdx].SecretsEnc, passphrase); err != nil {
			keyOK = false
		}
	}

	views := make([]sharedViewEntry, 0, len(merged))
	for _, s := range merged {
		views = append(views, sharedViewEntry{
			session:     s,
			decryptable: s.SecretsEnc == "" || keyOK,
		})
	}

	pending := rt.local.PendingEntries()
	rt.mu.Lock()
	rt.merged = views
	rt.status.EntryCount = len(views)
	rt.status.PendingCount = len(pending)
	rt.status.Owner = owner
	rt.status.HasSecretKey = passphrase != ""
	if !keyOK && sampleIdx >= 0 {
		rt.keyWarning = "共享密钥可能不正确，无法解密共享密码"
	} else {
		rt.keyWarning = ""
	}
	rt.mu.Unlock()

	// 补推本地待推端点（登录时推送失败的兜底通道）
	for _, e := range pending {
		upload := sessionshare.SharedSession{
			Owner:       owner,
			Name:        e.Name,
			Protocol:    e.Protocol,
			Host:        e.Host,
			Port:        e.Port,
			User:        e.User,
			SecretsEnc:  e.SecretsEnc,
			LastLoginAt: e.LastLoginAt,
			UpdatedAt:   time.Now(),
		}
		if err := rt.store.Upload(ctx, upload); err != nil {
			slog.Warn("session share: flush pending entry failed", "error", err, "host", e.Host)
			return fmt.Errorf("upload pending session %s: %w", e.Host, err)
		}
		rt.local.MarkPushed(e.Protocol, e.Host, e.Port, e.User, e.LastLoginAt)
	}

	return nil
}

// --- Wails 绑定 ---

// GetSessionShareStatus 返回同步状态快照 JSON。
func (a *App) GetSessionShareStatus() string {
	rt := a.getSessionShare()
	if rt == nil {
		cfg := a.configMgr.Config.SessionShare
		return marshalJSON(SessionShareStatus{
			Enabled:         false,
			Configured:      strings.TrimSpace(cfg.RemoteURL) != "",
			HasSecretKey:    strings.TrimSpace(cfg.SecretKey) != "",
			LastSyncMessage: "会话共享未启用",
		})
	}
	rt.updateStatus(func(status *SessionShareStatus) {
		status.PendingCount = len(rt.local.PendingEntries())
	})
	return marshalJSON(rt.statusSnapshot())
}

// GetSharedSessions 返回跨用户合并后的共享会话列表（不含明文凭据）。
// 返回 {"enabled":bool,"owner":string,"sessions":[...]}。
func (a *App) GetSharedSessions() string {
	rt := a.getSessionShare()
	if rt == nil {
		return `{"enabled":false,"sessions":[]}`
	}

	owner := rt.resolveOwner(context.Background())
	merged := rt.mergedSnapshot()
	const maxEntries = 50
	if len(merged) > maxEntries {
		merged = merged[:maxEntries]
	}

	sessions := make([]SharedSessionView, 0, len(merged))
	for _, v := range merged {
		s := v.session
		sessions = append(sessions, SharedSessionView{
			EntryKey:    s.EntryKey(),
			Owner:       s.Owner,
			Name:        s.Name,
			Protocol:    s.Protocol,
			Host:        s.Host,
			Port:        s.Port,
			User:        s.User,
			LastLoginAt: s.LastLoginAt.Format("2006-01-02 15:04:05"),
			Own:         s.Owner == owner,
			HasSecrets:  s.SecretsEnc != "",
			Decryptable: v.decryptable,
		})
	}

	result := map[string]interface{}{
		"enabled":  true,
		"owner":    owner,
		"sessions": sessions,
	}
	return marshalJSON(result)
}

// SharedConnectResult ConnectSharedSession 的返回：解密后的连接配置交还前端，
// 由前端走统一连接流程（App.tsx handleConnect：开终端 tab、状态栏、错误弹窗）。
// 凭据解密在后端完成后随配置返回——与既有架构一致
// （Connect/GetSavedSessions 本就由前端携带密码）。
type SharedConnectResult struct {
	Success bool           `json:"success"`
	Message string         `json:"message,omitempty"`
	Config  *ConnectConfig `json:"config,omitempty"`
}

// ConnectSharedSession 取回一条共享会话的解密连接配置（不拨号），
// 前端拿到配置后走与普通连接完全相同的流程。
func (a *App) ConnectSharedSession(entryKey string) SharedConnectResult {
	rt := a.getSessionShare()
	if rt == nil {
		return SharedConnectResult{Success: false, Message: "会话共享未启用"}
	}

	entry, ok := findSharedEntry(rt, entryKey)
	if !ok {
		return SharedConnectResult{Success: false, Message: "共享会话不存在，请刷新后重试"}
	}

	secrets, err := a.decryptSharedSecrets(entry.session)
	if err != nil {
		return SharedConnectResult{Success: false, Message: err.Error()}
	}

	cfg := ConnectConfig{
		Name:     entry.session.Name,
		Protocol: entry.session.Protocol,
		Host:     entry.session.Host,
		Port:     entry.session.Port,
		User:     entry.session.User,
		Password: secrets.Password,
		// root 密码走 RootPassword 字段（ConnectWithID 会据此选择 sudo 启动）
		RootPassword: secrets.RootPassword,
	}
	if secrets.Bastion != nil {
		cfg.Bastion = &ConnectConfig{
			Name:     secrets.Bastion.Name,
			Host:     secrets.Bastion.Host,
			Port:     secrets.Bastion.Port,
			User:     secrets.Bastion.User,
			Password: secrets.Bastion.Password,
		}
	}

	return SharedConnectResult{Success: true, Config: &cfg}
}

// SaveSharedSessionToLocal 将共享会话解密后保存到本地会话树。
func (a *App) SaveSharedSessionToLocal(entryKey string) string {
	rt := a.getSessionShare()
	if rt == nil {
		return "会话共享未启用"
	}

	entry, ok := findSharedEntry(rt, entryKey)
	if !ok {
		return "共享会话不存在，请刷新后重试"
	}

	secrets, err := a.decryptSharedSecrets(entry.session)
	if err != nil {
		return err.Error()
	}

	cfg := remote.ConnectConfig{
		Name:     entry.session.Name,
		Protocol: entry.session.Protocol,
		Host:     entry.session.Host,
		Port:     entry.session.Port,
		User:     entry.session.User,
		Password: secrets.Password,
	}
	if secrets.RootPassword != "" {
		cfg.RootPassword = secrets.RootPassword
	}
	if secrets.Bastion != nil {
		cfg.Bastion = &remote.ConnectConfig{
			Name:     secrets.Bastion.Name,
			Host:     secrets.Bastion.Host,
			Port:     secrets.Bastion.Port,
			User:     secrets.Bastion.User,
			Password: secrets.Bastion.Password,
		}
	}

	if err := a.savedSessionMgr.Upsert(cfg, ""); err != nil {
		return fmt.Sprintf("保存失败: %v", err)
	}
	return ""
}

// RemoveSharedSession 删除共享条目（仅允许删除自己的）。
func (a *App) RemoveSharedSession(entryKey string) string {
	rt := a.getSessionShare()
	if rt == nil {
		return "会话共享未启用"
	}

	parts := strings.Split(entryKey, "|")
	if len(parts) != 5 {
		return "条目标识格式错误"
	}
	owner, protocol, host, portStr, user := parts[0], parts[1], parts[2], parts[3], parts[4]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "条目标识格式错误（端口）"
	}

	ctxOwner := rt.resolveOwner(context.Background())
	if owner != ctxOwner {
		return "只能删除自己共享的条目"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	endpoint := sessionshare.SharedSession{
		Owner: owner, Protocol: protocol, Host: host, Port: port, User: user,
	}
	if err := rt.store.Delete(ctx, endpoint); err != nil {
		return fmt.Sprintf("删除共享失败: %v", err)
	}

	// 同步清理本地簿记与内存视图，防止下次同步补推
	rt.local.Remove(protocol, host, port, user)
	rt.mu.Lock()
	kept := rt.merged[:0]
	for _, v := range rt.merged {
		if v.session.EntryKey() != entryKey {
			kept = append(kept, v)
		}
	}
	rt.merged = kept
	rt.status.EntryCount = len(rt.merged)
	rt.mu.Unlock()
	return ""
}

// RetrySessionShareSync 手动触发一次同步。
func (a *App) RetrySessionShareSync() string {
	rt := a.getSessionShare()
	if rt == nil {
		return "会话共享未启用或未配置 Git 仓库"
	}
	if err := a.runSessionShareSync(rt); err != nil {
		return err.Error()
	}
	return ""
}

// --- 辅助 ---

func findSharedEntry(rt *sessionShareRuntime, entryKey string) (sharedViewEntry, bool) {
	for _, v := range rt.mergedSnapshot() {
		if v.session.EntryKey() == entryKey {
			return v, true
		}
	}
	return sharedViewEntry{}, false
}

// decryptSharedSecrets 用当前配置的团队密钥解密条目凭据。
func (a *App) decryptSharedSecrets(s sessionshare.SharedSession) (sessionshare.SecretsPayload, error) {
	if s.SecretsEnc == "" {
		return sessionshare.SecretsPayload{}, nil
	}
	passphrase := strings.TrimSpace(a.configMgr.Config.SessionShare.SecretKey)
	if passphrase == "" {
		return sessionshare.SecretsPayload{}, fmt.Errorf("未配置共享密钥，无法解密连接密码")
	}
	return sessionshare.DecryptSecrets(s.SecretsEnc, passphrase)
}

func marshalJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Error("session share: marshal json failed", "error", err)
		return "{}"
	}
	return string(data)
}

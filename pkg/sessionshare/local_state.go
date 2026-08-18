package sessionshare

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// LocalEntry 是本地簿记中一个端点的状态。
// last_login_at / last_pushed_at 的差值驱动"待推送"判定，
// 替代补丁机制的 outbox：登录时即加密暂存（本地不落明文），
// 推送失败自动留存，下次同步补推。
type LocalEntry struct {
	Name         string    `json:"name"`
	Protocol     string    `json:"protocol,omitempty"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	User         string    `json:"user"`
	SecretsEnc   string    `json:"secrets_enc,omitempty"`
	LastLoginAt  time.Time `json:"last_login_at"`
	LastPushedAt time.Time `json:"last_pushed_at,omitempty"`
}

// Pending 判断该端点是否有未推送的登录记录。
func (e LocalEntry) Pending() bool {
	return e.LastLoginAt.After(e.LastPushedAt)
}

// LocalState 本地簿记（sessionstore-local.json），单文件单写者。
type LocalState struct {
	filePath string
	mu       sync.Mutex
	entries  map[string]LocalEntry // key: protocol|host|port|user
}

// NewLocalState 创建本地簿记，path 通常是 <log目录同级>/sessionstore-local.json。
func NewLocalState(path string) *LocalState {
	return &LocalState{
		filePath: path,
		entries:  make(map[string]LocalEntry),
	}
}

func localKey(protocol, host string, port int, user string) string {
	return normalizeProtocol(protocol) + "|" + host + "|" + strconv.Itoa(port) + "|" + user
}

// Load 从磁盘加载（不存在时初始化空文件，便于排查）。
func (l *LocalState) Load() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := os.ReadFile(l.filePath)
	if os.IsNotExist(err) {
		l.entries = make(map[string]LocalEntry)
		return l.saveLocked()
	}
	if err != nil {
		return err
	}

	entries := make(map[string]LocalEntry)
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parse local state: %w", err)
	}
	l.entries = entries
	return nil
}

// RecordLogin 记录一次成功登录（仅当登录时间不早于既有记录时更新，
// 保持 LastLoginAt 单调，避免乱序回调回退时间）。
func (l *LocalState) RecordLogin(entry LocalEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := localKey(entry.Protocol, entry.Host, entry.Port, entry.User)
	if existing, ok := l.entries[key]; ok {
		if entry.LastLoginAt.Before(existing.LastLoginAt) {
			return
		}
		// 保留既有推送进度：登录时间更新但尚未推送
		entry.LastPushedAt = existing.LastPushedAt
		// 新登录未携带凭据（如密钥认证）而旧记录有密文时，保留旧密文
		if entry.SecretsEnc == "" {
			entry.SecretsEnc = existing.SecretsEnc
		}
	}
	l.entries[key] = entry
	if err := l.saveLocked(); err != nil {
		// 簿记写失败不阻断调用方（下次登录/同步会重写），仅记录
		fmt.Fprintf(os.Stderr, "[WARN] session share: write local state: %v\n", err)
	}
}

// PendingEntries 返回待推送端点，按登录时间升序（先登录先推）。
func (l *LocalState) PendingEntries() []LocalEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	var pending []LocalEntry
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

// MarkPushed 将端点的推送进度推进到指定登录时间。
func (l *LocalState) MarkPushed(protocol, host string, port int, user string, loginAt time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	key := localKey(protocol, host, port, user)
	e, ok := l.entries[key]
	if !ok || e.LastPushedAt.After(loginAt) || e.LastPushedAt.Equal(loginAt) {
		return
	}
	e.LastPushedAt = loginAt
	l.entries[key] = e
	if err := l.saveLocked(); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] session share: write local state: %v\n", err)
	}
}

// Remove 删除一个端点的簿记（端点共享被删除时使用）。
func (l *LocalState) Remove(protocol, host string, port int, user string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.entries, localKey(protocol, host, port, user))
	if err := l.saveLocked(); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] session share: write local state: %v\n", err)
	}
}

func (l *LocalState) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(l.filePath), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	data, err := json.MarshalIndent(l.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.filePath, data, 0600) // 含密文，收紧权限
}

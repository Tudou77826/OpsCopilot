// Package sessionshare 提供基于 Git 仓库的团队会话连接信息共享。
//
// 设计与 knowledge/patchstore 同构：本包为纯叶子包，不依赖项目内任何包，
// 仅通过自有类型 + 构造参数与调用方解耦。仓库内每个用户一个文件
// (sessions/<owner>.json)，跨用户零冲突；同端点重复记录在本地 merge 时
// 取 LastLoginAt 最新者。
package sessionshare

import (
	"context"
	"sort"
	"strconv"
	"time"
)

// SecretsPayload 是需要加密保护的连接凭据。序列化为 JSON 后整体加密。
type SecretsPayload struct {
	Password     string          `json:"password,omitempty"`
	RootPassword string          `json:"root_password,omitempty"`
	Bastion      *BastionSecrets `json:"bastion,omitempty"`
}

// BastionSecrets 堡垒机连接信息（作为主连接 secrets 的一部分加密存储）。
type BastionSecrets struct {
	Name     string `json:"name,omitempty"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

// SharedSession 是仓库中一个端点条目。元数据明文存储，
// SecretsEnc 为 AES-256-GCM 密文（base64(salt‖nonce‖ciphertext)）。
type SharedSession struct {
	Owner       string    `json:"owner"`
	Name        string    `json:"name"`
	Protocol    string    `json:"protocol,omitempty"` // 空 = ssh
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	User        string    `json:"user"`
	SecretsEnc  string    `json:"secrets_enc,omitempty"`
	LastLoginAt time.Time `json:"last_login_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SameEndpoint 判断两条记录是否指向同一端点（协议空值归一化为 ssh）。
func SameEndpoint(a, b SharedSession) bool {
	return normalizeProtocol(a.Protocol) == normalizeProtocol(b.Protocol) &&
		a.Host == b.Host && a.Port == b.Port && a.User == b.User
}

// EntryKey 返回条目的唯一标识，格式 owner|protocol|host|port|user。
func (s SharedSession) EntryKey() string {
	return s.Owner + "|" + normalizeProtocol(s.Protocol) + "|" + s.Host + "|" +
		strconv.Itoa(s.Port) + "|" + s.User
}

func normalizeProtocol(p string) string {
	if p == "" {
		return "ssh"
	}
	return p
}

// Store 是会话共享存储接口（与 patchstore.PatchStore 同构，便于未来
// 扩展 http/sftp 等后端形态）。
type Store interface {
	// Upload 将一条记录 upsert 到本人文件：同端点仅当新 LastLoginAt
	// 不早于已有记录时覆盖，否则保留远端较新记录。
	Upload(ctx context.Context, session SharedSession) error
	// Delete 从本人文件中移除指定端点条目。
	Delete(ctx context.Context, endpoint SharedSession) error
	// DownloadAll 拉取并返回仓库中全部用户的条目。
	DownloadAll(ctx context.Context) ([]SharedSession, error)
}

// MergeSharedSessions 跨用户合并：同一端点多条记录取 LastLoginAt 最新者。
// 返回结果按 LastLoginAt 降序排序。这是确定性的收敛规则
// （last-writer-wins-by-login-time），不依赖写入时刻。
func MergeSharedSessions(all []SharedSession) []SharedSession {
	byEndpoint := make(map[string]SharedSession, len(all))
	for _, s := range all {
		key := normalizeProtocol(s.Protocol) + "|" + s.Host + "|" +
			strconv.Itoa(s.Port) + "|" + s.User
		existing, ok := byEndpoint[key]
		if !ok || s.LastLoginAt.After(existing.LastLoginAt) {
			byEndpoint[key] = s
		}
	}

	merged := make([]SharedSession, 0, len(byEndpoint))
	for _, s := range byEndpoint {
		merged = append(merged, s)
	}
	sortByLastLoginDesc(merged)
	return merged
}

func sortByLastLoginDesc(sessions []SharedSession) {
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastLoginAt.After(sessions[j].LastLoginAt)
	})
}

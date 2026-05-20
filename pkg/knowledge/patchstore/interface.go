package patchstore

import (
	"context"
	"time"
)

// Patch 单条归档记录（补丁）
type Patch struct {
	ID        string    // 短 UUID（8 位）
	Service   string    // 微服务名
	Module    string    // 模块名
	Author    string    // 作者
	Timestamp time.Time // 归档时间
	Content   string    // Markdown 内容（一个 ## 场景段落）
}

// PatchStore 补丁存储接口
type PatchStore interface {
	// Upload 上传单条补丁
	Upload(ctx context.Context, patch Patch) error

	// Download 下载指定时间之后的补丁（增量同步）
	Download(ctx context.Context, since time.Time) ([]Patch, error)

	// DownloadAll 下载全部补丁（全量同步）
	DownloadAll(ctx context.Context) ([]Patch, error)

	// LastSyncTime 获取最近一次补丁的时间戳（用于增量同步起点）
	LastSyncTime(ctx context.Context) (time.Time, error)
}

package opssidecar

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"opscopilot/pkg/core/ops"
)

const (
	transferMaxPerHost = 2 // 每服务器并发上限：共享连接的 SFTP 通道有限
	transferMaxTotal   = 6 // 全局并发上限：控制 sidecar 内存与带宽占用

	transferProgressInterval = 100 * time.Millisecond // 进度事件节流间隔

	transferKindUpload   = "upload"
	transferKindDownload = "download"
)

var errTransferUnknown = errors.New("传输任务不存在")

// transferJob 一个异步传输任务。
type transferJob struct {
	id     string
	host   string
	ctx    context.Context
	cancel context.CancelFunc
}

// transferManager 传输任务注册表。任务在独立 goroutine 中执行，经事件下发
// 进度与结果；控制请求（启动/取消）即刻返回，不阻塞串行 dispatch。
type transferManager struct {
	emit func(event string, params any)

	mu      sync.Mutex
	jobs    map[string]*transferJob
	perHost map[string]chan struct{}
	total   chan struct{}
}

func newTransferManager(emit func(event string, params any)) *transferManager {
	return &transferManager{
		emit:    emit,
		jobs:    map[string]*transferJob{},
		perHost: map[string]chan struct{}{},
		total:   make(chan struct{}, transferMaxTotal),
	}
}

// start 启动传输任务并立即返回任务 ID。mgr/host 由调用方在串行路径上解析
// （含连接树新鲜度与节点校验），内核闸门（文件访问/大小限制）在 Manager
// 内部执行，调用方绕不过。
func (tm *transferManager) start(kind string, mgr *ops.Manager, host, localPath, remotePath string, maxBytes int, backup, mkdir bool) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	job := &transferJob{id: uuid.NewString(), host: host, ctx: ctx, cancel: cancel}

	tm.mu.Lock()
	tm.jobs[job.id] = job
	slot := tm.perHost[host]
	if slot == nil {
		slot = make(chan struct{}, transferMaxPerHost)
		tm.perHost[host] = slot
	}
	tm.mu.Unlock()

	go tm.run(kind, job, slot, mgr, localPath, remotePath, maxBytes, backup, mkdir)
	return job.id, nil
}

func (tm *transferManager) run(kind string, job *transferJob, slot chan struct{}, mgr *ops.Manager, localPath, remotePath string, maxBytes int, backup, mkdir bool) {
	defer tm.finish(job)

	// 并发限流：每服务器 2、全局 6；排队期间可被取消
	select {
	case slot <- struct{}{}:
	case <-job.ctx.Done():
		tm.emitDone(job, fmt.Errorf("传输已取消"), true, nil)
		return
	}
	defer func() { <-slot }()
	select {
	case tm.total <- struct{}{}:
	case <-job.ctx.Done():
		tm.emitDone(job, fmt.Errorf("传输已取消"), true, nil)
		return
	}
	defer func() { <-tm.total }()

	// 进度节流：100ms 一次，终值必达
	var progressMu sync.Mutex
	var last time.Time
	progress := func(done, total int64) {
		progressMu.Lock()
		defer progressMu.Unlock()
		now := time.Now()
		if done < total && now.Sub(last) < transferProgressInterval {
			return
		}
		last = now
		tm.emit(EventTransferProgress, map[string]any{
			"transferId": job.id,
			"done":       done,
			"total":      total,
		})
	}

	var err error
	var meta map[string]any
	switch kind {
	case transferKindUpload:
		res, uploadErr := mgr.Upload(job.host, remotePath, ops.UploadOptions{
			LocalPath: localPath,
			Backup:    backup,
			Mkdir:     mkdir,
			Progress:  progress,
			Context:   job.ctx,
		})
		err = uploadErr
		if res != nil {
			meta = res.Meta
		}
	case transferKindDownload:
		res, downloadErr := mgr.Download(job.host, remotePath, ops.DownloadOptions{
			LocalPath: localPath,
			MaxBytes:  maxBytes,
			Progress:  progress,
			Context:   job.ctx,
		})
		err = downloadErr
		if res != nil {
			meta = res.Meta
		}
	default:
		err = fmt.Errorf("未知传输类型: %s", kind)
	}
	tm.emitDone(job, err, job.ctx.Err() != nil, meta)
}

func (tm *transferManager) emitDone(job *transferJob, err error, cancelled bool, meta map[string]any) {
	params := map[string]any{"transferId": job.id}
	if err != nil {
		message := err.Error()
		if cancelled {
			message = "传输已取消"
		}
		params["ok"] = false
		params["error"] = message
		if cancelled {
			params["cancelled"] = true
		}
	} else {
		params["ok"] = true
		params["meta"] = meta
	}
	tm.emit(EventTransferDone, params)
}

// cancel 取消任务。任务已结束时为安全空操作。
func (tm *transferManager) cancel(transferID string) error {
	tm.mu.Lock()
	job := tm.jobs[transferID]
	tm.mu.Unlock()
	if job == nil {
		return errTransferUnknown
	}
	job.cancel()
	return nil
}

// finish 任务结束（成功/失败/取消）后移出注册表。
func (tm *transferManager) finish(job *transferJob) {
	tm.mu.Lock()
	delete(tm.jobs, job.id)
	tm.mu.Unlock()
}

// cancelAll 服务退出时取消全部在途任务。
func (tm *transferManager) cancelAll() {
	tm.mu.Lock()
	jobs := make([]*transferJob, 0, len(tm.jobs))
	for _, job := range tm.jobs {
		jobs = append(jobs, job)
	}
	tm.mu.Unlock()
	for _, job := range jobs {
		job.cancel()
	}
}

package opssidecar

// 文件传输与本地文件操作（FilesPanel 的 FileTransferHost 端口实现）。
//
// 语义对齐 OpsCopilot Wails App 面：
//   - FT*/Local* 方法返回 ftResponse JSON 信封字符串（app.go:1988 的结构）
//   - 进度/完成走 file-transfer-progress / file-transfer-done 事件，
//     载荷 {taskId, sessionId, bytesDone, bytesTotal, speedBps, step?} /
//     {taskId, sessionId, ok, code?, message?, cancelled?}
//   - 传输与「终端会话或服务器」绑定：sessionId 命中终端会话用其独立连接，
//     否则按树节点走共享池连接；SFTP 通道即用即建
//   - 并发限流：每目标 2、全局 6；排队任务先发 step="排队等待..." 进度

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/google/uuid"

	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/core/ops"
	"opscopilot/pkg/filetransfer"
)

// ftResponse 与 OpsCopilot app.go 的信封逐字段一致（FilesPanel 解析依赖）。
type ftResponse struct {
	OK      bool                         `json:"ok"`
	Message string                       `json:"message,omitempty"`
	Error   *filetransfer.TransferError  `json:"error,omitempty"`
	TaskID  string                       `json:"taskId,omitempty"`
	Entries []filetransfer.Entry         `json:"entries,omitempty"`
	Entry   *filetransfer.Entry          `json:"entry,omitempty"`
	Result  *filetransfer.TransferResult `json:"result,omitempty"`
}

func ftOK() string { out, _ := json.Marshal(ftResponse{OK: true}); return string(out) }
func ftMessage(msg string) string {
	out, _ := json.Marshal(ftResponse{OK: true, Message: msg})
	return string(out)
}
func ftTaskEnvelope(taskID string) string {
	out, _ := json.Marshal(ftResponse{OK: true, TaskID: taskID})
	return string(out)
}
func ftEntries(entries []filetransfer.Entry) string {
	out, _ := json.Marshal(ftResponse{OK: true, Entries: entries})
	return string(out)
}
func ftEntry(entry *filetransfer.Entry) string {
	out, _ := json.Marshal(ftResponse{OK: true, Entry: entry})
	return string(out)
}
func ftResult(result *filetransfer.TransferResult) string {
	out, _ := json.Marshal(ftResponse{OK: true, Result: result})
	return string(out)
}
func ftErr(err error) string {
	te, ok := err.(*filetransfer.TransferError)
	if !ok {
		te = &filetransfer.TransferError{Code: "error", Message: err.Error()}
	}
	return string(mustMarshalJSON(ftResponse{OK: false, Error: te}))
}

func mustMarshalJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

// sshCarrier 支持导出底层 *ssh.Client 的连接（sshclient.Client 实现；
// telnet 不实现，SFTP 请求对其返回明确错误）。
type sshCarrier interface{ SSHClient() *ssh.Client }

// ftHub FT 域：目标解析（终端会话 / 树节点 → SFTP transport）+ 异步任务队列。
type ftHub struct {
	svc  *Service
	emit func(event string, params any)

	mu     sync.Mutex
	tasks  map[string]*ftTask
	perKey map[string]chan struct{}
	total  chan struct{}
}

const (
	ftPerTargetLimit = 2
	ftTotalLimit     = 6
)

type ftTask struct {
	id        string
	targetKey string
	cancel    context.CancelFunc
}

func newFtHub(svc *Service, emit func(string, any)) *ftHub {
	return &ftHub{svc: svc, emit: emit, tasks: map[string]*ftTask{}, perKey: map[string]chan struct{}{}, total: make(chan struct{}, ftTotalLimit)}
}

// transportFor 解析目标：终端会话 ID → 独立连接；否则树节点 ID → 共享池
// 连接（惰性 Connect）。返回即用即关的 SFTP transport。
func (h *ftHub) transportFor(target string) (*filetransfer.SFTPTransport, string, error) {
	if target == "" {
		return nil, "", errors.New("缺少目标会话或服务器")
	}
	// 终端会话优先：其独立连接专用于本会话的传输
	if sess := h.svc.terminals.get(target); sess != nil {
		sess.mu.Lock()
		conn, alive := sess.conn, sess.alive
		sess.mu.Unlock()
		if !alive || conn == nil {
			return nil, target, errors.New("终端会话已断开，请先重连")
		}
		return h.transportFromConn(conn, target)
	}
	// 树节点 → 共享池
	mgr, host, err := h.svc.resolveServer(target)
	if err != nil {
		return nil, target, err
	}
	if _, err := mgr.Connect(host); err != nil {
		return nil, target, err
	}
	conn := mgr.GetConnection(host)
	if conn == nil {
		return nil, target, errors.New("连接不可用")
	}
	return h.transportFromConn(conn, target)
}

func (h *ftHub) transportFromConn(conn *ops.Connection, key string) (*filetransfer.SFTPTransport, string, error) {
	carrier, ok := conn.Conn.(sshCarrier)
	if !ok {
		return nil, key, errors.New("当前协议不支持 SFTP 文件传输")
	}
	return filetransfer.NewSFTPTransport(carrier.SSHClient()), key, nil
}

// start 异步任务：注册 + 立即返回 taskId；排队与执行经事件回报。
func (h *ftHub) start(target, kind, localPath, remotePath string) string {
	id := uuid.NewString()
	ctx, cancel := context.WithCancel(context.Background())
	h.mu.Lock()
	h.tasks[id] = &ftTask{id: id, targetKey: target, cancel: cancel}
	slot := h.perKey[target]
	if slot == nil {
		slot = make(chan struct{}, ftPerTargetLimit)
		h.perKey[target] = slot
	}
	h.mu.Unlock()
	go h.run(id, target, slot, ctx, kind, localPath, remotePath)
	return id
}

func (h *ftHub) run(id, target string, slot chan struct{}, ctx context.Context, kind, localPath, remotePath string) {
	defer h.finishTask(id)
	h.emitProgress(id, target, filetransfer.Progress{Step: "排队等待其他传输完成..."})
	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		h.emitDone(id, target, "传输已取消", true)
		return
	}
	defer func() { <-slot }()
	select {
	case h.total <- struct{}{}:
	case <-ctx.Done():
		h.emitDone(id, target, "传输已取消", true)
		return
	}
	defer func() { <-h.total }()

	transport, _, err := h.transportFor(target)
	if err != nil {
		h.emitDone(id, target, err.Error(), false)
		return
	}
	h.emitProgress(id, target, filetransfer.Progress{Step: "连接传输通道..."})

	var files int64
	var bytes int64
	progress := h.throttledProgress(id, target)
	switch kind {
	case "upload":
		if isDir(localPath) {
			files, bytes, err = h.uploadDir(ctx, transport, localPath, remotePath, progress)
		} else {
			files, bytes, err = 1, 0, nil
			var res filetransfer.TransferResult
			res, err = transport.Upload(ctx, localPath, remotePath, progress)
			bytes = res.Bytes
		}
	case "download":
		stat, err := transport.Stat(ctx, remotePath)
		if err == nil && stat.IsDir {
			files, bytes, err = h.downloadDir(ctx, transport, remotePath, localPath, progress)
		} else if err == nil {
			files = 1
			var res filetransfer.TransferResult
			res, err = transport.Download(ctx, remotePath, localPath, progress)
			bytes = res.Bytes
		}
	default:
		err = fmt.Errorf("未知传输类型: %s", kind)
	}
	if err != nil {
		cancelled := errors.Is(err, context.Canceled) || ctx.Err() != nil
		h.emitDone(id, target, err.Error(), cancelled)
		return
	}
	h.emit(EventTransferDone, map[string]any{
		"taskId": id, "sessionId": target, "ok": true,
		"files": files, "bytes": bytes,
	})
}

// throttledProgress 100ms 节流 + 速度计算（对齐 ops 前端的进度展示节奏）。
func (h *ftHub) throttledProgress(id, target string) func(filetransfer.Progress) {
	var mu sync.Mutex
	var last time.Time
	var lastBytes int64
	return func(p filetransfer.Progress) {
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		if p.Step == "" && now.Sub(last) < 100*time.Millisecond {
			return
		}
		if p.Step == "" && lastBytes > 0 {
			if dt := now.Sub(last).Seconds(); dt > 0 {
				p.SpeedBps = int64(float64(p.BytesDone-lastBytes)/dt) | p.SpeedBps
			}
		}
		last, lastBytes = now, p.BytesDone
		h.emitProgress(id, target, p)
	}
}

func (h *ftHub) emitProgress(id, target string, p filetransfer.Progress) {
	payload := map[string]any{
		"taskId": id, "sessionId": target,
		"bytesDone": p.BytesDone, "bytesTotal": p.BytesTotal, "speedBps": p.SpeedBps,
	}
	if p.Step != "" {
		payload["step"] = p.Step
	}
	h.emit(EventTransferProgress, payload)
}

func (h *ftHub) emitDone(id, target, message string, cancelled bool) {
	payload := map[string]any{"taskId": id, "sessionId": target, "ok": false, "message": message}
	if cancelled {
		payload["cancelled"] = true
	}
	h.emit(EventTransferDone, payload)
}

func (h *ftHub) cancel(id string) string {
	h.mu.Lock()
	task := h.tasks[id]
	h.mu.Unlock()
	if task == nil {
		return ftMessage("任务不存在或已结束")
	}
	task.cancel()
	return ftOK()
}

func (h *ftHub) finishTask(id string) {
	h.mu.Lock()
	delete(h.tasks, id)
	h.mu.Unlock()
}

func (h *ftHub) cancelAll() {
	h.mu.Lock()
	tasks := make([]*ftTask, 0, len(h.tasks))
	for _, t := range h.tasks {
		tasks = append(tasks, t)
	}
	h.mu.Unlock()
	for _, t := range tasks {
		t.cancel()
	}
}

// ---- 目录递归（上传 walk 本地 / 下载 walk 远端，逐文件走 transport） ----

func (h *ftHub) uploadDir(ctx context.Context, t *filetransfer.SFTPTransport, localRoot, remoteRoot string, progress func(filetransfer.Progress)) (int64, int64, error) {
	var total, bytes int64
	// 先收集文件与总大小（进度分母）
	type item struct{ local, remote string }
	var items []item
	var sum int64
	err := filepath.WalkDir(localRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(localRoot, path)
		items = append(items, item{local: path, remote: pathJoinRemote(remoteRoot, filepath.ToSlash(rel))})
		sum += info.Size()
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	var done int64
	for _, it := range items {
		if ctx.Err() != nil {
			return total, bytes, ctx.Err()
		}
		base := done
		res, err := t.Upload(ctx, it.local, it.remote, func(p filetransfer.Progress) {
			progress(filetransfer.Progress{BytesDone: base + p.BytesDone, BytesTotal: sum})
		})
		if err != nil {
			return total, bytes, err
		}
		done += res.Bytes
		bytes += res.Bytes
		total++
	}
	return total, bytes, nil
}

func (h *ftHub) downloadDir(ctx context.Context, t *filetransfer.SFTPTransport, remoteRoot, localRoot string, progress func(filetransfer.Progress)) (int64, int64, error) {
	var total, bytes int64
	var walk func(dir, localDir string) error
	walk = func(dir, localDir string) error {
		entries, err := t.List(ctx, dir)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(localDir, 0o755); err != nil {
			return err
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if entry.IsDir {
				if err := walk(entry.Path, filepath.Join(localDir, entry.Name)); err != nil {
					return err
				}
				continue
			}
			res, err := t.Download(ctx, entry.Path, filepath.Join(localDir, entry.Name), func(p filetransfer.Progress) {
				progress(p)
			})
			if err != nil {
				return err
			}
			bytes += res.Bytes
			total++
		}
		return nil
	}
	if err := walk(remoteRoot, localRoot); err != nil {
		return total, bytes, err
	}
	return total, bytes, nil
}

func pathJoinRemote(base, rel string) string {
	clean := strings.TrimSuffix(strings.TrimPrefix(rel, "./"), "/")
	if clean == "" || clean == "." {
		return base
	}
	return strings.TrimSuffix(base, "/") + "/" + clean
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ---- 本地文件系（FilesPanel 本地栏；语义对齐 ops App.Local*，无远端闸门） ----

func localEntry(info os.FileInfo, path string) *filetransfer.Entry {
	return &filetransfer.Entry{
		Path: filepath.ToSlash(path), Name: info.Name(), IsDir: info.IsDir(),
		Size: info.Size(), Mode: uint32(info.Mode().Perm()), ModTime: info.ModTime(),
	}
}

func localList(root string) string {
	p := strings.TrimSpace(root)
	if p == "" || p == "." {
		if home, err := os.UserHomeDir(); err == nil {
			p = home
		} else {
			p = "."
		}
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return ftErr(err)
	}
	list := []filetransfer.Entry{}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		list = append(list, *localEntry(info, filepath.Join(p, e.Name())))
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].IsDir != list[j].IsDir {
			return list[i].IsDir
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	return ftEntries(list)
}

func localMkdir(path string) string {
	if err := os.MkdirAll(strings.TrimSpace(path), 0o755); err != nil {
		return ftErr(err)
	}
	return ftOK()
}

func localRemove(path string) string {
	if err := os.RemoveAll(strings.TrimSpace(path)); err != nil {
		return ftErr(err)
	}
	return ftOK()
}

func localRename(oldPath, newPath string) string {
	if err := os.Rename(strings.TrimSpace(oldPath), strings.TrimSpace(newPath)); err != nil {
		return ftErr(err)
	}
	return ftOK()
}

func localCopy(src, dst string) string {
	src, dst = strings.TrimSpace(src), strings.TrimSpace(dst)
	info, err := os.Stat(src)
	if err != nil {
		return ftErr(err)
	}
	if info.IsDir() {
		return ftErr(errors.New("暂不支持复制目录"))
	}
	in, err := os.Open(src)
	if err != nil {
		return ftErr(err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return ftErr(err)
	}
	out, err := os.Create(dst)
	if err != nil {
		return ftErr(err)
	}
	defer out.Close()
	written, err := io.Copy(out, in)
	if err != nil {
		return ftErr(err)
	}
	return ftResult(&filetransfer.TransferResult{Bytes: written})
}

func localStat(path string) string {
	info, err := os.Stat(strings.TrimSpace(path))
	if err != nil {
		return ftErr(err)
	}
	return ftEntry(localEntry(info, strings.TrimSpace(path)))
}

// unused guard：connectionstore 类型引用（resolveServer 返回错误使用处）。
var _ = connectionstore.KindConnection

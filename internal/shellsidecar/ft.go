package shellsidecar

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"

	"opscopilot/pkg/filetransfer"
	"opscopilot/pkg/remote"
)

// FTService：共享 FilesPanel 的 sidecar 后端。
// 方法集与 Wails app.go 的 FT*/Local* 一一对应（FTCheck/FTList/…/LocalList/…），
// RPC 层（shell.ft.* / shell.fs.*）统一返回 ftEnvelope，前端适配器原样 JSON 化即可。
//
// 与 Wails 的差异（能力边界，由共享组件按可选能力隐藏入口）：
//   - "本地"文件系统 = sidecar 数据目录沙箱（浏览器宿主读不到用户 OS 路径）；
//   - 无 LocalCopy / SelectSavePath（无 OS 拖放与原生保存对话框）。
type FTService struct {
	mu       sync.Mutex
	svc      *TerminalService
	dataDir  string
	root     *os.Root
	notify   func(method string, params any)
	tasks    map[string]*ftTask
	limiters map[string]chan struct{}
}

// ftTask 记录进行中的传输任务（取消入口）。
type ftTask struct {
	id     string
	termID string
	cancel context.CancelFunc
}

// ftEnvelope 与 Wails ftResponse/localFSResponse 同构的 JSON 信封。
type ftEnvelope struct {
	OK      bool                         `json:"ok"`
	Message string                       `json:"message,omitempty"`
	Error   *filetransfer.TransferError  `json:"error,omitempty"`
	TaskID  string                       `json:"taskId,omitempty"`
	Entries []filetransfer.Entry         `json:"entries,omitempty"`
	Entry   *filetransfer.Entry          `json:"entry,omitempty"`
	Content string                       `json:"content,omitempty"`
	Result  *filetransfer.TransferResult `json:"result,omitempty"`
}

// ftErr 携带产品错误码（面板 formatError 识别 FILE_SIZE_EXCEEDED 等）。
type ftErr struct {
	code filetransfer.ErrorCode
	msg  string
}

func (e *ftErr) Error() string { return e.msg }

func ftOK() *ftEnvelope { return &ftEnvelope{OK: true} }
func ftFail(err error) *ftEnvelope {
	code := filetransfer.ErrorCodeUnknown
	if fe, ok := err.(*ftErr); ok {
		code = fe.code
	}
	te := &filetransfer.TransferError{Code: code, Message: err.Error()}
	return &ftEnvelope{OK: false, Message: err.Error(), Error: te}
}

const ftMaxConcurrentPerTerminal = 4

func NewFTService(svc *TerminalService, dataDir string) *FTService {
	return &FTService{svc: svc, dataDir: dataDir, tasks: map[string]*ftTask{}, limiters: map[string]chan struct{}{}}
}

func (s *FTService) SetNotify(fn func(method string, params any)) { s.notify = fn }

// ---- 工具 ----

func (s *FTService) sftpOf(terminalID string) (*sftp.Client, error) {
	conn, err := s.svc.ConnectionOfTerminal(terminalID)
	if err != nil {
		return nil, err
	}
	capable, ok := conn.(remote.SFTPCapable)
	if !ok {
		return nil, fmt.Errorf("当前连接不支持 SFTP")
	}
	return capable.SFTPClient()
}

// resolveLocal 把面板传入的本地路径限制在数据目录沙箱内。
// 空路径/./ 根 → 数据目录本身；绝对路径必须已位于数据目录内；相对路径拼接后校验。
func (s *FTService) resolveLocal(p string) (string, error) {
	if s.root != nil {
		return workspacePath(p)
	}
	root := filepath.Clean(s.dataDir)
	if p == "" || p == "." || p == "/" {
		return root, nil
	}
	clean := filepath.Clean(p)
	joined := clean
	if !filepath.IsAbs(clean) {
		joined = filepath.Join(root, clean)
	}
	if joined != root && !strings.HasPrefix(joined, root+string(filepath.Separator)) {
		return "", fmt.Errorf("路径超出数据目录范围: %s", p)
	}
	return joined, nil
}

func localEntry(path string, info os.FileInfo) filetransfer.Entry {
	e := filetransfer.Entry{
		Path:    filepath.ToSlash(path),
		Name:    info.Name(),
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		Mode:    uint32(info.Mode()),
		ModTime: info.ModTime(),
	}
	return e
}

// ---- 远端操作（同步，SFTP） ----

// Check 探测该终端连接的传输协议。sidecar 仅支持 SFTP 子系统。
func (s *FTService) Check(terminalID string) *ftEnvelope {
	c, err := s.sftpOf(terminalID)
	if err != nil {
		return ftFail(err)
	}
	defer c.Close()
	return &ftEnvelope{OK: true, Message: "sftp(login)"}
}

func (s *FTService) List(terminalID, remotePath string) *ftEnvelope {
	c, err := s.sftpOf(terminalID)
	if err != nil {
		return ftFail(err)
	}
	defer c.Close()
	infos, err := c.ReadDir(remotePath)
	if err != nil {
		return ftFail(err)
	}
	entries := make([]filetransfer.Entry, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, remoteEntry(remotePath, info))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return &ftEnvelope{OK: true, Entries: entries}
}

func (s *FTService) Stat(terminalID, remotePath string) *ftEnvelope {
	c, err := s.sftpOf(terminalID)
	if err != nil {
		return ftFail(err)
	}
	defer c.Close()
	info, err := c.Stat(remotePath)
	if err != nil {
		// 面板用 ok=false 表示"不存在"，非错误
		return &ftEnvelope{OK: false, Message: err.Error()}
	}
	e := remoteEntry(remotePath, info)
	return &ftEnvelope{OK: true, Entry: &e}
}

func (s *FTService) RemoteMkdir(terminalID, remotePath string) *ftEnvelope {
	c, err := s.sftpOf(terminalID)
	if err != nil {
		return ftFail(err)
	}
	defer c.Close()
	if err := c.MkdirAll(remotePath); err != nil {
		return ftFail(err)
	}
	return ftOK()
}

func (s *FTService) RemoteRemove(terminalID, remotePath string) *ftEnvelope {
	c, err := s.sftpOf(terminalID)
	if err != nil {
		return ftFail(err)
	}
	defer c.Close()
	if err := c.Remove(remotePath); err == nil {
		return ftOK()
	}
	// 目录（含非空）走递归删除
	if err := s.removeRemoteRecursive(c, remotePath); err != nil {
		return ftFail(err)
	}
	return ftOK()
}

func (s *FTService) removeRemoteRecursive(c *sftp.Client, remotePath string) error {
	info, err := c.Lstat(remotePath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return c.Remove(remotePath)
	}
	infos, err := c.ReadDir(remotePath)
	if err != nil {
		return err
	}
	for _, info := range infos {
		child := strings.TrimSuffix(remotePath, "/") + "/" + info.Name()
		if err := s.removeRemoteRecursive(c, child); err != nil {
			return err
		}
	}
	return c.RemoveDirectory(remotePath)
}

func (s *FTService) RemoteRename(terminalID, oldPath, newPath string) *ftEnvelope {
	c, err := s.sftpOf(terminalID)
	if err != nil {
		return ftFail(err)
	}
	defer c.Close()
	// 优先 POSIX rename（覆盖目标），退化到基本 rename
	if err := c.PosixRename(oldPath, newPath); err != nil {
		if err := c.Rename(oldPath, newPath); err != nil {
			return ftFail(err)
		}
	}
	return ftOK()
}

const ftReadFileMax = 256 * 1024

func (s *FTService) RemoteReadFile(terminalID, remotePath string, maxBytes int64) *ftEnvelope {
	c, err := s.sftpOf(terminalID)
	if err != nil {
		return ftFail(err)
	}
	defer c.Close()
	if maxBytes <= 0 {
		maxBytes = ftReadFileMax
	}
	info, err := c.Stat(remotePath)
	if err != nil {
		return ftFail(err)
	}
	if info.Size() > maxBytes {
		return ftFail(&ftErr{code: filetransfer.ErrorCodeFileSizeExceeded, msg: "文件超出可编辑大小上限"})
	}
	f, err := c.Open(remotePath)
	if err != nil {
		return ftFail(err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return ftFail(err)
	}
	return &ftEnvelope{OK: true, Content: string(data), Result: &filetransfer.TransferResult{Bytes: int64(len(data))}}
}

func (s *FTService) RemoteWriteFile(terminalID, remotePath, content string) *ftEnvelope {
	c, err := s.sftpOf(terminalID)
	if err != nil {
		return ftFail(err)
	}
	defer c.Close()
	f, err := c.Create(remotePath)
	if err != nil {
		return ftFail(err)
	}
	defer f.Close()
	if _, err := f.Write([]byte(content)); err != nil {
		return ftFail(err)
	}
	return ftOK()
}

// ---- 本地操作（数据目录沙箱） ----

func (s *FTService) LocalList(path string) *ftEnvelope {
	dir, err := s.resolveLocal(path)
	if err != nil {
		return ftFail(err)
	}
	f, err := s.localOpen(dir, os.O_RDONLY)
	if err != nil {
		return ftFail(err)
	}
	defer f.Close()
	infos, err := f.ReadDir(-1)
	if err != nil {
		return ftFail(err)
	}
	entries := make([]filetransfer.Entry, 0, len(infos))
	for _, info := range infos {
		full := filepath.Join(dir, info.Name())
		fi, err := info.Info()
		if err != nil {
			continue
		}
		entries = append(entries, localEntry(full, fi))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return &ftEnvelope{OK: true, Entries: entries}
}

func (s *FTService) LocalStat(path string) *ftEnvelope {
	abs, err := s.resolveLocal(path)
	if err != nil {
		return ftFail(err)
	}
	info, err := s.localStat(abs)
	if err != nil {
		// ok=false 表示不存在
		return &ftEnvelope{OK: false, Message: err.Error()}
	}
	e := localEntry(abs, info)
	return &ftEnvelope{OK: true, Entry: &e}
}

func (s *FTService) LocalMkdir(path string) *ftEnvelope {
	abs, err := s.resolveLocal(path)
	if err != nil {
		return ftFail(err)
	}
	if err := s.localMkdirAll(abs); err != nil {
		return ftFail(err)
	}
	return ftOK()
}

func (s *FTService) LocalRemove(path string) *ftEnvelope {
	abs, err := s.resolveLocal(path)
	if err != nil {
		return ftFail(err)
	}
	if abs == filepath.Clean(s.dataDir) {
		return ftFail(fmt.Errorf("不能删除数据目录本身"))
	}
	if err := s.localRemoveAll(abs); err != nil {
		return ftFail(err)
	}
	return ftOK()
}

func (s *FTService) LocalRename(oldPath, newPath string) *ftEnvelope {
	oldAbs, err := s.resolveLocal(oldPath)
	if err != nil {
		return ftFail(err)
	}
	newAbs, err := s.resolveLocal(newPath)
	if err != nil {
		return ftFail(err)
	}
	if err := s.localRename(oldAbs, newAbs); err != nil {
		return ftFail(err)
	}
	return ftOK()
}

// ---- 异步传输任务（限流 + 取消 + 进度/完成事件） ----

// Upload 从数据目录沙箱内文件上传到远端。
func (s *FTService) Upload(terminalID, localPath, remotePath string) *ftEnvelope {
	localAbs, err := s.resolveLocal(localPath)
	if err != nil {
		return ftFail(err)
	}
	info, err := s.localStat(localAbs)
	if err != nil {
		return ftFail(err)
	}
	if info.IsDir() {
		return ftFail(&ftErr{code: filetransfer.ErrorCodeNotSupported, msg: "暂不支持目录上传，请先压缩"})
	}
	taskID := "ft-" + newID()
	ctx, cancel := context.WithCancel(context.Background())
	s.registerTask(taskID, terminalID, cancel)
	go s.runTask(ctx, taskID, terminalID, info.Size(), func(c *sftp.Client, progress func(filetransfer.Progress)) error {
		src, err := s.localOpen(localAbs, os.O_RDONLY)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := c.Create(remotePath)
		if err != nil {
			return err
		}
		defer dst.Close()
		_, err = copyFT(ctx, dst, src, info.Size(), progress)
		return err
	})
	return &ftEnvelope{OK: true, TaskID: taskID}
}

// Download 下载远端文件到数据目录沙箱内。
func (s *FTService) Download(terminalID, remotePath, localPath string) *ftEnvelope {
	localAbs, err := s.resolveLocal(localPath)
	if err != nil {
		return ftFail(err)
	}
	if dir := filepath.Dir(localAbs); dir != "" {
		if err := s.localMkdirAll(dir); err != nil {
			return ftFail(err)
		}
	}
	taskID := "ft-" + newID()
	ctx, cancel := context.WithCancel(context.Background())
	s.registerTask(taskID, terminalID, cancel)
	go s.runTask(ctx, taskID, terminalID, -1, func(c *sftp.Client, progress func(filetransfer.Progress)) error {
		src, err := c.Open(remotePath)
		if err != nil {
			return err
		}
		defer src.Close()
		info, err := src.Stat()
		if err != nil {
			return err
		}
		dst, err := s.localOpen(localAbs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
		if err != nil {
			return err
		}
		defer dst.Close()
		_, err = copyFT(ctx, dst, src, info.Size(), progress)
		return err
	})
	return &ftEnvelope{OK: true, TaskID: taskID}
}

func (s *FTService) Cancel(taskID string) *ftEnvelope {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	s.mu.Unlock()
	if !ok {
		return ftFail(fmt.Errorf("任务不存在"))
	}
	task.cancel()
	return ftOK()
}

func (s *FTService) registerTask(id, termID string, cancel context.CancelFunc) {
	s.mu.Lock()
	s.tasks[id] = &ftTask{id: id, termID: termID, cancel: cancel}
	s.mu.Unlock()
}

func (s *FTService) limiterFor(termID string) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.limiters[termID]
	if !ok {
		ch = make(chan struct{}, ftMaxConcurrentPerTerminal)
		s.limiters[termID] = ch
	}
	return ch
}

// runTask 统一承载排队/执行/事件语义，与 Wails startFileTransferTask 对齐：
// 排队 → step 事件；拿到槽位 → 字节进度（空 step 清除提示）；结束 → done 事件。
func (s *FTService) runTask(ctx context.Context, taskID, termID string, total int64, op func(*sftp.Client, func(filetransfer.Progress)) error) {
	limiter := s.limiterFor(termID)
	if len(limiter) >= cap(limiter) {
		s.emitProgress(taskID, termID, filetransfer.Progress{Step: "排队等待其他传输完成..."})
	}
	select {
	case limiter <- struct{}{}:
	case <-ctx.Done():
		s.finishTask(taskID)
		s.notifyDone(taskID, termID, false, true, "已取消", 0)
		return
	}
	defer func() { <-limiter }()

	if err := ctx.Err(); err != nil {
		s.finishTask(taskID)
		s.notifyDone(taskID, termID, false, true, "已取消", 0)
		return
	}

	c, err := s.sftpOf(termID)
	if err != nil {
		s.finishTask(taskID)
		s.notifyDone(taskID, termID, false, false, err.Error(), 0)
		return
	}
	defer c.Close()

	var copied int64
	// Closing the transfer's SFTP client interrupts a blocked read/write on cancel.
	stopCancel := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stopCancel()
	progress := func(p filetransfer.Progress) {
		if total >= 0 {
			p.BytesTotal = total
		}
		copied = p.BytesDone
		s.emitProgress(taskID, termID, p)
	}
	// 拿到槽位后首个字节进度会显式带空 step，清除排队提示（与 Wails 语义一致）
	s.emitProgress(taskID, termID, filetransfer.Progress{BytesTotal: total, Step: ""})

	if err := op(c, progress); err != nil {
		s.finishTask(taskID)
		cancelled := ctx.Err() != nil || strings.Contains(err.Error(), "取消")
		msg := err.Error()
		if cancelled {
			msg = "已取消"
		}
		s.notifyDone(taskID, termID, false, cancelled, msg, copied)
		return
	}
	s.finishTask(taskID)
	s.notifyDone(taskID, termID, true, false, "完成 (sftp(login))", copied)
}

func (s *FTService) finishTask(taskID string) {
	s.mu.Lock()
	delete(s.tasks, taskID)
	s.mu.Unlock()
}

func (s *FTService) emitProgress(taskID, termID string, p filetransfer.Progress) {
	if s.notify == nil {
		return
	}
	s.notify("shell.ft/progress", map[string]any{
		"taskId":     taskID,
		"sessionId":  termID,
		"bytesDone":  p.BytesDone,
		"bytesTotal": p.BytesTotal,
		"speedBps":   p.SpeedBps,
		"step":       p.Step,
	})
}

func (s *FTService) notifyDone(taskID, termID string, ok, cancelled bool, message string, bytes int64) {
	if s.notify == nil {
		return
	}
	s.notify("shell.ft/done", map[string]any{
		"taskId":    taskID,
		"sessionId": termID,
		"ok":        ok,
		"cancelled": cancelled,
		"message":   message,
		"bytes":     bytes,
	})
}

// copyFT 带取消与节流进度的拷贝（对齐 pkg/filetransfer copyWithProgress 语义；
// 该函数未导出，sidecar 侧实现同节奏：200ms 节流 + EOF 终报 + ctx 取消）。
func copyFT(ctx context.Context, dst io.Writer, src io.Reader, total int64, progress func(filetransfer.Progress)) (int64, error) {
	buf := make([]byte, 128*1024)
	var done int64
	start := time.Now()
	last := start
	report := func(force bool) {
		if !force && time.Since(last) < 200*time.Millisecond {
			return
		}
		secs := time.Since(start).Seconds()
		var speed int64
		if secs > 0 {
			speed = int64(float64(done) / secs)
		}
		progress(filetransfer.Progress{BytesDone: done, BytesTotal: total, SpeedBps: speed, Step: ""})
		last = time.Now()
	}
	for {
		if err := ctx.Err(); err != nil {
			return done, fmt.Errorf("传输已取消")
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return done, werr
			}
			done += int64(n)
			report(false)
		}
		if rerr == io.EOF {
			report(true)
			return done, nil
		}
		if rerr != nil {
			return done, rerr
		}
	}
}

func remoteEntry(dir string, info os.FileInfo) filetransfer.Entry {
	name := info.Name()
	path := name
	if dir != "" && dir != "." {
		path = strings.TrimSuffix(dir, "/") + "/" + name
	}
	return filetransfer.Entry{
		Path:    path,
		Name:    name,
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		Mode:    uint32(info.Mode()),
		ModTime: info.ModTime(),
	}
}

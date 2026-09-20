// tree.go 目录级传输的通用编排层。
//
// 设计约束（与产品决策一致，见会话记录）：
//   - 目标已存在同名目录 → 合并：目录并入、同名文件覆盖，不逐文件弹确认
//     （发起前的"一次性确认"由 UI 层负责，传输层只管合并语义）。
//   - 不保留权限/mtime/属主：文件落到目标侧的默认属性。
//   - 空目录也要建出来；本地符号链接跳过（防环），并记录在结果里。
//   - 单文件失败不中断整树：收集失败清单，结束后随结果上报
//     （中转模式的原子性见 RootRelayTransport.UploadTree）。
//
// 编排层不感知传输方式：调用方注入 Mkdir/UploadFile（或 List/DownloadFile）
// 即可，SFTP 直连、Root 中转、Base64 直传三种模式的差异全部留在调用方。
package filetransfer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxTreeDepth 目录递归深度上限，防御异常深树与符号链接环。
const maxTreeDepth = 40

// TreeFileFailure 单个文件传输失败记录。
type TreeFileFailure struct {
	Path string `json:"path"`
	Err  string `json:"err"`
}

// TreeResult 目录传输汇总。
type TreeResult struct {
	Dirs     int              `json:"dirs"` // 建立/枚举的目录数（含根）
	Files    int              `json:"files"`
	Bytes    int64            `json:"bytes"`
	Failures []TreeFileFailure `json:"failures,omitempty"`
	// Transport 实际使用的方式（用户可读标签），空由调用方按会话语义命名。
	Transport string `json:"transport,omitempty"`
}

// LocalTreeFile 本地待传文件（相对路径 + 绝对路径 + 大小）。
type LocalTreeFile struct {
	RelPath  string
	AbsPath  string
	Size     int64
}

// LocalTreePlan 本地目录扫描结果。
type LocalTreePlan struct {
	// RelDirs 相对子目录（不含根），父目录排在前，空目录也包含。
	RelDirs []string
	Files   []LocalTreeFile
	// SkippedLinks 被跳过的符号链接（相对路径）。
	SkippedLinks []string
}

// RemoteTreeFile 远端待下载文件。
type RemoteTreeFile struct {
	RelPath string
	AbsPath string
	Size    int64
}

// RemoteTreePlan 远端目录枚举结果。
type RemoteTreePlan struct {
	RelDirs []string
	Files   []RemoteTreeFile
}

// WalkLocalTree 递归扫描本地目录，生成传输计划。
// 符号链接一律跳过（无论指向文件还是目录），避免环与"传输内容与所见不一致"。
func WalkLocalTree(root string) (LocalTreePlan, error) {
	root = filepath.Clean(root)
	st, err := os.Stat(root)
	if err != nil {
		return LocalTreePlan{}, toTransferError(err)
	}
	if !st.IsDir() {
		return LocalTreePlan{}, &TransferError{Code: ErrorCodeUnknown, Message: fmt.Sprintf("不是本地目录: %s", root)}
	}

	var plan LocalTreePlan
	var walk func(dir, rel string, depth int) error
	walk = func(dir, rel string, depth int) error {
		if depth > maxTreeDepth {
			return &TransferError{Code: ErrorCodeUnknown, Message: fmt.Sprintf("目录层级超过 %d 层，中止扫描: %s", maxTreeDepth, dir)}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return toTransferError(err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, e := range entries {
			name := e.Name()
			childRel := name
			if rel != "" {
				childRel = rel + "/" + name
			}
			// 符号链接：跳过（os.ReadDir 的 Type() 是 lstat 语义，可靠识别链接本身）。
			if e.Type()&fs.ModeSymlink != 0 {
				plan.SkippedLinks = append(plan.SkippedLinks, childRel)
				continue
			}
			if e.IsDir() {
				plan.RelDirs = append(plan.RelDirs, childRel)
				if err := walk(filepath.Join(dir, name), childRel, depth+1); err != nil {
					return err
				}
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue // 条目消失，跳过
			}
			plan.Files = append(plan.Files, LocalTreeFile{
				RelPath: childRel,
				AbsPath: filepath.Join(dir, name),
				Size:    info.Size(),
			})
		}
		return nil
	}

	if err := walk(root, "", 0); err != nil {
		return LocalTreePlan{}, err
	}
	return plan, nil
}

// WalkRemoteTree 通过注入的 List 递归枚举远端目录。
func WalkRemoteTree(ctx context.Context, root string, list func(ctx context.Context, remotePath string) ([]Entry, error)) (RemoteTreePlan, error) {
	var plan RemoteTreePlan
	var walk func(dir, rel string, depth int) error
	walk = func(dir, rel string, depth int) error {
		if depth > maxTreeDepth {
			return &TransferError{Code: ErrorCodeUnknown, Message: fmt.Sprintf("目录层级超过 %d 层，中止枚举: %s", maxTreeDepth, dir)}
		}
		entries, err := list(ctx, dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Name == "" || e.Name == "." || e.Name == ".." {
				continue
			}
			childRel := e.Name
			if rel != "" {
				childRel = rel + "/" + e.Name
			}
			if e.IsDir {
				plan.RelDirs = append(plan.RelDirs, childRel)
				if err := walk(e.Path, childRel, depth+1); err != nil {
					return err
				}
				continue
			}
			plan.Files = append(plan.Files, RemoteTreeFile{RelPath: childRel, AbsPath: e.Path, Size: e.Size})
		}
		return nil
	}

	if err := walk(root, "", 0); err != nil {
		return RemoteTreePlan{}, err
	}
	return plan, nil
}

// UploadTreeOps 上传编排所需的传输原语。
type UploadTreeOps struct {
	// Mkdir 建远端目录；已存在不得报错（合并语义）。
	Mkdir func(ctx context.Context, remoteDir string) error
	// UploadFile 上传单个文件到远端绝对路径。
	UploadFile func(ctx context.Context, localPath, remotePath string, progress func(Progress)) (TransferResult, error)
	// OnPlan 扫描完成、传输开始前的预检钩子（如 Base64 模式的单文件上限检查）。
	OnPlan func(plan LocalTreePlan) error
}

// UploadTree 把 localDir 整树上传到 remoteDir（合并语义）。
// 单文件失败收集到 Failures 并继续；ctx 取消立即中止并返回 ctx.Err()。
func UploadTree(ctx context.Context, localDir, remoteDir string, emit func(Progress), ops UploadTreeOps) (TreeResult, error) {
	if ops.Mkdir == nil || ops.UploadFile == nil {
		return TreeResult{}, &TransferError{Code: ErrorCodeUnknown, Message: "UploadTreeOps 未提供传输原语"}
	}

	emitStep(emit, "正在扫描本地目录...")
	plan, err := WalkLocalTree(localDir)
	if err != nil {
		return TreeResult{}, err
	}
	if ops.OnPlan != nil {
		if err := ops.OnPlan(plan); err != nil {
			return TreeResult{}, err
		}
	}

	var totalBytes int64
	for _, f := range plan.Files {
		totalBytes += f.Size
	}
	emitStep(emit, fmt.Sprintf("共 %d 个文件（%s）、%d 个子目录", len(plan.Files), HumanBytes(totalBytes), len(plan.RelDirs)))

	tp := newTreeProgress(emit, totalBytes, len(plan.Files))
	result := TreeResult{Dirs: len(plan.RelDirs) + 1}

	// 建目录：根 + 子目录（walk 保证父在前，逐层 mkdir -p 语义）。
	emitStep(emit, "正在创建远端目录...")
	if err := ops.Mkdir(ctx, normalizeRemotePath(remoteDir)); err != nil {
		return result, err
	}
	for _, d := range plan.RelDirs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := ops.Mkdir(ctx, joinRemote(remoteDir, filepath.ToSlash(d))); err != nil {
			return result, err
		}
	}

	for _, f := range plan.Files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		tp.nextFile(f.RelPath)
		res, err := ops.UploadFile(ctx, f.AbsPath, joinRemote(remoteDir, filepath.ToSlash(f.RelPath)), tp.child())
		if err != nil {
			result.Failures = append(result.Failures, TreeFileFailure{Path: f.RelPath, Err: err.Error()})
			tp.skipFile(f.Size)
			continue
		}
		result.Files++
		result.Bytes += res.Bytes
		tp.fileDone(f.Size)
	}
	return result, nil
}

// DownloadTreeOps 下载编排所需的传输原语。
type DownloadTreeOps struct {
	List         func(ctx context.Context, remotePath string) ([]Entry, error)
	DownloadFile func(ctx context.Context, remotePath, localPath string, progress func(Progress)) (TransferResult, error)
	// OnPlan 枚举完成、传输开始前的预检钩子。
	OnPlan func(plan RemoteTreePlan) error
}

// DownloadTree 把 remoteDir 整树下载到 localDir（合并语义，同名文件覆盖）。
func DownloadTree(ctx context.Context, remoteDir, localDir string, emit func(Progress), ops DownloadTreeOps) (TreeResult, error) {
	if ops.List == nil || ops.DownloadFile == nil {
		return TreeResult{}, &TransferError{Code: ErrorCodeUnknown, Message: "DownloadTreeOps 未提供传输原语"}
	}

	emitStep(emit, "正在枚举远端目录...")
	plan, err := WalkRemoteTree(ctx, normalizeRemotePath(remoteDir), ops.List)
	if err != nil {
		return TreeResult{}, err
	}
	if ops.OnPlan != nil {
		if err := ops.OnPlan(plan); err != nil {
			return TreeResult{}, err
		}
	}

	var totalBytes int64
	for _, f := range plan.Files {
		totalBytes += f.Size
	}
	emitStep(emit, fmt.Sprintf("共 %d 个文件（%s）、%d 个子目录", len(plan.Files), HumanBytes(totalBytes), len(plan.RelDirs)))

	tp := newTreeProgress(emit, totalBytes, len(plan.Files))
	result := TreeResult{Dirs: len(plan.RelDirs) + 1}

	emitStep(emit, "正在创建本地目录...")
	lp := filepath.Clean(localDir)
	if err := os.MkdirAll(lp, 0o755); err != nil {
		return result, toTransferError(err)
	}
	for _, d := range plan.RelDirs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := os.MkdirAll(filepath.Join(lp, filepath.FromSlash(d)), 0o755); err != nil {
			return result, toTransferError(err)
		}
	}

	for _, f := range plan.Files {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		tp.nextFile(f.RelPath)
		res, err := ops.DownloadFile(ctx, f.AbsPath, filepath.Join(lp, filepath.FromSlash(f.RelPath)), tp.child())
		if err != nil {
			result.Failures = append(result.Failures, TreeFileFailure{Path: f.RelPath, Err: err.Error()})
			tp.skipFile(f.Size)
			continue
		}
		result.Files++
		result.Bytes += res.Bytes
		tp.fileDone(f.Size)
	}
	return result, nil
}

// treeProgress 把单文件进度映射为整树字节进度。
// 步骤文案（Step）带 (当前/总数) 前缀透传，字节进度不带步骤——与任务层的
// "Step 与字节互斥"约定一致。
type treeProgress struct {
	emit       func(Progress)
	totalBytes int64
	doneBytes  int64 // 已完成（或已跳过）文件的累计字节
	filesTotal int
	filesDone  int
}

func newTreeProgress(emit func(Progress), totalBytes int64, filesTotal int) *treeProgress {
	return &treeProgress{emit: emit, totalBytes: totalBytes, filesTotal: filesTotal}
}

func (tp *treeProgress) nextFile(rel string) {
	if tp.emit == nil {
		return
	}
	tp.emit(Progress{Step: fmt.Sprintf("正在传输 %s (%d/%d)", rel, tp.filesDone+1, tp.filesTotal)})
}

// child 返回当前文件的单文件进度函数：字节映射为全树进度。
func (tp *treeProgress) child() func(Progress) {
	return func(p Progress) {
		if tp.emit == nil {
			return
		}
		if p.Step != "" {
			tp.emit(Progress{Step: fmt.Sprintf("(%d/%d) %s", tp.filesDone+1, tp.filesTotal, p.Step)})
			return
		}
		if p.BytesDone > 0 || p.BytesTotal > 0 {
			tp.emit(Progress{
				BytesDone:  tp.doneBytes + p.BytesDone,
				BytesTotal: tp.totalBytes,
				SpeedBps:   p.SpeedBps,
			})
		}
	}
}

func (tp *treeProgress) fileDone(size int64) {
	tp.doneBytes += size
	tp.filesDone++
}

func (tp *treeProgress) skipFile(size int64) {
	tp.doneBytes += size
	tp.filesDone++
}

// RejectOversizedLocal Base64 直传上传前的预检：任一文件超过单文件上限即整体拒绝，
// 一次性列出全部超限文件，避免传输进行到一半才失败。
func RejectOversizedLocal(plan LocalTreePlan) error {
	sizes := make([]sizedPath, 0, len(plan.Files))
	for _, f := range plan.Files {
		sizes = append(sizes, sizedPath{rel: f.RelPath, size: f.Size})
	}
	return rejectOversized(sizes)
}

// RejectOversizedRemote Base64 直传下载前的预检，语义同 RejectOversizedLocal。
func RejectOversizedRemote(plan RemoteTreePlan) error {
	sizes := make([]sizedPath, 0, len(plan.Files))
	for _, f := range plan.Files {
		sizes = append(sizes, sizedPath{rel: f.RelPath, size: f.Size})
	}
	return rejectOversized(sizes)
}

type sizedPath struct {
	rel  string
	size int64
}

func rejectOversized(files []sizedPath) error {
	var over []string
	for _, f := range files {
		if f.size > maxBase64DirectBytes {
			over = append(over, fmt.Sprintf("  %s（%s）", f.rel, HumanBytes(f.size)))
		}
	}
	if len(over) == 0 {
		return nil
	}
	return &TransferError{
		Code: ErrorCodeFileSizeExceeded,
		Message: fmt.Sprintf("Base64 直传单文件上限 %d MB，所选目录中有 %d 个文件超限：\n%s\n请改用 Root 中转模式（非跳板机）或拆分目录。",
			maxBase64DirectBytes/1024/1024, len(over), strings.Join(over, "\n")),
	}
}

// HumanBytes 字节数的用户可读形式（进度/汇总文案用）。
func HumanBytes(n int64) string {
	switch {
	case n >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

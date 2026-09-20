package filetransfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// newSFTPTreeEnv 起一台带 SFTP 的假 SSH 服务器并拨号，返回传输器。
func newSFTPTreeEnv(t *testing.T) *SFTPTransport {
	t.Helper()
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root, EnableSFTP: true})
	t.Cleanup(func() { srv.Close() })

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return NewSFTPTransport(client)
}

// seedLocalTree 构造测试用本地目录树：
//
//	src/
//	├── a.txt            "aaa"
//	├── sub/
//	│   ├── b.log        "bbbb"
//	│   └── deep/
//	│       └── c.conf   "cc"
//	├── empty/           （空目录）
//	└── link -> a.txt    （符号链接，应跳过；Windows 无权限建链接时没有这一项）
//
// 第二个返回值表示符号链接是否创建成功。
func seedLocalTree(t *testing.T) (string, bool) {
	t.Helper()
	src := t.TempDir()
	writeTreeFile := func(rel, content string) {
		p := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeTreeFile("a.txt", "aaa")
	writeTreeFile("sub/b.log", "bbbb")
	writeTreeFile("sub/deep/c.conf", "cc")
	if err := os.MkdirAll(filepath.Join(src, "empty"), 0o755); err != nil {
		t.Fatalf("mkdir empty: %v", err)
	}
	hasLink := true
	if err := os.Symlink(filepath.Join(src, "a.txt"), filepath.Join(src, "link")); err != nil {
		hasLink = false // Windows 无开发者模式/管理员权限时无法建符号链接
	}
	return src, hasLink
}

func readRemote(t *testing.T, tr *SFTPTransport, ctx context.Context, path string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "probe")
	if _, err := tr.Download(ctx, path, dst, nil); err != nil {
		t.Fatalf("download probe %s: %v", path, err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	return string(b)
}

// 远端树上传：文件落位、空目录建出、符号链接跳过、进度步骤带文件计数。
func TestUploadTree_SFTP_CreatesDirsAndSkipsLinks(t *testing.T) {
	tr := newSFTPTreeEnv(t)
	src, hasLink := seedLocalTree(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var steps []string
	res, err := UploadTree(ctx, src, "dest", func(p Progress) {
		if p.Step != "" {
			steps = append(steps, p.Step)
		}
	}, UploadTreeOps{Mkdir: tr.Mkdir, UploadFile: tr.Upload})
	if err != nil {
		t.Fatalf("UploadTree: %v", err)
	}
	if res.Files != 3 || res.Dirs != 4 { // 根 + sub + deep + empty
		t.Fatalf("结果计数: files=%d dirs=%d, want 3/4", res.Files, res.Dirs)
	}
	if res.Bytes != 3+4+2 {
		t.Fatalf("字节数: %d, want 9", res.Bytes)
	}
	if len(res.Failures) != 0 {
		t.Fatalf("不应有失败: %+v", res.Failures)
	}

	// 文件内容
	if got := readRemote(t, tr, ctx, "dest/a.txt"); got != "aaa" {
		t.Errorf("a.txt 内容 %q", got)
	}
	if got := readRemote(t, tr, ctx, "dest/sub/deep/c.conf"); got != "cc" {
		t.Errorf("c.conf 内容 %q", got)
	}
	// 空目录被创建、符号链接被跳过
	entries, err := tr.List(ctx, "dest")
	if err != nil {
		t.Fatalf("list dest: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	want := "a.txt,empty,sub"
	if hasLink {
		want = "a.txt,empty,link,sub"
	}
	if strings.Join(names, ",") != want {
		t.Errorf("dest 下条目: %v, want %s", names, want)
	}
	// 进度步骤包含文件计数
	joined := strings.Join(steps, "\n")
	if !strings.Contains(joined, "3 个文件") {
		t.Errorf("进度缺少文件计数: %v", steps)
	}
}

// 合并语义：目标目录已存在时并入，同名文件覆盖，目标独有的文件保留。
func TestUploadTree_SFTP_MergesIntoExistingDir(t *testing.T) {
	tr := newSFTPTreeEnv(t)
	src, _ := seedLocalTree(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 预置目标：旧版 a.txt（应被覆盖）+ 目标独有文件 keep.txt（应保留）。
	if err := tr.Mkdir(ctx, "dest"); err != nil {
		t.Fatalf("预置 dest: %v", err)
	}
	if _, err := tr.Upload(ctx, mustTempFile(t, "old"), "dest/a.txt", nil); err != nil {
		t.Fatalf("预置旧 a.txt: %v", err)
	}
	if _, err := tr.Upload(ctx, mustTempFile(t, "keep"), "dest/keep.txt", nil); err != nil {
		t.Fatalf("预置 keep.txt: %v", err)
	}

	if _, err := UploadTree(ctx, src, "dest", nil, UploadTreeOps{Mkdir: tr.Mkdir, UploadFile: tr.Upload}); err != nil {
		t.Fatalf("UploadTree: %v", err)
	}

	if got := readRemote(t, tr, ctx, "dest/a.txt"); got != "aaa" {
		t.Errorf("同名文件未覆盖: %q", got)
	}
	if got := readRemote(t, tr, ctx, "dest/keep.txt"); got != "keep" {
		t.Errorf("目标独有文件丢失: %q", got)
	}
}

// 远端树下载：本地合并覆盖、子目录创建、本地独有文件保留。
func TestDownloadTree_SFTP_MergesIntoLocalDir(t *testing.T) {
	tr := newSFTPTreeEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 在假服务器上种一棵远端树
	seedRemote := func(rel, content string) {
		if err := tr.Mkdir(ctx, parentRemote(rel)); err != nil {
			t.Fatalf("远端 mkdir %s: %v", rel, err)
		}
		if _, err := tr.Upload(ctx, mustTempFile(t, content), rel, nil); err != nil {
			t.Fatalf("远端写 %s: %v", rel, err)
		}
	}
	seedRemote("srv/app.conf", "conf-v1")
	seedRemote("srv/logs/run.log", "log-line")

	// 本地预置目标目录（同名旧文件应被覆盖，独有文件保留）
	dst := filepath.Join(t.TempDir(), "srv")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "app.conf"), []byte("conf-v0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "local-only.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := DownloadTree(ctx, "srv", dst, nil, DownloadTreeOps{List: tr.List, DownloadFile: tr.Download})
	if err != nil {
		t.Fatalf("DownloadTree: %v", err)
	}
	if res.Files != 2 {
		t.Fatalf("files=%d, want 2", res.Files)
	}

	if b, _ := os.ReadFile(filepath.Join(dst, "app.conf")); string(b) != "conf-v1" {
		t.Errorf("同名文件未覆盖: %q", string(b))
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "logs", "run.log")); string(b) != "log-line" {
		t.Errorf("子目录文件内容错误: %q", string(b))
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "local-only.txt")); string(b) != "mine" {
		t.Errorf("本地独有文件丢失: %q", string(b))
	}
}

// 单文件失败收集后继续，其余文件照常完成。
func TestUploadTree_CollectsPerFileFailures(t *testing.T) {
	src, _ := seedLocalTree(t)

	var failed bool
	ops := UploadTreeOps{
		Mkdir: func(ctx context.Context, dir string) error { return nil },
		UploadFile: func(ctx context.Context, local, remote string, progress func(Progress)) (TransferResult, error) {
			if strings.HasSuffix(remote, "b.log") {
				failed = true
				return TransferResult{}, &TransferError{Code: ErrorCodePermissionDenied, Message: "模拟权限失败"}
			}
			return TransferResult{Bytes: 1}, nil
		},
	}
	res, err := UploadTree(context.Background(), src, "dest", nil, ops)
	if err != nil {
		t.Fatalf("不应整体失败: %v", err)
	}
	if !failed || res.Files != 2 || len(res.Failures) != 1 {
		t.Fatalf("失败收集: files=%d failures=%d", res.Files, len(res.Failures))
	}
	if res.Failures[0].Path != "sub/b.log" || !strings.Contains(res.Failures[0].Err, "模拟权限失败") {
		t.Errorf("失败记录: %+v", res.Failures[0])
	}
}

// 取消传播：ctx 取消后立即中止。
func TestUploadTree_CancelStopsImmediately(t *testing.T) {
	src, _ := seedLocalTree(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ops := UploadTreeOps{
		Mkdir:      func(ctx context.Context, dir string) error { return ctx.Err() },
		UploadFile: func(ctx context.Context, local, remote string, progress func(Progress)) (TransferResult, error) { return TransferResult{}, nil },
	}
	if _, err := UploadTree(ctx, src, "dest", nil, ops); err == nil {
		t.Fatalf("已取消的 ctx 应返回错误")
	}
}

// Base64 预检：超限文件一次性列出，不传任何字节。
func TestRejectOversizedLocal_ListsAllOffenders(t *testing.T) {
	plan := LocalTreePlan{Files: []LocalTreeFile{
		{RelPath: "ok.txt", Size: 1024},
		{RelPath: "big1.bin", Size: maxBase64DirectBytes + 1},
		{RelPath: "big2.bin", Size: 2 * maxBase64DirectBytes},
	}}
	err := RejectOversizedLocal(plan)
	if err == nil {
		t.Fatalf("应拒绝")
	}
	msg := err.Error()
	if !strings.Contains(msg, "big1.bin") || !strings.Contains(msg, "big2.bin") || !strings.Contains(msg, "2 个文件超限") {
		t.Errorf("超限清单不完整: %s", msg)
	}

	ok := LocalTreePlan{Files: []LocalTreeFile{{RelPath: "ok.txt", Size: maxBase64DirectBytes}}}
	if err := RejectOversizedLocal(ok); err != nil {
		t.Errorf("恰好等于上限不应拒绝: %v", err)
	}
}

// 本地 walk：符号链接识别、深度上限、父先子后的目录顺序。
func TestWalkLocalTree_SymlinkAndDepth(t *testing.T) {
	src, hasLink := seedLocalTree(t)
	plan, err := WalkLocalTree(src)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if hasLink {
		if len(plan.SkippedLinks) != 1 || plan.SkippedLinks[0] != "link" {
			t.Errorf("应只跳过符号链接: %v", plan.SkippedLinks)
		}
	}
	if len(plan.RelDirs) != 3 { // sub, sub/deep, empty
		t.Errorf("子目录数: %v", plan.RelDirs)
	}
	// 父先子后（同级按名排序，故 empty 可能在 sub 前，但 sub 必须在 sub/deep 前）
	idx := func(name string) int {
		for i, d := range plan.RelDirs {
			if d == name {
				return i
			}
		}
		return -1
	}
	if idx("sub") > idx("sub/deep") || idx("sub/deep") < 0 || idx("sub") < 0 {
		t.Errorf("目录顺序非父先子后: %v", plan.RelDirs)
	}

	// 超深目录触发上限
	deep := t.TempDir()
	p := deep
	for i := 0; i < maxTreeDepth+2; i++ {
		p = filepath.Join(p, fmt.Sprintf("d%d", i))
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := WalkLocalTree(deep); err == nil {
		t.Errorf("超深树应报错")
	}
}

func parentRemote(rel string) string {
	i := strings.LastIndex(rel, "/")
	if i <= 0 {
		return "."
	}
	return rel[:i]
}

func mustTempFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "seed")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

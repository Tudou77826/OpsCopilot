package shellsidecar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"opscopilot/pkg/remote"
)

// 阶段 4：FT 服务全链路测试（fakessh SFTP 子系统 + 数据目录沙箱）。

// ftTestRig 组装一套 svc + FTService + fakessh(SFTP) 并开好一个终端。
type ftTestRig struct {
	svc      *TerminalService
	ft       *FTService
	dataDir  string
	sftpRoot string
	terminal string

	mu    sync.Mutex
	event []map[string]any
}

func newFTRig(t *testing.T) *ftTestRig {
	t.Helper()
	dataDir := t.TempDir()
	sftpRoot := t.TempDir()
	server := startFakeSSHWithSFTP(t, sftpRoot)

	svc := NewTerminalService("test")
	t.Cleanup(svc.Shutdown)
	ft := NewFTService(svc, dataDir)

	connID, err := svc.Connect(remote.ConnectConfig{
		Host: "127.0.0.1", Port: server.Port(), User: "test", Password: "test",
		Protocol: remote.ProtocolSSH,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := svc.OpenTerminal(connID, 80, 24)
	if err != nil {
		t.Fatal(err)
	}

	rig := &ftTestRig{svc: svc, ft: ft, dataDir: dataDir, sftpRoot: sftpRoot, terminal: terminal}
	ft.SetNotify(func(method string, params any) {
		data, _ := json.Marshal(params)
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		rig.mu.Lock()
		rig.event = append(rig.event, map[string]any{"method": method, "params": m})
		rig.mu.Unlock()
	})
	return rig
}

func (r *ftTestRig) waitDone(t *testing.T, taskID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, e := range r.event {
			if e["method"] == "shell.ft/done" {
				p := e["params"].(map[string]any)
				if p["taskId"] == taskID {
					r.mu.Unlock()
					return p
				}
			}
		}
		r.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("done event for %s not received", taskID)
	return nil
}

func TestFTCheckAndRemoteOps(t *testing.T) {
	rig := newFTRig(t)

	if env := rig.ft.Check(rig.terminal); !env.OK || env.Message != "sftp(login)" {
		t.Fatalf("Check = ok:%v msg:%q err:%v", env.OK, env.Message, env.Message)
	}
	if env := rig.ft.Check("term-none"); env.OK {
		t.Fatal("Check on missing terminal should fail")
	}

	// mkdir → write → stat → list → read → rename → remove 全链路
	if env := rig.ft.RemoteMkdir(rig.terminal, "/dir-a"); !env.OK {
		t.Fatalf("mkdir: %v", env.Message)
	}
	if env := rig.ft.RemoteWriteFile(rig.terminal, "/dir-a/hello.txt", "你好 ft"); !env.OK {
		t.Fatalf("write: %v", env.Message)
	}
	env := rig.ft.Stat(rig.terminal, "/dir-a/hello.txt")
	if !env.OK || env.Entry == nil || env.Entry.Size == 0 {
		t.Fatalf("stat: ok=%v entry=%+v", env.OK, env.Entry)
	}
	listEnv := rig.ft.List(rig.terminal, "/dir-a")
	if !listEnv.OK || len(listEnv.Entries) != 1 || listEnv.Entries[0].Name != "hello.txt" {
		t.Fatalf("list: ok=%v n=%d", listEnv.OK, len(listEnv.Entries))
	}
	readEnv := rig.ft.RemoteReadFile(rig.terminal, "/dir-a/hello.txt", 0)
	if !readEnv.OK || readEnv.Content != "你好 ft" {
		t.Fatalf("read: ok=%v content=%q", readEnv.OK, readEnv.Content)
	}
	if env := rig.ft.RemoteRename(rig.terminal, "/dir-a/hello.txt", "/dir-a/renamed.txt"); !env.OK {
		t.Fatalf("rename: %v", env.Message)
	}
	// 递归删除目录
	if env := rig.ft.RemoteRemove(rig.terminal, "/dir-a"); !env.OK {
		t.Fatalf("remove dir: %v", env.Message)
	}
	if env := rig.ft.Stat(rig.terminal, "/dir-a/renamed.txt"); env.OK {
		t.Fatal("removed file should not exist")
	}
	// stat 不存在 → ok=false 而非异常
	if env := rig.ft.Stat(rig.terminal, "/nope"); env.OK {
		t.Fatal("stat missing should be ok=false")
	}
}

func TestFTLocalOpsSandbox(t *testing.T) {
	rig := newFTRig(t)

	if env := rig.ft.LocalMkdir("sub"); !env.OK {
		t.Fatalf("mkdir: %v", env.Message)
	}
	if err := os.WriteFile(filepath.Join(rig.dataDir, "a.txt"), []byte("AAA"), 0o644); err != nil {
		t.Fatal(err)
	}
	listEnv := rig.ft.LocalList("")
	if !listEnv.OK || len(listEnv.Entries) != 2 {
		t.Fatalf("list root: ok=%v n=%d", listEnv.OK, len(listEnv.Entries))
	}
	if listEnv.Entries[0].Path == "" || !strings.HasPrefix(listEnv.Entries[0].Path, filepath.ToSlash(filepath.Clean(rig.dataDir))) {
		t.Fatalf("entry path should be absolute under dataDir, got %q", listEnv.Entries[0].Path)
	}
	if env := rig.ft.LocalStat("a.txt"); !env.OK || env.Entry == nil {
		t.Fatalf("stat a.txt: %v", env.Message)
	}
	if env := rig.ft.LocalRename("a.txt", "sub/b.txt"); !env.OK {
		t.Fatalf("rename: %v", env.Message)
	}
	if env := rig.ft.LocalRemove("sub"); !env.OK {
		t.Fatalf("remove: %v", env.Message)
	}
	// 沙箱逃逸拒绝
	if env := rig.ft.LocalList("../"); env.OK {
		t.Fatal("escape should be rejected")
	}
}

func TestFTUploadDownloadRoundtrip(t *testing.T) {
	rig := newFTRig(t)

	payload := strings.Repeat("0123456789abcdef", 8192) // 128KB，触发多次进度上报
	if err := os.WriteFile(filepath.Join(rig.dataDir, "up.bin"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	up := rig.ft.Upload(rig.terminal, "up.bin", "/up.bin")
	if !up.OK || up.TaskID == "" {
		t.Fatalf("upload start failed: %v", up.Message)
	}
	doneUp := rig.waitDone(t, up.TaskID)
	if doneUp["ok"] != true {
		t.Fatalf("upload done event: %+v", doneUp)
	}

	down := rig.ft.Download(rig.terminal, "/up.bin", "downloads/down.bin")
	if !down.OK || down.TaskID == "" {
		t.Fatalf("download start failed: %v", down.Message)
	}
	doneDown := rig.waitDone(t, down.TaskID)
	if doneDown["ok"] != true {
		t.Fatalf("download done event: %+v", doneDown)
	}

	got, err := os.ReadFile(filepath.Join(rig.dataDir, "downloads", "down.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatalf("roundtrip mismatch: got %d bytes want %d", len(got), len(payload))
	}

	// 进度事件：至少一条字节进度（step 为空串）
	rig.mu.Lock()
	progressCount, hasByteProgress := 0, false
	for _, e := range rig.event {
		if e["method"] == "shell.ft/progress" {
			progressCount++
			p := e["params"].(map[string]any)
			if p["bytesTotal"] != nil && p["bytesTotal"].(float64) > 0 {
				hasByteProgress = true
			}
		}
	}
	rig.mu.Unlock()
	if progressCount == 0 || !hasByteProgress {
		t.Fatalf("expected byte progress events, count=%d hasByte=%v", progressCount, hasByteProgress)
	}
}

func TestFTCancelUnknownTask(t *testing.T) {
	rig := newFTRig(t)
	if env := rig.ft.Cancel("ft-none"); env.OK {
		t.Fatal("cancel unknown task should fail")
	}
}

func TestFTUploadValidatesLocalPath(t *testing.T) {
	rig := newFTRig(t)
	// 越界本地路径直接拒绝（不开任务）
	if env := rig.ft.Upload(rig.terminal, "../../secret", "/x"); env.OK {
		t.Fatal("sandbox escape upload should be rejected")
	}
	// 本地不存在的文件
	if env := rig.ft.Upload(rig.terminal, "missing.bin", "/x"); env.OK {
		t.Fatal("missing local file should be rejected")
	}
}

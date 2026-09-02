package shellsidecar

import (
	"testing"

	"opscopilot/pkg/remote"
)

// S5：快捷命令/脚本持久化（同格式兼容 + upsert + 重启恢复）。
func TestQuickCmdAndScriptPersistence(t *testing.T) {
	dir := t.TempDir()
	qc, err := NewQuickCmdService(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := qc.Save(QuickCommand{Name: "重启 nginx", Content: "systemctl restart nginx", Group: "运维"})
	if err != nil {
		t.Fatal(err)
	}
	// 兼容旧格式：手工写入一份旧 APPDATA 形态的文件再读
	if _, err := qc.Save(QuickCommand{ID: "3", Name: "旧命令", Content: "old"}); err != nil {
		t.Fatal(err)
	}
	if len(qc.List()) != 2 {
		t.Fatalf("expect 2 commands, got %d", len(qc.List()))
	}
	reopened, err := NewQuickCmdService(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range reopened.List() {
		if c.ID == id && c.Group == "运维" {
			found = true
		}
	}
	if !found {
		t.Fatal("saved command lost after reopen")
	}

	// 结构化脚本（阶段 5 起复用 pkg/script）与旧文本脚本迁移见 scriptsvc_test.go。
}

// S6（阶段 4 重构）：FT 服务的本地沙箱校验取代旧的分片上传前置校验。
// 完整 SFTP 链路（列表/上传/下载/取消）见 ft_test.go，走 fakessh SFTP 子系统。
func TestFTLocalSandboxRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	ft := NewFTService(NewTerminalService("test"), dir)
	for _, p := range []string{"..", "../../etc", "/etc/passwd"} {
		if env := ft.LocalList(p); env.OK {
			t.Fatalf("LocalList(%q) should be rejected by sandbox", p)
		}
	}
	if env := ft.LocalStat("../x"); env.OK {
		t.Fatal("LocalStat escape should be rejected")
	}
	// 沙箱内相对路径与空路径可用
	if env := ft.LocalList(""); !env.OK || len(env.Entries) != 0 {
		t.Fatalf("LocalList(\"\") should list empty data dir, got ok=%v n=%d", env.OK, len(env.Entries))
	}
	if env := ft.LocalMkdir("downloads"); !env.OK {
		t.Fatalf("LocalMkdir within sandbox failed: %v", env.Message)
	}
}

// S8：监控采样解析（真实格式文本 → 结构化）。
func TestMonitorSampleParsing(t *testing.T) {
	out := `0.42 0.31 0.28 1/312 4242
MemTotal:        2048000 kB
MemAvailable:    1536000 kB
512  409600 286720 122880  31% /
`
	sample, err := parseMonitorSample(out)
	if err != nil {
		t.Fatal(err)
	}
	if sample.Load1 != "0.42" {
		t.Fatalf("load1 = %q", sample.Load1)
	}
	if sample.MemTotalMB != 2000 || sample.MemUsedMB != 500 {
		t.Fatalf("mem = %v/%v", sample.MemTotalMB, sample.MemUsedMB)
	}
	if sample.MemUsedPct != 25 || sample.DiskUsedPct != 31 {
		t.Fatalf("pct = %v/%v", sample.MemUsedPct, sample.DiskUsedPct)
	}
	if sample.DiskPath != "/" {
		t.Fatalf("diskPath = %q", sample.DiskPath)
	}
	if _, err := parseMonitorSample("total garbage"); err == nil {
		t.Fatal("garbage should fail parsing")
	}
}

// S8：监控集成——fakessh exec 返回 canned 样本，真实 sshclient Run 走通。
func TestMonitorIntegrationOverRealSSH(t *testing.T) {
	server := startFakeSSH(t)
	svc := NewTerminalService("test")
	svc.dialer = func(cfg *remote.ConnectConfig) (remote.Connection, error) { return remote.Dial(cfg) }
	t.Cleanup(svc.Shutdown)

	connID, err := svc.Connect(remote.ConnectConfig{
		Host: "127.0.0.1", Port: server.Port(), User: "test", Password: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	sample, err := svc.SampleMonitor(connID)
	if err != nil {
		t.Fatalf("monitor sample over real ssh: %v", err)
	}
	if sample.Load1 != "0.42" || sample.MemTotalMB != 2000 {
		t.Fatalf("unexpected sample: %+v", sample)
	}
}

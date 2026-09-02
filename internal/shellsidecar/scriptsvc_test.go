package shellsidecar

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"opscopilot/pkg/remote"
	"opscopilot/pkg/script"
)

// 阶段 5：结构化脚本服务全链路（fakessh 真链路 + pkg/script 引擎复用）。

func newScriptRig(t *testing.T) (*TerminalService, *StructuredScriptService, string) {
	t.Helper()
	dataDir := t.TempDir()
	server := startFakeSSH(t)
	svc := NewTerminalService("test")
	t.Cleanup(svc.Shutdown)
	ss, err := NewStructuredScriptService(svc, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	connID, err := svc.Connect(remote.ConnectConfig{
		Host: "127.0.0.1", Port: server.Port(), User: "test", Password: "test",
		Protocol: remote.ProtocolSSH,
	})
	if err != nil {
		t.Fatal(err)
	}
	termID, err := svc.OpenTerminal(connID, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	return svc, ss, termID
}

func TestStructuredScriptCRUDAndReplay(t *testing.T) {
	_, ss, termID := newScriptRig(t)

	created, err := ss.Create("部署", "演示脚本")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Name != "部署" {
		t.Fatalf("created = %+v", created)
	}

	loaded, err := ss.Load(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Steps = []script.ScriptStep{
		{Command: "echo step-one", Enabled: true},
		{Command: "echo ${greet}", Enabled: true},
	}
	loaded.Variables = []script.ScriptVariable{
		{Name: "greet", DisplayName: "问候语", DefaultValue: "hello", Required: true},
	}
	if err := ss.Update(loaded); err != nil {
		t.Fatal(err)
	}

	list, err := ss.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %d err = %v", len(list), err)
	}

	// 变量回放（写入终端 stdin；fakessh 回显，无需断言输出）
	if err := ss.ReplayWithVars(created.ID, termID, map[string]string{"greet": "hi"}); err != nil {
		t.Fatalf("replayWithVars: %v", err)
	}
	// 无变量回放路径
	loaded2, _ := ss.Load(created.ID)
	loaded2.Variables = nil
	if err := ss.Update(loaded2); err != nil {
		t.Fatal(err)
	}
	if err := ss.Replay(created.ID, termID); err != nil {
		t.Fatalf("replay: %v", err)
	}

	if err := ss.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := ss.List(); len(l) != 0 {
		t.Fatalf("expected empty after delete, got %d", len(l))
	}
}

func TestStructuredScriptRecordingRoundtrip(t *testing.T) {
	svc, ss, termID := newScriptRig(t)

	if _, err := ss.StartRecording("录制演示", "", termID); err != nil {
		t.Fatal(err)
	}
	st := ss.RecordingStatus()
	if !st.IsRecording || st.Name != "录制演示" {
		t.Fatalf("status = %+v", st)
	}

	// 模拟键入两条命令（WriteInput → 录制钩子 → LineBuffer 提交行）
	_ = svc.WriteInput(termID, []byte("echo aa\n"))
	_ = svc.WriteInput(termID, []byte("echo bb\n"))
	time.Sleep(100 * time.Millisecond)

	stopped, err := ss.StopRecording()
	if err != nil {
		t.Fatal(err)
	}
	if len(stopped.Commands) != 2 || stopped.Commands[0].Content != "echo aa" {
		t.Fatalf("recorded commands = %+v", stopped.Commands)
	}
	if st2 := ss.RecordingStatus(); st2.IsRecording {
		t.Fatal("should not be recording after stop")
	}

	list, _ := ss.List()
	if len(list) != 1 || list[0].Name != "录制演示" {
		t.Fatalf("saved script missing: %+v", list)
	}
}

func TestLegacyTextScriptMigration(t *testing.T) {
	dir := t.TempDir()
	legacy := `[{"id":"old-1","name":"旧脚本","content":"echo a\necho b","group":"g1"}]`
	if err := os.WriteFile(filepath.Join(dir, "scripts.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewTerminalService("test")
	t.Cleanup(svc.Shutdown)
	ss, err := NewStructuredScriptService(svc, dir)
	if err != nil {
		t.Fatal(err)
	}

	list, err := ss.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("migrated list = %d err = %v", len(list), err)
	}
	m := list[0]
	if m.Name != "旧脚本" || len(m.Steps) != 2 || m.Steps[0].Command != "echo a" {
		t.Fatalf("migrated script = %+v", m)
	}
	if _, err := os.Stat(filepath.Join(dir, "scripts.json.migrated")); err != nil {
		t.Fatal("legacy file should be renamed to .migrated")
	}
	// 幂等：再次初始化不重复迁移
	ss2, err := NewStructuredScriptService(svc, dir)
	if err != nil {
		t.Fatal(err)
	}
	if l2, _ := ss2.List(); len(l2) != 1 {
		t.Fatalf("re-init duplicated scripts: %d", len(l2))
	}
}

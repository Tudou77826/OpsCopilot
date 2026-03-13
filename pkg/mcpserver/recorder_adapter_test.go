package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"opscopilot/pkg/recorder"
)

func TestMCPRecorderAdapter_StartSession(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	// 创建录制器和适配器
	r := recorder.NewRecorder(tmpDir)
	adapter := NewMCPRecorderAdapter(r)

	// 开始会话
	session, err := adapter.StartSession("测试问题")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if session.Problem != "测试问题" {
		t.Errorf("expected problem '测试问题', got '%s'", session.Problem)
	}

	if session.ID == "" {
		t.Error("expected non-empty session ID")
	}

	// 再次开始会话应该失败
	_, err = adapter.StartSession("另一个问题")
	if err == nil {
		t.Error("expected error when starting second session")
	}
}

func TestMCPRecorderAdapter_RecordCommand(t *testing.T) {
	tmpDir := t.TempDir()

	r := recorder.NewRecorder(tmpDir)
	adapter := NewMCPRecorderAdapter(r)

	// 没有活动会话时记录命令应该失败
	err := adapter.RecordCommand("server1", "ls -la", "output", 0, time.Second, "")
	if err == nil {
		t.Error("expected error when recording without active session")
	}

	// 开始会话
	_, _ = adapter.StartSession("测试问题")

	// 记录命令
	err = adapter.RecordCommand("server1", "ls -la", "output", 0, time.Second, "list files")
	if err != nil {
		t.Fatalf("RecordCommand failed: %v", err)
	}

	// 验证命令被记录
	session := adapter.GetCurrentSession()
	if len(session.Commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(session.Commands))
	}

	cmd := session.Commands[0]
	if cmd.Command != "ls -la" {
		t.Errorf("expected command 'ls -la', got '%s'", cmd.Command)
	}
	if cmd.Server != "server1" {
		t.Errorf("expected server 'server1', got '%s'", cmd.Server)
	}
	if !session.Servers["server1"] {
		t.Error("expected server1 in Servers map")
	}
}

func TestMCPRecorderAdapter_EndSession(t *testing.T) {
	tmpDir := t.TempDir()

	r := recorder.NewRecorder(tmpDir)
	adapter := NewMCPRecorderAdapter(r)

	// 开始会话
	_, _ = adapter.StartSession("测试问题")
	_ = adapter.RecordCommand("server1", "ls -la", "output", 0, time.Second, "")

	// 结束会话
	session, err := adapter.EndSession("root cause", "conclusion", []string{"finding1"})
	if err != nil {
		t.Fatalf("EndSession failed: %v", err)
	}

	if session.RootCause != "root cause" {
		t.Errorf("expected root cause 'root cause', got '%s'", session.RootCause)
	}
	if session.Conclusion != "conclusion" {
		t.Errorf("expected conclusion 'conclusion', got '%s'", session.Conclusion)
	}
	if len(session.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(session.Findings))
	}
	if session.EndTime == nil {
		t.Error("expected EndTime to be set")
	}

	// 验证文件已保存
	files, err := os.ReadDir(filepath.Join(tmpDir, "troubleshoot"))
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}

	// 会话结束后，GetCurrentSession 应该返回 nil
	if adapter.GetCurrentSession() != nil {
		t.Error("expected nil session after EndSession")
	}
}

func TestMCPRecorderAdapter_GetSessionStatus(t *testing.T) {
	tmpDir := t.TempDir()

	r := recorder.NewRecorder(tmpDir)
	adapter := NewMCPRecorderAdapter(r)

	// 没有活动会话
	status := adapter.GetSessionStatus()
	if status["active"] != false {
		t.Error("expected active=false when no session")
	}

	// 开始会话
	_, _ = adapter.StartSession("测试问题")
	_ = adapter.RecordCommand("server1", "ls -la", "output", 0, time.Second, "")

	// 有活动会话
	status = adapter.GetSessionStatus()
	if status["active"] != true {
		t.Error("expected active=true when session is active")
	}
	if status["problem"] != "测试问题" {
		t.Errorf("expected problem '测试问题', got '%v'", status["problem"])
	}
	if status["executed_commands"] != 1 {
		t.Errorf("expected 1 command, got %v", status["executed_commands"])
	}
}

func TestMCPRecorderAdapter_OutputTruncation(t *testing.T) {
	tmpDir := t.TempDir()

	r := recorder.NewRecorder(tmpDir)
	adapter := NewMCPRecorderAdapter(r)

	_, _ = adapter.StartSession("测试问题")

	// 创建超长输出
	longOutput := make([]byte, 15000)
	for i := range longOutput {
		longOutput[i] = 'a'
	}

	err := adapter.RecordCommand("server1", "cmd", string(longOutput), 0, time.Second, "")
	if err != nil {
		t.Fatalf("RecordCommand failed: %v", err)
	}

	session := adapter.GetCurrentSession()
	if len(session.Commands[0].Output) > 10050 {
		t.Errorf("output should be truncated, got length %d", len(session.Commands[0].Output))
	}
}

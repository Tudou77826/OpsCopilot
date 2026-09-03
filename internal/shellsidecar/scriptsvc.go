package shellsidecar

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"opscopilot/pkg/recorder"
	"opscopilot/pkg/script"
)

// StructuredScriptService：结构化脚本（pkg/script）的 sidecar 服务。
// 与 Wails 同一套引擎与数据格式（script_<id>.json），录制走 TerminalService
// 的 Write 拦截钩子（LineBuffer 归一化），回放经 CommandSender 写回终端。
//
// 数据兼容：旧的文本脚本（scripts.json，{id,name,content,group}）在初始化时
// 迁移为结构化脚本（content 按行拆为 steps），原文件改名为 scripts.json.migrated。
type StructuredScriptService struct {
	mu     sync.Mutex
	svc    *TerminalService
	mgr    *script.Manager
	rec    *recorder.Recorder
	jobs   map[string]*replayJob
	closed bool
}

func NewStructuredScriptService(svc *TerminalService, dataDir string) (*StructuredScriptService, error) {
	s, err := NewStructuredScriptServiceWithPaths(svc, filepath.Join(dataDir, "scripts"), filepath.Join(dataDir, "recordings"))
	if err != nil {
		return nil, err
	}
	if err = s.migrateLegacyTextScripts(dataDir, filepath.Join(dataDir, "scripts")); err != nil {
		return nil, err
	}
	return s, nil
}

func NewStructuredScriptServiceWithPaths(svc *TerminalService, scriptsDir, recDir string) (*StructuredScriptService, error) {
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建脚本目录失败: %w", err)
	}
	if err := os.MkdirAll(recDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建录制目录失败: %w", err)
	}

	s := &StructuredScriptService{svc: svc}
	s.rec = recorder.NewRecorder(recDir)
	s.mgr = script.NewManager(s.rec, scriptsDir, s)

	// 录制钩子：终端键入 → LineBuffer → 提交行进入录制会话
	svc.SetInputRecorder(func(terminalID string, data []byte) {
		_ = s.rec.RecordRawInput(terminalID, string(data))
	})
	return s, nil
}

// Recorder 暴露内部 recorder（诊断案例共享同一实例，终端键入才能进入排查时间线）。
func (s *StructuredScriptService) Recorder() *recorder.Recorder { return s.rec }

// SendCommand 实现 script.CommandSender：回放命令写入终端 stdin。
func (s *StructuredScriptService) SendCommand(sessionID string, command string) error {
	return s.svc.WriteInput(sessionID, []byte(command+"\n"))
}

// migrateLegacyTextScripts 把旧 scripts.json 的文本脚本转为结构化脚本。
func (s *StructuredScriptService) migrateLegacyTextScripts(dataDir, scriptsDir string) error {
	legacy := filepath.Join(dataDir, "scripts.json")
	data, err := os.ReadFile(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var items []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Content string `json:"content"`
		Group   string `json:"group"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		// 损坏的旧文件不阻塞启动
		return nil
	}
	for _, it := range items {
		if it.Name == "" || it.Content == "" {
			continue
		}
		if existing, _ := s.mgr.LoadScript(it.ID); existing != nil {
			continue
		}
		created, err := s.mgr.CreateScript(it.Name, "由旧文本脚本迁移")
		if err != nil {
			continue
		}
		migrated := *created
		for _, line := range strings.Split(strings.ReplaceAll(it.Content, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			migrated.Steps = append(migrated.Steps, script.ScriptStep{Command: line, Enabled: true})
		}
		_ = s.mgr.UpdateScript(&migrated)
	}
	return os.Rename(legacy, legacy+".migrated")
}

// ---- 结构化脚本操作（RPC 直接返回脚本对象，与 Wails 绑定形状一致） ----

func (s *StructuredScriptService) List() ([]*script.Script, error) { return s.mgr.ListScripts() }

func (s *StructuredScriptService) Load(id string) (*script.Script, error) {
	return s.mgr.LoadScript(id)
}

func (s *StructuredScriptService) Update(sc *script.Script) error { return s.mgr.UpdateScript(sc) }

func (s *StructuredScriptService) Delete(id string) error { return s.mgr.DeleteScript(id) }

func (s *StructuredScriptService) Create(name, description string) (*script.Script, error) {
	return s.mgr.CreateScript(name, description)
}

func (s *StructuredScriptService) Replay(id, terminalID string) error {
	return s.mgr.ReplayScript(id, terminalID)
}

func (s *StructuredScriptService) ReplayWithVars(id, terminalID string, values map[string]string) error {
	return s.mgr.ReplayScriptWithVars(id, terminalID, values)
}

// StartRecording 开始录制。host/user 取自终端所属连接配置（与 Wails 语义一致）。
func (s *StructuredScriptService) StartRecording(name, description, terminalID string) (*script.Script, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("script service closed")
	}
	for _, job := range s.jobs {
		if replayActive(job.snapshot().State) {
			return nil, fmt.Errorf("script replay is active")
		}
	}
	host, user, err := s.svc.SessionInfo(terminalID)
	if err != nil {
		return nil, err
	}
	return s.mgr.StartRecording(name, description, terminalID, host, user)
}

func (s *StructuredScriptService) StopRecording() (*script.Script, error) {
	return s.mgr.StopRecording()
}

func (s *StructuredScriptService) RecordingStatus() script.ScriptStatus {
	return s.mgr.GetRecordingStatus()
}

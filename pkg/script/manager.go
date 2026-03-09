package script

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"opscopilot/pkg/recorder"
)

// CommandSender 命令发送器接口（用于回放时发送命令）
type CommandSender interface {
	SendCommand(sessionID string, command string) error
}

// Manager 脚本管理器
type Manager struct {
	recorder    *recorder.Recorder
	storagePath string
	current     *Script
	mu          sync.RWMutex
	commandSender CommandSender // 用于回放时发送命令
}

// NewManager 创建脚本管理器
func NewManager(rec *recorder.Recorder, storagePath string, commandSender CommandSender) *Manager {
	return &Manager{
		recorder:      rec,
		storagePath:   storagePath,
		commandSender: commandSender,
	}
}

// SetCommandSender 设置命令发送器
func (m *Manager) SetCommandSender(sender CommandSender) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commandSender = sender
}

// SendCommand 实现 CommandSender 接口（用于 App）
func (m *Manager) SendCommand(sessionID string, command string) error {
	if m.commandSender == nil {
		return fmt.Errorf("command sender not set")
	}
	return m.commandSender.SendCommand(sessionID, command)
}

// StartRecording 开始录制脚本
func (m *Manager) StartRecording(name, description, sessionID, host, user string) (*Script, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current != nil {
		return nil, fmt.Errorf("already recording")
	}

	// 开始录制
	session, err := m.recorder.Start(recorder.RecordingTypeScript, sessionID, host, user)
	if err != nil {
		return nil, fmt.Errorf("failed to start recording: %w", err)
	}

	// 创建脚本
	script := NewScript(session, name, description)

	m.current = script
	return script, nil
}

// StopRecording 停止录制
func (m *Manager) StopRecording() (*Script, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil {
		return nil, fmt.Errorf("not recording")
	}

	// 停止录制
	baseSession, err := m.recorder.Stop()
	if err != nil {
		return nil, fmt.Errorf("failed to stop recording: %w", err)
	}

	// 更新基础录制会话
	m.current.SyncFromRecordingSession(baseSession)

	// 将基础命令转换为可编辑的脚本命令
	m.current.Commands = make([]ScriptCommand, len(baseSession.Commands))
	for i, cmd := range baseSession.Commands {
		m.current.Commands[i] = ScriptCommand{
			RecordedCommand: cmd,
			Comment:         "",
			Delay:           0,
			Enabled:         true,
		}
	}

	// 保存脚本
	if err := m.saveScript(m.current); err != nil {
		return nil, fmt.Errorf("failed to save script: %w", err)
	}

	script := m.current
	m.current = nil
	return script, nil
}

// GetCurrentScript 获取当前录制的脚本
func (m *Manager) GetCurrentScript() *Script {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// UpdateScript 更新脚本（编辑后保存）
func (m *Manager) UpdateScript(script *Script) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	script.UpdatedAt = time.Now()
	return m.saveScript(script)
}

// LoadScript 加载脚本
func (m *Manager) LoadScript(scriptID string) (*Script, error) {
	// 从录制引擎加载基础会话
	session, err := m.recorder.Load(recorder.RecordingTypeScript, scriptID)
	if err != nil {
		return nil, fmt.Errorf("failed to load recording: %w", err)
	}

	// 加载扩展的脚本数据
	script := &Script{}
	filename := filepath.Join(m.storagePath, fmt.Sprintf("script_%s.json", scriptID))

	data, err := os.ReadFile(filename)
	if err != nil {
		// 如果没有扩展数据，从基础会话创建脚本
		script = NewScript(session, "", "")
		return script, nil
	}

	if err := json.Unmarshal(data, script); err != nil {
		return nil, fmt.Errorf("failed to unmarshal script: %w", err)
	}

	// 同步基础会话数据
	script.SyncFromRecordingSession(session)

	return script, nil
}

// ListScripts 列出所有脚本
func (m *Manager) ListScripts() ([]*Script, error) {
	sessions, err := m.recorder.List(recorder.RecordingTypeScript)
	if err != nil {
		return nil, err
	}

	scripts := make([]*Script, 0, len(sessions))
	for _, session := range sessions {
		script := &Script{}
		script.SyncFromRecordingSession(session)

		// 尝试加载扩展数据
		filename := filepath.Join(m.storagePath, fmt.Sprintf("script_%s.json", session.ID))
		if data, err := os.ReadFile(filename); err == nil {
			if err := json.Unmarshal(data, script); err == nil {
				script.SyncFromRecordingSession(session)
				// 成功加载扩展数据，继续使用这个script对象
				scripts = append(scripts, script)
				continue
			}
		}

		// 如果没有扩展数据或加载失败，从基础会话创建脚本
		script = NewScript(session, "", "")
		scripts = append(scripts, script)
	}

	return scripts, nil
}

// DeleteScript 删除脚本
func (m *Manager) DeleteScript(scriptID string) error {
	// 删除扩展数据
	filename := filepath.Join(m.storagePath, fmt.Sprintf("script_%s.json", scriptID))
	os.Remove(filename)

	// 删除录制数据
	return m.recorder.Delete(recorder.RecordingTypeScript, scriptID)
}

// ReplayScript 回放脚本
func (m *Manager) ReplayScript(scriptID string, sessionID string) error {
	log.Printf("[ScriptReplay] Starting replay: scriptID=%s, sessionID=%s", scriptID, sessionID)

	// 加载脚本
	scriptData, err := m.LoadScript(scriptID)
	if err != nil {
		log.Printf("[ScriptReplay] Failed to load script: %v", err)
		return err
	}

	log.Printf("[ScriptReplay] Loaded script '%s' with %d commands", scriptData.Name, len(scriptData.Commands))

	// 逐条执行命令
	for i, cmd := range scriptData.Commands {
		if !cmd.Enabled {
			log.Printf("[ScriptReplay] Skipping disabled command %d: %s", i, cmd.Content)
			continue
		}

		// 延迟
		if cmd.Delay > 0 {
			time.Sleep(time.Duration(cmd.Delay) * time.Millisecond)
		}

		// 发送命令
		log.Printf("[ScriptReplay] Executing command %d: %s", i, cmd.Content)
		if err := m.commandSender.SendCommand(sessionID, cmd.Content+"\n"); err != nil {
			log.Printf("[ScriptReplay] Failed to execute command '%s': %v", cmd.Content, err)
			return fmt.Errorf("failed to execute command '%s': %w", cmd.Content, err)
		}

		// 等待命令执行完成（简单等待，可以根据提示符优化）
		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("[ScriptReplay] Replay completed successfully")
	return nil
}

// ExportScript 导出为Shell脚本
func (m *Manager) ExportScript(scriptID string) (string, error) {
	script, err := m.LoadScript(scriptID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder

	sb.WriteString("#!/bin/bash\n")
	sb.WriteString(fmt.Sprintf("# %s\n", script.Name))
	sb.WriteString(fmt.Sprintf("# %s\n\n", script.Description))
	sb.WriteString(fmt.Sprintf("# Recorded: %s\n", script.StartTime.Format("2006-01-02 15:04:05")))
	if script.Host != "" {
		sb.WriteString(fmt.Sprintf("# Host: %s@%s\n\n", script.User, script.Host))
	}

	for _, cmd := range script.Commands {
		if !cmd.Enabled {
			sb.WriteString(fmt.Sprintf("# %s (disabled)\n", cmd.Content))
			continue
		}

		if cmd.Comment != "" {
			sb.WriteString(fmt.Sprintf("# %s\n", cmd.Comment))
		}

		sb.WriteString(fmt.Sprintf("%s\n", cmd.Content))

		if cmd.Delay > 0 {
			sb.WriteString(fmt.Sprintf("sleep %f\n", float64(cmd.Delay)/1000))
		}
	}

	return sb.String(), nil
}

// GetRecordingStatus 获取录制状态
func (m *Manager) GetRecordingStatus() ScriptStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := ScriptStatus{
		IsRecording:  m.current != nil,
		CommandCount: 0,
	}

	if m.current != nil {
		status.ScriptID = m.current.ID
		status.Name = m.current.Name

		// 从核心录制器获取实时命令数（而不是本地的 Commands）
		recorderStatus := m.recorder.GetStatus()
		status.CommandCount = recorderStatus.CommandCount
		status.Duration = recorderStatus.Duration

		// 备用：如果核心录制器没有返回命令数，使用本地计算
		if status.CommandCount == 0 && len(m.current.Commands) > 0 {
			status.CommandCount = len(m.current.Commands)
			if m.current.EndTime.IsZero() {
				status.Duration = int64(time.Since(m.current.StartTime).Seconds())
			} else {
				status.Duration = int64(m.current.EndTime.Sub(m.current.StartTime).Seconds())
			}
		}
	}

	return status
}

// saveScript 保存脚本扩展数据
func (m *Manager) saveScript(script *Script) error {
	if err := os.MkdirAll(m.storagePath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filename := filepath.Join(m.storagePath, fmt.Sprintf("script_%s.json", script.ID))
	data, err := json.MarshalIndent(script, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal script: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

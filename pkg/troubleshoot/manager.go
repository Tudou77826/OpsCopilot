package troubleshoot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"opscopilot/pkg/recorder"
)

// Manager 故障排查管理器
type Manager struct {
	recorder      *recorder.Recorder
	storagePath   string
	currentCase   *Case
	mu            sync.RWMutex
}

// NewManager 创建故障排查管理器
func NewManager(rec *recorder.Recorder, storagePath string) *Manager {
	return &Manager{
		recorder:    rec,
		storagePath: storagePath,
	}
}

// StartCase 开始故障排查
func (m *Manager) StartCase(problem string, context []string, sessionID, host, user string) (*Case, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentCase != nil {
		return nil, fmt.Errorf("already troubleshooting")
	}

	// 开始录制
	session, err := m.recorder.Start(recorder.RecordingTypeTroubleshoot, sessionID, host, user)
	if err != nil {
		return nil, fmt.Errorf("failed to start recording: %w", err)
	}

	// 创建故障排查案例
	caseData := NewCase(session, problem, context)

	m.currentCase = caseData
	return caseData, nil
}

// StopCase 结束故障排查
func (m *Manager) StopCase(rootCause, conclusion string) (*Case, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentCase == nil {
		return nil, fmt.Errorf("no active case")
	}

	// 停止录制
	baseSession, err := m.recorder.Stop()
	if err != nil {
		return nil, fmt.Errorf("failed to stop recording: %w", err)
	}

	// 更新案例信息
	m.currentCase.RecordingSession = *baseSession
	m.currentCase.RootCause = rootCause
	m.currentCase.Conclusion = conclusion

	// 保存案例
	if err := m.saveCase(m.currentCase); err != nil {
		return nil, fmt.Errorf("failed to save case: %w", err)
	}

	caseData := m.currentCase
	m.currentCase = nil
	return caseData, nil
}

// GetCurrentCase 获取当前案例
func (m *Manager) GetCurrentCase() *Case {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentCase
}

// GetStatus 获取排查状态
func (m *Manager) GetStatus() CaseStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := CaseStatus{
		IsActive:     m.currentCase != nil,
		CommandCount: 0,
	}

	if m.currentCase != nil {
		status.CaseID = m.currentCase.ID
		status.Problem = m.currentCase.Problem
		status.CommandCount = len(m.currentCase.Commands)
		status.Duration = int64(m.currentCase.EndTime.Sub(m.currentCase.StartTime).Seconds())
	}

	return status
}

// LoadCase 加载案例
func (m *Manager) LoadCase(caseID string) (*Case, error) {
	// 从录制引擎加载基础会话
	session, err := m.recorder.Load(recorder.RecordingTypeTroubleshoot, caseID)
	if err != nil {
		return nil, fmt.Errorf("failed to load recording: %w", err)
	}

	// 加载扩展的案例数据
	caseData := &Case{}
	filename := filepath.Join(m.storagePath, fmt.Sprintf("case_%s.json", caseID))

	data, err := os.ReadFile(filename)
	if err != nil {
		// 如果没有扩展数据，返回基础案例
		caseData.RecordingSession = *session
		return caseData, nil
	}

	if err := json.Unmarshal(data, caseData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal case: %w", err)
	}

	return caseData, nil
}

// ListCases 列出所有案例
func (m *Manager) ListCases() ([]*Case, error) {
	sessions, err := m.recorder.List(recorder.RecordingTypeTroubleshoot)
	if err != nil {
		return nil, err
	}

	cases := make([]*Case, 0, len(sessions))
	for _, session := range sessions {
		caseData := &Case{
			RecordingSession: *session,
		}

		// 尝试加载扩展数据
		filename := filepath.Join(m.storagePath, fmt.Sprintf("case_%s.json", session.ID))
		if data, err := os.ReadFile(filename); err == nil {
			json.Unmarshal(data, caseData)
		}

		cases = append(cases, caseData)
	}

	return cases, nil
}

// DeleteCase 删除案例
func (m *Manager) DeleteCase(caseID string) error {
	// 删除扩展数据
	filename := filepath.Join(m.storagePath, fmt.Sprintf("case_%s.json", caseID))
	os.Remove(filename)

	// 删除录制数据
	return m.recorder.Delete(recorder.RecordingTypeTroubleshoot, caseID)
}

// GenerateDocument 生成排查文档（Markdown格式）
func (m *Manager) GenerateDocument(caseID string) (string, error) {
	caseData, err := m.LoadCase(caseID)
	if err != nil {
		return "", err
	}

	var md strings.Builder

	md.WriteString("# 故障排查报告\n\n")
	md.WriteString(fmt.Sprintf("**时间**: %s\n", caseData.StartTime.Format("2006-01-02 15:04:05")))
	if !caseData.EndTime.IsZero() {
		md.WriteString(fmt.Sprintf("**时长**: %s\n", caseData.EndTime.Sub(caseData.StartTime).String()))
	}
	md.WriteString(fmt.Sprintf("**主机**: %s@%s\n\n", caseData.User, caseData.Host))

	md.WriteString("## 问题描述\n\n")
	md.WriteString(caseData.Problem)
	md.WriteString("\n\n")

	if len(caseData.Context) > 0 {
		md.WriteString("## 知识上下文\n\n")
		for _, ctx := range caseData.Context {
			md.WriteString(fmt.Sprintf("- %s\n", ctx))
		}
		md.WriteString("\n")
	}

	md.WriteString("## 排查过程\n\n")
	if len(caseData.Commands) == 0 {
		md.WriteString("*无命令记录*\n\n")
	} else {
		for i, cmd := range caseData.Commands {
			md.WriteString(fmt.Sprintf("%d. ```bash\n%s\n```\n\n", i+1, cmd.Content))
		}
	}

	if caseData.RootCause != "" {
		md.WriteString("## 根因分析\n\n")
		md.WriteString(caseData.RootCause)
		md.WriteString("\n\n")
	}

	if caseData.Conclusion != "" {
		md.WriteString("## 结论\n\n")
		md.WriteString(caseData.Conclusion)
		md.WriteString("\n\n")
	}

	if len(caseData.Suggestions) > 0 {
		md.WriteString("## 建议\n\n")
		for i, suggestion := range caseData.Suggestions {
			md.WriteString(fmt.Sprintf("%d. %s\n", i+1, suggestion))
		}
		md.WriteString("\n")
	}

	return md.String(), nil
}

// ExportCase 导出案例为JSON
func (m *Manager) ExportCase(caseID string) (string, error) {
	caseData, err := m.LoadCase(caseID)
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(caseData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal case: %w", err)
	}

	return string(data), nil
}

// saveCase 保存案例扩展数据
func (m *Manager) saveCase(caseData *Case) error {
	if err := os.MkdirAll(m.storagePath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filename := filepath.Join(m.storagePath, fmt.Sprintf("case_%s.json", caseData.ID))
	data, err := json.MarshalIndent(caseData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal case: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

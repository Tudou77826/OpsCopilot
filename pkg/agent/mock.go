// Package agent 定义 Agent 组件的核心接口和类型
package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockRecorder Mock 录制器
type MockRecorder struct {
	mu          sync.Mutex
	current     *RecordingSession
	status      RecorderStatus
	recordError error
}

// NewMockRecorder 创建 Mock 录制器
func NewMockRecorder() *MockRecorder {
	return &MockRecorder{}
}

// Start 开始录制
func (r *MockRecorder) Start(recType RecordingType, sessionID, host, user string) (*RecordingSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.recordError != nil {
		return nil, r.recordError
	}

	if r.current != nil {
		return nil, fmt.Errorf("already recording")
	}

	r.current = &RecordingSession{
		ID:        fmt.Sprintf("rec-%s", sessionID),
		Type:      recType,
		StartTime: time.Now(),
		SessionID: sessionID,
		Host:      host,
		User:      user,
		Commands:  make([]RecordedCommand, 0),
		Metadata:  make(map[string]interface{}),
	}

	r.status = RecorderStatus{
		IsRecording:  true,
		SessionID:    sessionID,
		Type:         recType,
		CommandCount: 0,
	}

	return r.current, nil
}

// RecordInput 记录输入
func (r *MockRecorder) RecordInput(sessionID string, command string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return nil
	}

	r.current.Commands = append(r.current.Commands, RecordedCommand{
		Index:     len(r.current.Commands),
		Content:   command,
		Timestamp: time.Since(r.current.StartTime).Milliseconds(),
	})
	r.status.CommandCount = len(r.current.Commands)

	return nil
}

// Stop 停止录制
func (r *MockRecorder) Stop() (*RecordingSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return nil, fmt.Errorf("not recording")
	}

	r.current.EndTime = time.Now()
	session := r.current
	r.current = nil
	r.status = RecorderStatus{IsRecording: false}

	return session, nil
}

// GetStatus 获取状态
func (r *MockRecorder) GetStatus() RecorderStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// GetCurrentSession 获取当前会话
func (r *MockRecorder) GetCurrentSession() *RecordingSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current
}

// SetRecordError 设置错误（用于测试）
func (r *MockRecorder) SetRecordError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordError = err
}

// MockTroubleshotter Mock 故障排查器
type MockTroubleshotter struct {
	mu          sync.RWMutex
	currentCase *TroubleshootCase
	cases       map[string]*TroubleshootCase
}

// NewMockTroubleshotter 创建 Mock 故障排查器
func NewMockTroubleshotter() *MockTroubleshotter {
	return &MockTroubleshotter{
		cases: make(map[string]*TroubleshootCase),
	}
}

// StartCase 开始故障排查
func (t *MockTroubleshotter) StartCase(problem string, context []string, sessionID, host, user string) (*TroubleshootCase, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.currentCase != nil {
		return nil, fmt.Errorf("already troubleshooting")
	}

	t.currentCase = &TroubleshootCase{
		RecordingSession: RecordingSession{
			ID:        fmt.Sprintf("case-%s", sessionID),
			Type:      RecordingTypeTroubleshoot,
			StartTime: time.Now(),
			SessionID: sessionID,
			Host:      host,
			User:      user,
			Commands:  make([]RecordedCommand, 0),
		},
		Problem:     problem,
		Context:     context,
		Suggestions: make([]string, 0),
	}

	t.cases[t.currentCase.ID] = t.currentCase
	return t.currentCase, nil
}

// StopCase 结束故障排查
func (t *MockTroubleshotter) StopCase(rootCause, conclusion string) (*TroubleshootCase, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.currentCase == nil {
		return nil, fmt.Errorf("no active case")
	}

	t.currentCase.EndTime = time.Now()
	t.currentCase.RootCause = rootCause
	t.currentCase.Conclusion = conclusion

	caseData := t.currentCase
	t.currentCase = nil
	return caseData, nil
}

// GetCurrentCase 获取当前案例
func (t *MockTroubleshotter) GetCurrentCase() *TroubleshootCase {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.currentCase
}

// GetStatus 获取状态
func (t *MockTroubleshotter) GetStatus() TroubleshootStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	status := TroubleshootStatus{IsActive: t.currentCase != nil}
	if t.currentCase != nil {
		status.CaseID = t.currentCase.ID
		status.Problem = t.currentCase.Problem
		status.CommandCount = len(t.currentCase.Commands)
	}
	return status
}

// LoadCase 加载案例
func (t *MockTroubleshotter) LoadCase(caseID string) (*TroubleshootCase, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	caseData, ok := t.cases[caseID]
	if !ok {
		return nil, fmt.Errorf("case not found: %s", caseID)
	}
	return caseData, nil
}

// ListCases 列出所有案例
func (t *MockTroubleshotter) ListCases() ([]*TroubleshootCase, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cases := make([]*TroubleshootCase, 0, len(t.cases))
	for _, c := range t.cases {
		cases = append(cases, c)
	}
	return cases, nil
}

// DeleteCase 删除案例
func (t *MockTroubleshotter) DeleteCase(caseID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.cases[caseID]; !ok {
		return fmt.Errorf("case not found: %s", caseID)
	}
	delete(t.cases, caseID)
	return nil
}

// GenerateDocument 生成文档
func (t *MockTroubleshotter) GenerateDocument(caseID string) (string, error) {
	caseData, err := t.LoadCase(caseID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("# Case: %s\n\n%s", caseData.Problem, caseData.RootCause), nil
}

// MockAgentService Mock Agent 服务
type MockAgentService struct {
	mu       sync.RWMutex
	running  bool
	sessions map[string][]string // sessionID -> context
}

// NewMockAgentService 创建 Mock Agent 服务
func NewMockAgentService() *MockAgentService {
	return &MockAgentService{
		sessions: make(map[string][]string),
	}
}

// ProcessUserInput 处理用户输入
func (s *MockAgentService) ProcessUserInput(ctx context.Context, sessionID string, input string) (*AIResponse, error) {
	return &AIResponse{
		Success: true,
		Message: fmt.Sprintf("Processed: %s", input),
	}, nil
}

// GetStatus 获取状态
func (s *MockAgentService) GetStatus() ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ServiceStatus{
		Running:      s.running,
		SessionCount: len(s.sessions),
	}
}

// Start 启动服务
func (s *MockAgentService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = true
	return nil
}

// Stop 停止服务
func (s *MockAgentService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	return nil
}

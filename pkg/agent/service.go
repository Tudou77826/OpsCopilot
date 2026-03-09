// Package agent 定义 Agent 组件的核心接口和类型
package agent

import (
	"context"
	"fmt"
	"sync"

	"opscopilot/pkg/bridge"
	"opscopilot/pkg/recorder"
)

// ServiceImpl Agent 服务实现
type ServiceImpl struct {
	mu            sync.RWMutex
	bus           *bridge.BridgeImpl
	recorder      *recorder.Recorder
	running       bool
	commandExec   CommandExecutor
	eventHandlers map[string][]EventHandler
}

// ServiceOption 服务选项
type ServiceOption func(*ServiceImpl)

// NewService 创建 Agent 服务
func NewService(opts ...ServiceOption) *ServiceImpl {
	s := &ServiceImpl{
		eventHandlers: make(map[string][]EventHandler),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// WithBus 设置事件总线
func WithBus(bus *bridge.BridgeImpl) ServiceOption {
	return func(s *ServiceImpl) {
		s.bus = bus
	}
}

// WithRecorder 设置录制器
func WithRecorder(r *recorder.Recorder) ServiceOption {
	return func(s *ServiceImpl) {
		s.recorder = r
	}
}

// WithCommandExecutor 设置命令执行器
func WithCommandExecutor(exec CommandExecutor) ServiceOption {
	return func(s *ServiceImpl) {
		s.commandExec = exec
	}
}

// Start 启动服务
func (s *ServiceImpl) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("service already running")
	}

	// 订阅事件
	if s.bus != nil {
		_, err := s.bus.SubscribeAll(s.handleEvent)
		if err != nil {
			return fmt.Errorf("failed to subscribe to events: %w", err)
		}
	}

	s.running = true
	return nil
}

// Stop 停止服务
func (s *ServiceImpl) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.running = false
	return nil
}

// GetStatus 获取服务状态
func (s *ServiceImpl) GetStatus() ServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return ServiceStatus{
		Running: s.running,
	}
}

// ProcessUserInput 处理用户输入
func (s *ServiceImpl) ProcessUserInput(ctx context.Context, sessionID string, input string) (*AIResponse, error) {
	// 这里应该调用实际的 AI 处理逻辑
	// 目前返回一个简单的响应
	return &AIResponse{
		Success: true,
		Message: fmt.Sprintf("Processed input for session %s: %s", sessionID, input),
	}, nil
}

// handleEvent 处理事件
func (s *ServiceImpl) handleEvent(ctx context.Context, event bridge.Event) error {
	s.mu.RLock()
	handlers := s.eventHandlers[event.Type]
	s.mu.RUnlock()

	for _, h := range handlers {
		if err := h(event.Payload); err != nil {
			fmt.Printf("event handler error for %s: %v\n", event.Type, err)
		}
	}

	return nil
}

// Subscribe 订阅事件
func (s *ServiceImpl) Subscribe(eventType string, handler EventHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.eventHandlers[eventType] = append(s.eventHandlers[eventType], handler)
	return nil
}

// Unsubscribe 取消订阅
func (s *ServiceImpl) Unsubscribe(eventType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.eventHandlers, eventType)
	return nil
}

// StartRecording 开始录制
func (s *ServiceImpl) StartRecording(recType RecordingType, sessionID, host, user string) (*RecordingSession, error) {
	if s.recorder == nil {
		return nil, fmt.Errorf("recorder not configured")
	}

	session, err := s.recorder.Start(recorder.RecordingType(recType), sessionID, host, user)
	if err != nil {
		return nil, err
	}

	// 转换类型
	return convertRecordingSession(session), nil
}

// StopRecording 停止录制
func (s *ServiceImpl) StopRecording() (*RecordingSession, error) {
	if s.recorder == nil {
		return nil, fmt.Errorf("recorder not configured")
	}

	session, err := s.recorder.Stop()
	if err != nil {
		return nil, err
	}

	return convertRecordingSession(session), nil
}

// convertRecordingSession 转换录制会话类型
func convertRecordingSession(session *recorder.RecordingSession) *RecordingSession {
	result := &RecordingSession{
		ID:        session.ID,
		Type:      RecordingType(session.Type),
		StartTime: session.StartTime,
		EndTime:   session.EndTime,
		SessionID: session.SessionID,
		Host:      session.Host,
		User:      session.User,
		Commands:  make([]RecordedCommand, len(session.Commands)),
		Metadata:  session.Metadata,
	}

	for i, cmd := range session.Commands {
		result.Commands[i] = RecordedCommand{
			Index:     cmd.Index,
			Content:   cmd.Content,
			Output:    cmd.Output,
			Timestamp: cmd.Timestamp,
			Duration:  cmd.Duration,
		}
	}

	return result
}

// GetRecordingStatus 获取录制状态
func (s *ServiceImpl) GetRecordingStatus() RecorderStatus {
	if s.recorder == nil {
		return RecorderStatus{}
	}

	status := s.recorder.GetStatus()
	return RecorderStatus{
		IsRecording:  status.IsRecording,
		SessionID:    status.SessionID,
		Type:         RecordingType(status.Type),
		CommandCount: status.CommandCount,
		Duration:     status.Duration,
	}
}

// ExecuteCommand 执行命令
func (s *ServiceImpl) ExecuteCommand(ctx context.Context, sessionID string, command string) error {
	if s.commandExec == nil {
		return fmt.Errorf("command executor not configured")
	}

	return s.commandExec.ExecuteCommand(ctx, sessionID, command)
}

// ExecuteCommands 批量执行命令
func (s *ServiceImpl) ExecuteCommands(ctx context.Context, sessionID string, commands []string) error {
	if s.commandExec == nil {
		return fmt.Errorf("command executor not configured")
	}

	return s.commandExec.ExecuteCommands(ctx, sessionID, commands)
}

// RecordInput 记录输入
func (s *ServiceImpl) RecordInput(sessionID string, command string) error {
	if s.recorder == nil {
		return nil
	}

	return s.recorder.RecordInput(sessionID, command)
}

// PublishEvent 发布事件
func (s *ServiceImpl) PublishEvent(ctx context.Context, eventType string, sessionID string, payload interface{}) error {
	if s.bus == nil {
		return nil
	}

	return s.bus.PublishAgentEvent(ctx, eventType, sessionID, payload)
}

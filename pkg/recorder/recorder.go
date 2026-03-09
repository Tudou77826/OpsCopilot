package recorder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"opscopilot/pkg/terminal"
)

// Recorder 核心录制器
type Recorder struct {
	mu          sync.Mutex
	current     *RecordingSession
	lineBuffers map[string]*terminal.LineBuffer
	storagePath string
}

// NewRecorder 创建录制器
func NewRecorder(storagePath string) *Recorder {
	return &Recorder{
		lineBuffers: make(map[string]*terminal.LineBuffer),
		storagePath: storagePath,
	}
}

// Start 开始录制
func (r *Recorder) Start(recType RecordingType, sessionID, host, user string) (*RecordingSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current != nil {
		return nil, fmt.Errorf("already recording")
	}

	session := &RecordingSession{
		ID:         uuid.New().String(),
		Type:       recType,
		StartTime:  time.Now(),
		SessionID:  sessionID,
		Host:       host,
		User:       user,
		Commands:   make([]RecordedCommand, 0),
		Metadata:   make(map[string]interface{}),
		Timeline:   make([]TimelineEvent, 0),
		Context:    make([]string, 0),
		Suggestions: make([]string, 0),
	}

	r.current = session
	// 重置 LineBuffer
	r.lineBuffers = make(map[string]*terminal.LineBuffer)
	return session, nil
}

// StartSession 开始故障排查会话（兼容旧 API）
func (r *Recorder) StartSession(problem string, context []string) *RecordingSession {
	r.mu.Lock()
	defer r.mu.Unlock()

	session := &RecordingSession{
		ID:          uuid.New().String(),
		Type:        RecordingTypeTroubleshoot,
		StartTime:   time.Now(),
		Commands:    make([]RecordedCommand, 0),
		Metadata:    make(map[string]interface{}),
		Timeline:    make([]TimelineEvent, 0),
		Problem:     problem,
		Context:     context,
		Suggestions: make([]string, 0),
	}

	r.current = session
	// 重置 LineBuffer
	r.lineBuffers = make(map[string]*terminal.LineBuffer)
	return session
}

// RecordInput 记录输入（由App.go在收到Enter时调用）
func (r *Recorder) RecordInput(sessionID string, commandLine string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil || r.current.SessionID != sessionID {
		return nil
	}

	// 过滤空命令
	trimmed := commandLine
	if len(trimmed) == 0 {
		return nil
	}

	// 计算相对时间戳
	timestamp := time.Since(r.current.StartTime).Milliseconds()

	cmd := RecordedCommand{
		Index:     len(r.current.Commands),
		Content:   commandLine,
		Timestamp: timestamp,
	}

	r.current.Commands = append(r.current.Commands, cmd)
	return nil
}

// RecordRawInput 记录原始终端输入（内部使用 LineBuffer 处理）
// 此方法接收原始字符数据，通过 LineBuffer 处理退格、光标移动等
func (r *Recorder) RecordRawInput(sessionID string, rawData string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return nil
	}

	// 获取或创建 LineBuffer
	lb, exists := r.lineBuffers[sessionID]
	if !exists {
		lb = terminal.NewLineBuffer()
		r.lineBuffers[sessionID] = lb
	}

	// 处理输入
	if line, committed := lb.Handle(rawData); committed {
		// 添加到时间线
		r.addTimelineEventLocked("terminal_input", line, map[string]interface{}{
			"session_id": sessionID,
		})

		// 同时添加到命令列表（仅当有内容时）
		if line != "" {
			r.addCommandLocked(line)
		}
	}

	return nil
}

// AddEvent 添加时间线事件
// 返回值: (命令内容, 是否提交, 错误)
// 当 terminal_input 事件触发命令提交时，返回命令内容供调用方使用
func (r *Recorder) AddEvent(eventType string, content string, metadata map[string]interface{}) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return "", false, fmt.Errorf("not recording")
	}

	// 对于 terminal_input 事件，使用 LineBuffer 处理
	if eventType == "terminal_input" {
		sessionID, ok := metadata["session_id"].(string)
		if !ok || sessionID == "" {
			// 如果没有 session ID，直接添加事件
			r.addTimelineEventLocked(eventType, content, metadata)
			return "", false, nil
		}

		lb, exists := r.lineBuffers[sessionID]
		if !exists {
			lb = terminal.NewLineBuffer()
			r.lineBuffers[sessionID] = lb
		}

		// 通过 LineBuffer 处理
		if line, committed := lb.Handle(content); committed {
			if line != "" {
				r.addTimelineEventLocked(eventType, line, metadata)
				r.addCommandLocked(line)
				return line, true, nil // 返回提交的命令
			}
			return "", true, nil // 空命令也返回提交状态
		}
		return "", false, nil
	}

	r.addTimelineEventLocked(eventType, content, metadata)
	return "", false, nil
}

// AddBroadcastInput 处理广播输入，对多个会话去重
func (r *Recorder) AddBroadcastInput(sessionIDs []string, content string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return fmt.Errorf("not recording")
	}

	// 按提交的行分组会话
	committedLines := make(map[string][]string)

	for _, sessionID := range sessionIDs {
		lb, exists := r.lineBuffers[sessionID]
		if !exists {
			lb = terminal.NewLineBuffer()
			r.lineBuffers[sessionID] = lb
		}

		if line, committed := lb.Handle(content); committed {
			if line != "" {
				committedLines[line] = append(committedLines[line], sessionID)
			}
		}
	}

	// 为唯一的提交行记录事件
	for line, sids := range committedLines {
		metadata := map[string]interface{}{
			"session_ids": sids,
			"broadcast":   true,
		}
		r.addTimelineEventLocked("terminal_input", line, metadata)
		r.addCommandLocked(line)
	}

	return nil
}

// addTimelineEventLocked 添加时间线事件（需要已持有锁）
func (r *Recorder) addTimelineEventLocked(eventType, content string, metadata map[string]interface{}) {
	event := TimelineEvent{
		Timestamp: time.Now(),
		Type:      eventType,
		Content:   content,
		Metadata:  metadata,
	}
	r.current.Timeline = append(r.current.Timeline, event)
}

// addCommandLocked 添加命令（需要已持有锁）
func (r *Recorder) addCommandLocked(commandLine string) {
	if r.current == nil {
		return
	}

	// 过滤空命令
	if len(commandLine) == 0 {
		return
	}

	timestamp := time.Since(r.current.StartTime).Milliseconds()
	cmd := RecordedCommand{
		Index:     len(r.current.Commands),
		Content:   commandLine,
		Timestamp: timestamp,
	}
	r.current.Commands = append(r.current.Commands, cmd)
}

// Stop 停止录制
func (r *Recorder) Stop() (*RecordingSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return nil, fmt.Errorf("not recording")
	}

	r.current.EndTime = time.Now()
	r.current.UpdatedAt = time.Now()

	// 保存到文件
	if err := r.save(r.current); err != nil {
		return nil, err
	}

	session := r.current
	r.current = nil
	return session, nil
}

// StopSession 停止故障排查会话（兼容旧 API）
func (r *Recorder) StopSession(rootCause string, conclusion string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return fmt.Errorf("no active session")
	}

	r.current.EndTime = time.Now()
	r.current.UpdatedAt = time.Now()
	r.current.RootCause = rootCause
	r.current.Conclusion = conclusion

	// 保存到文件
	err := r.save(r.current)
	if err != nil {
		return err
	}

	// 清除当前会话，允许后续录制
	r.current = nil
	return nil
}

// CancelSession 取消当前会话（不保存，仅清除状态）
func (r *Recorder) CancelSession() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return fmt.Errorf("no active session")
	}

	// 仅清除当前会话，不保存
	r.current = nil
	r.lineBuffers = make(map[string]*terminal.LineBuffer)
	return nil
}

// UpdateTimeline 更新时间线
func (r *Recorder) UpdateTimeline(events []TimelineEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return fmt.Errorf("no active session")
	}

	r.current.Timeline = events
	return nil
}

// SetSuggestions 设置建议
func (r *Recorder) SetSuggestions(suggestions []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return fmt.Errorf("no active session")
	}

	r.current.Suggestions = suggestions
	return nil
}

// GetLineBuffer 获取指定会话的 LineBuffer（用于测试）
func (r *Recorder) GetLineBuffer(sessionID string) *terminal.LineBuffer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lineBuffers[sessionID]
}

// GetStatus 获取录制状态
func (r *Recorder) GetStatus() RecorderStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	status := RecorderStatus{
		IsRecording:  r.current != nil,
		CommandCount: 0,
	}

	if r.current != nil {
		status.SessionID = r.current.SessionID
		status.Type = r.current.Type
		status.CommandCount = len(r.current.Commands)
		status.Duration = int64(time.Since(r.current.StartTime).Seconds())
	}

	return status
}

// GetCurrentSession 获取当前录制会话
func (r *Recorder) GetCurrentSession() *RecordingSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current
}

// save 保存录制会话到文件
func (r *Recorder) save(session *RecordingSession) error {
	dir := filepath.Join(r.storagePath, string(session.Type))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filename := filepath.Join(dir, fmt.Sprintf("recording_%s.json", session.ID))
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Load 加载录制会话
func (r *Recorder) Load(recType RecordingType, id string) (*RecordingSession, error) {
	filename := filepath.Join(r.storagePath, string(recType), fmt.Sprintf("recording_%s.json", id))

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var session RecordingSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// List 列出所有录制会话
func (r *Recorder) List(recType RecordingType) ([]*RecordingSession, error) {
	dir := filepath.Join(r.storagePath, string(recType))

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*RecordingSession{}, nil
		}
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	sessions := make([]*RecordingSession, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var session RecordingSession
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}

		sessions = append(sessions, &session)
	}

	return sessions, nil
}

// Delete 删除录制会话
func (r *Recorder) Delete(recType RecordingType, id string) error {
	filename := filepath.Join(r.storagePath, string(recType), fmt.Sprintf("recording_%s.json", id))

	if err := os.Remove(filename); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

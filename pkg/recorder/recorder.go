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
		ID:        uuid.New().String(),
		Type:      recType,
		StartTime: time.Now(),
		SessionID: sessionID,
		Host:      host,
		User:      user,
		Commands:  make([]RecordedCommand, 0),
		Metadata:  make(map[string]interface{}),
	}

	r.current = session
	return session, nil
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

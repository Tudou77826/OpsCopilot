package session_recorder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Recorder struct {
	currentSession *TroubleshootingSession
	storagePath    string
	mu             sync.Mutex
}

func NewRecorder(storagePath string) *Recorder {
	return &Recorder{
		storagePath: storagePath,
	}
}

// StartSession initializes a new troubleshooting session
func (r *Recorder) StartSession(problem string, context []string) *TroubleshootingSession {
	r.mu.Lock()
	defer r.mu.Unlock()

	session := &TroubleshootingSession{
		ID:        uuid.New().String(),
		StartTime: time.Now(),
		Problem:   problem,
		Context:   context,
		Timeline:  make([]TimelineEvent, 0),
	}
	r.currentSession = session
	return session
}

// AddEvent adds an event to the current session's timeline
func (r *Recorder) AddEvent(eventType, content string, metadata map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	// 优化：聚合连续的 terminal_input 事件，直到遇到回车符 (\r 或 \n)
	// 即使中间夹杂了 terminal_output，我们也尝试向前查找最后一条 terminal_input 进行聚合
	if eventType == "terminal_input" {
		// 1. 过滤退格符 (\x7f 或 \b)
		if content == "\x7f" || content == "\b" {
			// 向前查找最近的一条 input
			for i := len(r.currentSession.Timeline) - 1; i >= 0; i-- {
				lastEvent := &r.currentSession.Timeline[i]
				if lastEvent.Type == "terminal_input" {
					if len(lastEvent.Content) > 0 {
						// 移除最后一个字符
						lastEvent.Content = lastEvent.Content[:len(lastEvent.Content)-1]
						lastEvent.Timestamp = time.Now()
					}
					return nil // 找到了就处理并返回
				}
				// 如果遇到非 input/output 的事件（如 user_query），则停止回溯，不处理退格（或者认为退格无效）
				if lastEvent.Type != "terminal_output" {
					break
				}
			}
			return nil // 没找到可删除的 input
		}

		// 2. 尝试聚合
		// 从后往前找最近的一条 terminal_input
		var targetEvent *TimelineEvent

		for i := len(r.currentSession.Timeline) - 1; i >= 0; i-- {
			e := &r.currentSession.Timeline[i]
			if e.Type == "terminal_input" {
				targetEvent = e
				break
			}
			// 如果遇到除了 terminal_output 之外的其他事件（例如 user_query, ai_suggestion），则打断聚合
			// 这意味着上下文已经变了，不能把 input 聚合到“很久以前”的 input 上
			if e.Type != "terminal_output" {
				break
			}
		}

		if targetEvent != nil {
			// 检查该 input 是否已经包含回车
			hasNewline := false
			for _, c := range targetEvent.Content {
				if c == '\r' || c == '\n' {
					hasNewline = true
					break
				}
			}

			// 如果还没有回车，则追加到该记录
			if !hasNewline {
				targetEvent.Content += content
				targetEvent.Timestamp = time.Now()
				return nil
			}
		}
	}

	// 忽略空内容的 terminal_input (除了回车)
	if eventType == "terminal_input" && content == "" {
		return nil
	}

	event := TimelineEvent{
		Timestamp: time.Now(),
		Type:      eventType,
		Content:   content,
		Metadata:  metadata,
	}
	r.currentSession.Timeline = append(r.currentSession.Timeline, event)
	return nil
}

// StopSession ends the current session and saves it
func (r *Recorder) StopSession(rootCause string, conclusion string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	r.currentSession.EndTime = time.Now()
	r.currentSession.RootCause = rootCause
	r.currentSession.Conclusion = conclusion

	// Save to disk
	if err := r.saveSessionLocked(); err != nil {
		return err
	}

	// Session is stopped but kept in memory until next StartSession or explicit clear
	return nil
}

// saveSessionLocked saves the current session to a JSON file
func (r *Recorder) saveSessionLocked() error {
	if r.currentSession == nil {
		return nil
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(r.storagePath, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	filename := filepath.Join(r.storagePath, fmt.Sprintf("session_%s.json", r.currentSession.ID))
	data, err := json.MarshalIndent(r.currentSession, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// GetCurrentSession returns the current session (thread-safe)
func (r *Recorder) GetCurrentSession() *TroubleshootingSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentSession
}

// UpdateTimeline updates the timeline of the current session
func (r *Recorder) UpdateTimeline(events []TimelineEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	r.currentSession.Timeline = events
	return nil
}

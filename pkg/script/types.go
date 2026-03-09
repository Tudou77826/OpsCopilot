package script

import (
	"opscopilot/pkg/recorder"
	"time"
)

// Script 命令脚本（基于RecordingSession）
type Script struct {
	ID          string                 `json:"id"`
	Type        recorder.RecordingType `json:"type"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at,omitempty"`
	SessionID   string                 `json:"session_id"`
	Host        string                 `json:"host"`
	User        string                 `json:"user"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Timeline    []recorder.TimelineEvent `json:"timeline,omitempty"`
	Problem     string                 `json:"problem,omitempty"`
	Context     []string               `json:"context,omitempty"`
	RootCause   string                 `json:"root_cause,omitempty"`
	Conclusion  string                 `json:"conclusion,omitempty"`
	Suggestions []string               `json:"suggestions,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Commands    []ScriptCommand        `json:"commands"`
}

// ScriptCommand 可编辑的脚本命令
type ScriptCommand struct {
	recorder.RecordedCommand // 嵌入基础命令

	Comment string `json:"comment"` // 备注说明
	Delay   int    `json:"delay"`   // 执行前延迟（毫秒）
	Enabled bool   `json:"enabled"` // 是否启用
}

// NewScript 从录制会话创建脚本
func NewScript(base *recorder.RecordingSession, name, description string) *Script {
	// 转换命令
	commands := make([]ScriptCommand, len(base.Commands))
	for i, cmd := range base.Commands {
		commands[i] = ScriptCommand{
			RecordedCommand: cmd,
			Comment:         "",
			Delay:           0,
			Enabled:         true,
		}
	}

	script := &Script{
		Name:        name,
		Description: description,
		Commands:    commands,
	}
	script.SyncFromRecordingSession(base)
	return script
}

func (s *Script) SyncFromRecordingSession(base *recorder.RecordingSession) {
	s.ID = base.ID
	s.Type = base.Type
	s.StartTime = base.StartTime
	s.EndTime = base.EndTime
	s.UpdatedAt = base.UpdatedAt
	s.SessionID = base.SessionID
	s.Host = base.Host
	s.User = base.User
	s.Metadata = base.Metadata
	s.Timeline = base.Timeline
	s.Problem = base.Problem
	s.Context = base.Context
	s.RootCause = base.RootCause
	s.Conclusion = base.Conclusion
	s.Suggestions = base.Suggestions
}

// ScriptStatus 脚本录制状态
type ScriptStatus struct {
	IsRecording  bool   `json:"is_recording"`
	ScriptID     string `json:"script_id,omitempty"`
	Name         string `json:"name,omitempty"`
	CommandCount int    `json:"command_count"`
	Duration     int64  `json:"duration"` // 已录制时长（秒）
}

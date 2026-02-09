package recorder

import "time"

// RecordingType 录制类型
type RecordingType string

const (
	RecordingTypeTroubleshoot RecordingType = "troubleshoot" // 故障排查
	RecordingTypeScript        RecordingType = "script"       // 命令脚本
)

// RecordingSession 录制会话（基类）
type RecordingSession struct {
	ID        string            `json:"id"`
	Type      RecordingType     `json:"type"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time,omitempty"`
	UpdatedAt time.Time         `json:"updated_at,omitempty"`
	SessionID string            `json:"session_id"` // SSH会话ID
	Host      string            `json:"host"`       // 主机信息
	User      string            `json:"user"`       // 用户信息
	Commands  []RecordedCommand `json:"commands"`   // 录制的命令列表
	Metadata  map[string]interface{} `json:"metadata,omitempty"` // 扩展元数据
}

// RecordedCommand 录制的单条命令
type RecordedCommand struct {
	Index     int    `json:"index"`     // 命令序号
	Content   string `json:"content"`   // 命令内容
	Output    string `json:"output,omitempty"` // 命令输出（可选）
	Timestamp int64  `json:"timestamp"` // 录制时间戳（相对开始时间的毫秒数）
	Duration  int    `json:"duration,omitempty"` // 执行时长（毫秒，可选）
}

// RecorderStatus 录制器状态
type RecorderStatus struct {
	IsRecording  bool          `json:"is_recording"`
	SessionID    string        `json:"session_id,omitempty"`
	Type         RecordingType `json:"type,omitempty"`
	CommandCount int           `json:"command_count"`
	Duration     int64         `json:"duration"` // 已录制时长（秒）
}

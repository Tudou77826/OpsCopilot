package script

import (
	"time"

	"opscopilot/pkg/recorder"
)

// Script 命令脚本
type Script struct {
	ID          string    `json:"id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	Host        string    `json:"host"`
	User        string    `json:"user"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Commands    []ScriptCommand `json:"commands"`

	// 变量和步骤树
	Variables []ScriptVariable `json:"variables,omitempty"`
	Steps     []ScriptStep     `json:"steps,omitempty"`
}

// ScriptCommand 可编辑的脚本命令
type ScriptCommand struct {
	Content string `json:"content"`
	Comment string `json:"comment"`
	Delay   int    `json:"delay"`
	Enabled bool   `json:"enabled"`
}

// NewScript 从录制会话创建脚本
func NewScript(base *recorder.RecordingSession, name, description string) *Script {
	commands := make([]ScriptCommand, len(base.Commands))
	for i, cmd := range base.Commands {
		commands[i] = ScriptCommand{
			Content: cmd.Content,
			Comment: "",
			Delay:   0,
			Enabled: true,
		}
	}

	return &Script{
		ID:          base.ID,
		StartTime:   base.StartTime,
		EndTime:     base.EndTime,
		UpdatedAt:   base.UpdatedAt,
		Host:        base.Host,
		User:        base.User,
		Name:        name,
		Description: description,
		Commands:    commands,
	}
}

// ScriptVariable 脚本变量定义
type ScriptVariable struct {
	Name         string `json:"name"`          // 变量名，如 port, service_name
	DisplayName  string `json:"display_name"`  // UI 显示名，如 "端口号"
	DefaultValue string `json:"default_value"` // 默认值
	Required     bool   `json:"required"`      // 是否必填
	Description  string `json:"description"`   // 说明文字
}

// ScriptStep 脚本步骤
type ScriptStep struct {
	Command string `json:"command,omitempty"`
	Comment string `json:"comment,omitempty"`
	Delay   int    `json:"delay,omitempty"`
	Enabled bool   `json:"enabled"`

	// 兼容：映射回原始录制序号
	OriginalIndex int `json:"original_index,omitempty"`
}

// ScriptStatus 脚本录制状态
type ScriptStatus struct {
	IsRecording  bool   `json:"is_recording"`
	ScriptID     string `json:"script_id,omitempty"`
	Name         string `json:"name,omitempty"`
	CommandCount int    `json:"command_count"`
	Duration     int64  `json:"duration"` // 已录制时长（秒）
}

// MigrateCommandsToSteps 将旧的 Commands 迁移为 Steps（向后兼容）
func (s *Script) MigrateCommandsToSteps() {
	if len(s.Steps) > 0 {
		return
	}
	if len(s.Commands) == 0 {
		return
	}

	s.Steps = make([]ScriptStep, len(s.Commands))
	for i, cmd := range s.Commands {
		s.Steps[i] = ScriptStep{
			Command:       cmd.Content,
			Comment:       cmd.Comment,
			Delay:         cmd.Delay,
			Enabled:       cmd.Enabled,
			OriginalIndex: i,
		}
	}
}

// SyncStepsToCommands 将 Steps 同步回 Commands（保存时保持旧客户端兼容）
func (s *Script) SyncStepsToCommands() {
	if len(s.Steps) == 0 {
		return
	}
	commands := make([]ScriptCommand, len(s.Steps))
	for i, step := range s.Steps {
		commands[i] = ScriptCommand{
			Content: step.Command,
			Comment: step.Comment,
			Delay:   step.Delay,
			Enabled: step.Enabled,
		}
	}
	s.Commands = commands
}

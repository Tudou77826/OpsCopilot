package script

import (
	"opscopilot/pkg/recorder"
)

// Script 命令脚本（基于RecordingSession）
type Script struct {
	recorder.RecordingSession // 嵌入基础录制会话

	Name        string          `json:"name"`        // 脚本名称
	Description string          `json:"description"`  // 脚本描述
	Commands    []ScriptCommand `json:"commands"`    // 可编辑的命令列表
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

	return &Script{
		RecordingSession: *base,
		Name:             name,
		Description:      description,
		Commands:         commands,
	}
}

// ScriptStatus 脚本录制状态
type ScriptStatus struct {
	IsRecording  bool   `json:"is_recording"`
	ScriptID     string `json:"script_id,omitempty"`
	Name         string `json:"name,omitempty"`
	CommandCount int    `json:"command_count"`
	Duration     int64  `json:"duration"` // 已录制时长（秒）
}

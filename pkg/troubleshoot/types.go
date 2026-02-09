package troubleshoot

import (
	"opscopilot/pkg/recorder"
)

// Case 故障排查案例（基于RecordingSession）
type Case struct {
	recorder.RecordingSession // 嵌入基础录制会话

	Problem     string   `json:"problem"`     // 问题描述
	Context     []string `json:"context"`     // 加载的文档上下文
	RootCause   string   `json:"root_cause"`  // 根因分析
	Conclusion  string   `json:"conclusion"`  // 结论
	Suggestions []string `json:"suggestions"` // AI建议
}

// NewCase 从录制会话创建故障排查案例
func NewCase(base *recorder.RecordingSession, problem string, context []string) *Case {
	return &Case{
		RecordingSession: *base,
		Problem:          problem,
		Context:          context,
		Suggestions:      make([]string, 0),
	}
}

// CaseStatus 案例状态
type CaseStatus struct {
	IsActive     bool   `json:"is_active"`
	CaseID       string `json:"case_id,omitempty"`
	Problem      string `json:"problem,omitempty"`
	CommandCount int    `json:"command_count"`
	Duration     int64  `json:"duration"` // 已进行时长（秒）
}

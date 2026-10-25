package assistanttools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantlocal"
)

type LocalDirectoryTools struct {
	Turn     *assistantcontrol.Turn
	Projects *assistantlocal.Projects
	denied   bool
}

func (l *LocalDirectoryTools) Call(ctx context.Context, name, argument string) (any, error) {
	if ctx.Err() != nil || l.Turn.Context().Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	if name == "local_directories" {
		return l.Projects.Snapshot()
	}
	var in struct {
		ProjectID string   `json:"project_id"`
		Paths     []string `json:"paths"`
		Purpose   string   `json:"purpose"`
	}
	if name != "local_add_directories" || json.Unmarshal([]byte(argument), &in) != nil || in.ProjectID == "" || len(in.ProjectID) > 128 || strings.TrimSpace(in.Purpose) == "" || len(in.Purpose) > 512 {
		return nil, errors.New("请提供明确的项目、本地目录及添加原因")
	}
	if l.denied {
		return nil, errors.New("本轮已拒绝追加目录，请等待用户指示")
	}
	state, err := l.Projects.Snapshot()
	if err != nil {
		return nil, err
	}
	var project *assistantlocal.Project
	for _, p := range state.Projects {
		if p.ID == in.ProjectID {
			v := p
			project = &v
			break
		}
	}
	if project == nil {
		return nil, errors.New("目标项目不存在，请先选择或创建项目")
	}
	paths, err := assistantlocal.Normalize(in.Paths)
	if err != nil {
		return nil, err
	}
	details := []assistantcontrol.Detail{}
	for _, p := range paths {
		details = append(details, assistantcontrol.Detail{Label: "本地目录", Value: p})
	}
	answer, err := l.Turn.Request(ctx, assistantcontrol.Prompt{Kind: "approval", Title: "将本地目录加入项目「" + project.Name + "」", Description: "用途：" + in.Purpose + "\n目录将保存到此项目，新建对话不清空。不自动读取内容，文件修改、命令执行及敏感内容访问仍按现有规则确认。", Details: details, Choices: []assistantcontrol.Choice{{ID: "reject", Label: "拒绝"}, {ID: "allow", Label: "添加到此项目"}}, Expires: time.Now().Add(5 * time.Minute)})
	if err != nil {
		return nil, err
	}
	if len(answer.Selected) != 1 || answer.Selected[0] != "allow" {
		l.denied = true
		return nil, errors.New("未添加本地目录")
	}
	if ctx.Err() != nil || l.Turn.Context().Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	// Validate again after consent, so disappearing directories or changed
	// symlinks cannot silently become a different approved location.
	current, err := assistantlocal.Normalize(in.Paths)
	if err != nil {
		return nil, err
	}
	if strings.Join(paths, "\x00") != strings.Join(current, "\x00") {
		return nil, errors.New("目录位置已变化，请重新确认")
	}
	return l.Projects.Add(in.ProjectID, paths)
}

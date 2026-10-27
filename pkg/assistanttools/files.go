package assistanttools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"opscopilot/pkg/agenttransport/mcp"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/filecontrol"
	"opscopilot/pkg/terminalcontrol"
)

type FileTools struct {
	Turn                *assistantcontrol.Turn
	Service             *filecontrol.Service
	Inventory           func(context.Context) (any, error)
	Review              assistantcontrol.Reviewer
	Resolve             func(context.Context, string) (filecontrol.Endpoint, error)
	current             string
	denied              map[string]bool
	consented           map[string]bool
	tasks               map[string]string // task ID -> workspace, only tasks initiated this turn
	downloadNoOverwrite bool
}

func (f *FileTools) Close() {
	if f.current != "" {
		f.Service.Release(f.current, f.Turn.ID())
		f.observeTasks(f.current)
		f.current = ""
	}
}
func (f *FileTools) confirm(ctx context.Context, title, description string, details []assistantcontrol.Detail, key string, humanReasons ...string) error {
	if f.denied == nil {
		f.denied = map[string]bool{}
	}
	if f.denied[key] {
		return fmt.Errorf("%w：本轮已拒绝此文件操作，请等待用户指示", assistantcontrol.ErrActionDenied)
	}
	data, _ := json.Marshal(details)
	humanReason := ""
	if strings.HasPrefix(key, "upload\x00") {
		humanReason = "上传可能覆盖远端同名文件，当前通道不能保证不覆盖，需要人工核对"
	}
	if len(humanReasons) > 0 && humanReasons[0] != "" {
		humanReason = humanReasons[0]
	}
	input := assistantcontrol.ReviewInput{Tool: "file", Location: "文件工作区", Action: title + "\n" + description + "\n" + string(data)}
	verdict, err := f.Turn.ReviewAction(ctx, f.Review, input, humanReason)
	if err == nil {
		err = f.Turn.ConfirmAction(ctx, input, assistantcontrol.Prompt{Kind: "approval", Title: title, Description: description, Details: details, Choices: []assistantcontrol.Choice{{ID: "reject", Label: "取消"}, {ID: "allow", Label: "允许本次"}}, Expires: time.Now().Add(5 * time.Minute)}, verdict)
	}
	if err != nil {
		if errors.Is(err, assistantcontrol.ErrActionDenied) {
			f.denied[key] = true
		}
		return err
	}
	f.downloadNoOverwrite = strings.HasPrefix(key, "download\x00") && verdict.Allowed()
	if ctx.Err() != nil || f.Turn.Context().Err() != nil {
		return assistantcontrol.ErrStale
	}
	return nil
}
func (f *FileTools) Call(ctx context.Context, name, argument string) (value any, callErr error) {
	defer func() {
		if callErr != nil && !hasCallReceipt(ctx) {
			if errors.Is(callErr, assistantcontrol.ErrActionDenied) {
				title := "文件操作"
				if name == "file_transfer" {
					title = "文件传输"
				}
				f.Turn.ObserveOperation("file-error-"+fmt.Sprint(len(f.Turn.Snapshot().Messages)), title, assistantcontrol.Operation{Kind: "file", Status: "denied"})
				return
			}
			stage := "文件目录 / 页签"
			if name == "file_transfer" {
				stage = "传输准备"
			}
			f.Turn.ObserveOperation("file-error-"+fmt.Sprint(len(f.Turn.Snapshot().Messages)), "文件操作未完成", assistantcontrol.Operation{Kind: "file", Status: "failed", Stage: stage, Outcome: "本次工具调用未完成；此前的任务结果保留在传输记录中。", Next: "核对诊断详情与传输队列后再决定下一步。", Detail: callErr.Error()})
		}
	}()
	if ctx.Err() != nil || f.Turn.Context().Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	var in mcp.FileArgs
	if json.Unmarshal([]byte(argument), &in) != nil {
		return nil, errors.New("文件参数无效")
	}
	switch name {
	case "file_workspaces":
		return f.Inventory(ctx)
	case "file_open":
		f.Close()
		e, err := f.Resolve(ctx, in.SessionID)
		if err != nil {
			return nil, err
		}
		if err = f.allowBrowse(ctx, e); err != nil {
			return nil, err
		}
		w, err := f.Service.Open(ctx, e)
		if err != nil {
			return nil, err
		}
		if err = f.Service.Present(ctx, w.ID); err != nil {
			return nil, err
		}
		f.current = w.ID
		f.Turn.ObserveOperation("file-"+w.ID, "文件页签已打开 · "+e.Title, assistantcontrol.Operation{Kind: "file", WorkspaceID: w.ID, Status: "completed"})
		return f.Service.Snapshot(w.ID)
	case "file_take":
		f.Close()
		w, err := f.Service.Snapshot(in.WorkspaceID)
		if err != nil {
			return nil, err
		}
		e, err := f.Resolve(ctx, w.Endpoint.SessionID)
		if err != nil || e != w.Endpoint {
			return nil, filecontrol.ErrStale
		}
		if err = f.allowBrowse(ctx, e); err != nil {
			return nil, err
		}
		if err = f.Service.Present(ctx, w.ID); err != nil {
			return nil, err
		}
		if err = f.Service.Take(f.Turn.Context(), w.ID, f.Turn.ID()); err != nil {
			return nil, err
		}
		f.current = w.ID
		if err = f.Service.Present(ctx, w.ID); err != nil {
			f.Close()
			return nil, err
		}
		if err = f.Turn.ChangeTarget(assistantcontrol.Target{TabID: w.ID, ConnectionID: e.ConnectionID}); err != nil {
			f.Close()
			return nil, err
		}
		return f.Service.Snapshot(w.ID)
	case "file_release":
		f.Close()
		return map[string]bool{"owned": false}, nil
	case "file_tasks":
		if f.current == "" {
			return nil, errors.New("请先打开文件页签")
		}
		w, err := f.Service.Snapshot(f.current)
		f.observeTasks(f.current)
		return map[string]any{"id": w.ID, "tasks": w.Tasks, "ownerTurn": w.OwnerTurn, "error": w.Error}, err
	}
	if f.current == "" {
		return nil, errors.New("请先打开并接管文件页签")
	}
	switch name {
	case "file_list":
		if in.Side != "local" && in.Side != "remote" {
			return nil, errors.New("side 必须是 local 或 remote")
		}
		if err := validFilePath(in.Path, in.Side == "local"); err != nil {
			return nil, err
		}
		w, err := f.Service.Snapshot(f.current)
		if err != nil {
			return nil, err
		}
		if f.Review != nil {
			humanReason := ""
			if sensitiveFilePath(in.Path) {
				humanReason = "目录可能包含敏感凭据，需要人工确认"
			}
			if err = f.confirm(ctx, "浏览目录", "只列目录元信息，不读取文件内容。", []assistantcontrol.Detail{{Label: "目标节点", Value: w.Endpoint.Title + " / " + w.Endpoint.ConnectionID}, {Label: "位置", Value: in.Side}, {Label: "目录", Value: in.Path}}, "list:"+in.Side+":"+in.Path, humanReason); err != nil {
				return nil, err
			}
		}
		return f.Service.Browse(ctx, f.current, f.Turn.ID(), in.Path, in.Side == "local")
	case "file_transfer":
		if in.Direction != "upload" && in.Direction != "download" {
			return nil, errors.New("传输方向无效")
		}
		if err := validFilePath(in.LocalPath, true); err != nil {
			return nil, err
		}
		if err := validFilePath(in.RemotePath, false); err != nil {
			return nil, err
		}
		w, err := f.Service.Snapshot(f.current)
		if err != nil || w.OwnerTurn != f.Turn.ID() {
			return nil, filecontrol.ErrStale
		}
		if _, err = f.Service.Browse(ctx, w.ID, f.Turn.ID(), filepath.Dir(in.LocalPath), true); err != nil {
			return nil, err
		}
		if _, err = f.Service.Browse(ctx, w.ID, f.Turn.ID(), path.Dir(in.RemotePath), false); err != nil {
			return nil, err
		}
		key := in.Direction + "\x00" + in.LocalPath + "\x00" + in.RemotePath
		description := "仅上传一个文件，可能覆盖远端同名文件。不包含执行或安装。取消/失败可能留下部分文件。"
		humanReason := ""
		if sensitiveFilePath(in.LocalPath) || sensitiveFilePath(in.RemotePath) {
			humanReason = "传输路径可能涉及敏感凭据，需要人工确认"
		}
		if in.Direction == "download" {
			description = "仅下载一个文件；自动通过时使用不覆盖发布，若目标已经存在或安全发布不受支持则失败，不替换已有文件。不包含执行或安装。"
			listed, listErr := f.Service.Snapshot(w.ID)
			if listErr != nil {
				return nil, listErr
			}
			for _, entry := range listed.LocalEntries {
				if entry.Name == filepath.Base(in.LocalPath) {
					humanReason = "下载目标已存在，覆盖已有文件需要人工确认"
					description = "下载并覆盖已有本机文件，需要明确确认。"
					break
				}
			}
		}
		if err = f.confirm(ctx, "确认这一次文件传输", description, []assistantcontrol.Detail{{Label: "目标节点", Value: w.Endpoint.Title + " / " + w.Endpoint.ConnectionID}, {Label: "执行身份", Value: w.Endpoint.Identity}, {Label: "传输方向", Value: map[string]string{"upload": "本机 → 服务器", "download": "服务器 → 本机"}[in.Direction]}, {Label: "本机文件", Value: in.LocalPath}, {Label: "远端文件", Value: in.RemotePath}, {Label: "目的", Value: in.Purpose}}, key, humanReason); err != nil {
			return nil, err
		}
		if err = f.Service.Present(ctx, w.ID); err != nil {
			return nil, err
		}
		task, err := f.Service.Start(ctx, w.ID, f.Turn.ID(), filecontrol.Task{Direction: in.Direction, LocalPath: in.LocalPath, RemotePath: in.RemotePath, NoOverwrite: f.downloadNoOverwrite})
		if err == nil {
			if f.tasks == nil {
				f.tasks = map[string]string{}
			}
			f.tasks[task.ID] = w.ID
			f.observeTasks(w.ID)
		}
		return task, err
	case "file_cancel":
		return nil, f.Service.Cancel(f.current, f.Turn.ID(), in.TaskID)
	}
	return nil, errors.New("未知文件操作")
}

func sensitiveFilePath(value string) bool {
	encoded, _ := json.Marshal(map[string]string{"path": value})
	return terminalcontrol.RequiresHumanReview(string(encoded))
}
func (f *FileTools) observeTasks(id string) {
	w, err := f.Service.Snapshot(id)
	if err != nil {
		return
	}
	for _, task := range w.Tasks {
		if f.tasks[task.ID] != id {
			continue
		}
		op := assistantcontrol.Operation{Kind: "file", WorkspaceID: id, ItemID: task.ID, Status: task.Status, Stage: "文件传输"}
		op.Action, op.Location = task.LocalPath+" → "+task.RemotePath, w.Endpoint.Title
		if task.Direction == "download" {
			op.Action = task.RemotePath + " → " + task.LocalPath
		}
		name := filepath.Base(task.LocalPath)
		title := "正在传输 · " + name
		switch task.Status {
		case "done":
			title = "传输完成 · " + name
			op.Outcome = fmt.Sprintf("已确认传输完成，共 %d 字节。", task.Bytes)
		case "failed", "cancelled":
			title = "传输未完成 · " + name
			op.Outcome = "未确认完整传输成功，目标可能保留部分文件。"
			op.Next = "检查传输记录和目标文件后再决定是否重传。"
			op.Detail = task.Message
		default:
			op.Outcome = "任务已启动，尚未确认传输完成。"
		}
		if task.Transferred && task.Status != "done" {
			title, op.Status = "传输已完成，结果记录异常 · "+name, "warning"
			op.Outcome = fmt.Sprintf("传输通道已确认完成，共 %d 字节；结果审计未完整保存。", task.Bytes)
			op.Next = "先核对实际文件，不要直接重复传输。"
		}
		if task.Transferred {
			op.Result = fmt.Sprintf("已传输 %d 字节", task.Bytes)
		}
		f.Turn.ObserveOperation(task.ID, title, op)
	}
}
func (f *FileTools) allowBrowse(ctx context.Context, e filecontrol.Endpoint) error {
	if f.consented == nil {
		f.consented = map[string]bool{}
	}
	if f.consented[e.ConnectionID] {
		return nil
	}
	if err := f.confirm(ctx, "允许助手浏览文件目录？", "允许本轮浏览本机与此 SSH 节点的目录元信息，不读取文件内容。上传下载分别确认，文件访问不继承终端中临时切换的账号。", []assistantcontrol.Detail{{Label: "目标节点", Value: e.Title}, {Label: "执行身份", Value: e.Identity}}, "access:"+e.ConnectionID); err != nil {
		return err
	}
	if err := f.Service.Record(ctx, "", f.Turn.ID(), "browse_consent", "", e.ConnectionID, ""); err != nil {
		return err
	}
	f.consented[e.ConnectionID] = true
	return nil
}
func validFilePath(p string, local bool) error {
	if strings.TrimSpace(p) == "" || len(p) > 2048 || strings.ContainsAny(p, "\x00\r\n") {
		return errors.New("文件路径无效")
	}
	if local {
		if !filepath.IsAbs(p) {
			return errors.New("本机路径必须为绝对路径")
		}
	} else if !path.IsAbs(p) {
		return errors.New("远端路径必须为绝对路径")
	}
	return nil
}

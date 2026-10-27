package assistanttools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantreference"
	"opscopilot/pkg/terminalcontrol"
	"opscopilot/pkg/workspace"
)

type ConnectionPlan struct {
	Connection workspace.Connection
	Open       func(context.Context) (workspace.Tab, error)
}

type terminalDenials struct {
	read bool
	take bool
}

// WorkspaceTools serializes explicit target changes. A captured call belongs
// to its original target generation, even if its request body arrives late.
type WorkspaceTools struct {
	References         *ReferenceTools
	LocalDirectories   *LocalDirectoryTools
	Files              *FileTools
	Database           *DatabaseTools
	turn               *assistantcontrol.Turn
	list               func(context.Context) (workspace.Inventory, error)
	prepare            func(context.Context, string) (ConnectionPlan, error)
	selectTab          func(context.Context, string) (workspace.Tab, error)
	terminal           func(terminalcontrol.Target) *TerminalTools
	mu                 sync.Mutex
	generation         atomic.Uint64
	receiptSequence    atomic.Uint64
	current            *TerminalTools
	closed             bool
	denied             map[string]bool
	deniedTargets      map[terminalcontrol.Target]terminalDenials
	OnOperation        func(action, phase string, target terminalcontrol.Target)
	InitialPageContext string
	Review             assistantcontrol.Reviewer
}

func (w *WorkspaceTools) MessageContext() string { return w.InitialPageContext }

func NewWorkspaceTools(turn *assistantcontrol.Turn, target terminalcontrol.Target, list func(context.Context) (workspace.Inventory, error), prepare func(context.Context, string) (ConnectionPlan, error), selectTab func(context.Context, string) (workspace.Tab, error), terminal func(terminalcontrol.Target) *TerminalTools) *WorkspaceTools {
	return &WorkspaceTools{turn: turn, list: list, prepare: prepare, selectTab: selectTab, terminal: terminal, current: terminal(target), denied: make(map[string]bool), deniedTargets: make(map[terminalcontrol.Target]terminalDenials)}
}
func (w *WorkspaceTools) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	if w.Files != nil {
		w.Files.Close()
	}
	if w.Database != nil {
		w.Database.Close()
	}
	w.generation.Add(1)
	if w.current != nil {
		w.current.Close()
		w.current = nil
	}
}
func (w *WorkspaceTools) CaptureCall() func(context.Context, string, string, string) (any, error) {
	generation := w.generation.Load()
	return func(ctx context.Context, name, argument, purpose string) (any, error) {
		return w.call(ctx, generation, name, argument, purpose)
	}
}
func (w *WorkspaceTools) Call(ctx context.Context, name, argument string) (any, error) {
	return w.CallWithPurpose(ctx, name, argument, "")
}
func (w *WorkspaceTools) CallWithPurpose(ctx context.Context, name, argument, purpose string) (any, error) {
	return w.CaptureCall()(ctx, name, argument, purpose)
}
func (w *WorkspaceTools) call(ctx context.Context, generation uint64, name, argument, purpose string) (result any, callErr error) {
	if !w.mu.TryLock() {
		return nil, errors.New("工作台已有操作进行中，请顺序调用")
	}
	defer w.mu.Unlock()
	if w.closed || generation != w.generation.Load() || ctx.Err() != nil || w.turn.Context().Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	finishReceipt := w.beginCallReceipt(name, argument)
	defer func() { finishReceipt(result, callErr) }()
	if !hasDetailedReceipt(name) {
		ctx = context.WithValue(ctx, callReceiptKey{}, true)
	}
	for _, tool := range assistantreference.ToolNames() {
		if name == tool {
			if w.References == nil {
				return nil, errors.New("当前宿主不支持知识与快捷命令读取")
			}
			return w.References.Call(ctx, name, argument)
		}
	}
	if name == "local_directories" || name == "local_add_directories" {
		if w.LocalDirectories == nil {
			return nil, errors.New("当前宿主不支持本地目录列表")
		}
		return w.LocalDirectories.Call(ctx, name, argument)
	}
	if strings.HasPrefix(name, "file_") {
		if w.Files == nil {
			return nil, errors.New("当前宿主不支持文件工作台")
		}
		if name == "file_open" || name == "file_take" {
			w.generation.Add(1)
			defer w.generation.Add(1)
			if w.current != nil {
				w.current.Close()
				w.current = nil
			}
			if w.Database != nil {
				w.Database.Close()
			}
			if err := w.turn.ChangeTarget(assistantcontrol.Target{}); err != nil {
				return nil, err
			}
		}
		value, err := w.Files.Call(ctx, name, argument)
		if encoded, marshalErr := json.Marshal(value); marshalErr == nil && len(encoded) > 64<<10 {
			value = map[string]any{"truncated": true, "preview": strings.ToValidUTF8(string(encoded[:64<<10]), "�"), "message": "完整目录保留在文件页签；请缩小范围。日志分析请使用终端"}
		}
		return value, err
	}
	if strings.HasPrefix(name, "database_") {
		if w.Database == nil {
			return nil, errors.New("当前宿主不支持数据库工作台")
		}
		if name == "database_open" || name == "database_take" {
			if w.Files != nil {
				w.Files.Close()
			}
			w.generation.Add(1)
			defer w.generation.Add(1)
			if w.current != nil {
				w.current.Close()
				w.current = nil
			}
			if err := w.turn.ChangeTarget(assistantcontrol.Target{}); err != nil {
				return nil, err
			}
		}
		value, err := w.Database.Call(ctx, name, argument)
		// The full result stays in the normal UI; only bounded observations enter
		// model context. This is not a second database execution path.
		if encoded, marshalErr := json.Marshal(value); marshalErr == nil && len(encoded) > 64<<10 {
			value = map[string]any{"truncated": true, "preview": strings.ToValidUTF8(string(encoded[:64<<10]), "�"), "message": "完整内容保留在数据库页签；请缩小查询范围"}
		}
		return value, err
	}
	switch name {
	case "workspace_list":
		inventory, err := w.list(ctx)
		if w.current != nil {
			inventory.ToolTarget = workspace.Target{TabID: w.current.Target.SessionID, ConnectionID: w.current.Target.ConnectionID}
		}
		return inventory, err
	case "workspace_open", "workspace_select":
		if w.Files != nil {
			w.Files.Close()
		}
		if w.Database != nil {
			w.Database.Close()
		}
		phase := "failed"
		auditTarget := terminalcontrol.Target{}
		if w.OnOperation != nil {
			w.OnOperation(name, "requested", auditTarget)
		}
		defer func() {
			w.turn.SetActivity("")
			if w.OnOperation != nil {
				w.OnOperation(name, phase, auditTarget)
			}
		}()
		if name == "workspace_open" {
			w.turn.SetActivity("opening")
		} else {
			w.turn.SetActivity("selecting")
		}
		// Invalidate both requests captured before and during this transition.
		w.generation.Add(1)
		defer w.generation.Add(1)
		if w.current != nil {
			// Permission is never carried forward, but a refusal remains binding
			// for the same actual connection throughout this turn.
			w.deniedTargets[w.current.Target] = terminalDenials{read: w.current.readDenied, take: w.current.denied}
			w.current.Close()
			w.current = nil
		}
		if err := w.turn.ChangeTarget(assistantcontrol.Target{}); err != nil {
			return nil, err
		}
		var tab workspace.Tab
		var err error
		if name == "workspace_select" {
			tab, err = w.selectTab(ctx, argument)
		} else {
			if w.denied[argument] {
				return nil, terminalcontrol.ErrDenied
			}
			plan, prepareErr := w.prepare(ctx, argument)
			if prepareErr != nil {
				return nil, prepareErr
			}
			route := make([]string, 0, len(plan.Connection.Route))
			for _, hop := range plan.Connection.Route {
				route = append(route, fmt.Sprintf("%s@%s:%d", hop.User, hop.Host, hop.Port))
			}
			requestErr := w.turn.Authorize(ctx, w.Review, assistantcontrol.ReviewInput{Tool: "workspace_open", Location: strings.Join(route, " → "), Action: "使用已保存配置打开 SSH 连接：" + plan.Connection.Name + "；只建立连接和页签，不执行业务命令。"}, assistantcontrol.Prompt{Kind: "approval", Title: "打开这个 SSH 连接？", Description: "使用已保存的连接配置打开独立终端，不继承原终端的目录、环境或切换后的账号。", Details: []assistantcontrol.Detail{{Label: "连接", Value: plan.Connection.Name}, {Label: "访问路径", Value: strings.Join(route, " → ")}}, Choices: []assistantcontrol.Choice{{ID: "reject", Label: "取消"}, {ID: "allow", Label: "打开连接"}}, Expires: time.Now().Add(5 * time.Minute)}, "")
			if requestErr != nil {
				w.denied[argument] = true
				return nil, requestErr
			}
			if ctx.Err() != nil || w.turn.Context().Err() != nil {
				return nil, assistantcontrol.ErrStale
			}
			tab, err = plan.Open(ctx)
		}
		if err != nil {
			return nil, err
		}
		if ctx.Err() != nil || w.turn.Context().Err() != nil {
			return nil, assistantcontrol.ErrStale
		}
		target := terminalcontrol.Target{SessionID: tab.TabID, ConnectionID: tab.ConnectionID}
		if err = w.turn.ChangeTarget(assistantcontrol.Target{TabID: tab.TabID, ConnectionID: tab.ConnectionID}); err != nil {
			return nil, err
		}
		w.current = w.terminal(target)
		denials := w.deniedTargets[target]
		w.current.readDenied = denials.read
		w.current.denied = denials.take
		auditTarget = target
		phase = "presented"
		return map[string]any{"tab": tab, "owned": false, "message": "终端已呈现并设为工具目标，尚未取得读屏或命令授权"}, nil
	default:
		if w.current == nil {
			return unavailableSSH("ssh_required", "请先通过 workspace_list 查看工作台，再用 workspace_select 选择终端或 workspace_open 打开已保存的 SSH 连接"), nil
		}
		return w.current.CallWithPurpose(ctx, name, argument, purpose)
	}
}

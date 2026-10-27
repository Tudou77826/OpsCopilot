// Package assistanttools adapts application capabilities to assistant turns.
// Coordination remains independent of terminal and transport implementations.
package assistanttools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/terminalcontrol"
)

var ErrSSHUnavailable = errors.New("SSH 资源不可用")

// Missing prerequisites are normal tool outcomes, not transport failures.
// Some MCP clients discard error bodies; a structured outcome stays visible to
// the model without granting any input or screen-reading capability.
func unavailableSSH(code, message string) map[string]string {
	return map[string]string{"status": "unavailable", "requires": "ssh", "code": code, "message": message}
}

// TerminalTools binds one host-selected target to one turn. Neither model
// arguments nor a transport header can enlarge its authority.
type TerminalTools struct {
	Turn   *assistantcontrol.Turn
	Target terminalcontrol.Target
	// Validate binds/checks the actual host resource at tool use, not chat start.
	Validate func(context.Context) error
	// Open must verify the actual connection and shell identity and create an
	// audited Access. It runs only after explicit user consent, never from JSON.
	Open func(context.Context) (*terminalcontrol.Access, error)
	// Observe returns an audited, read-only screen snapshot. It must never
	// bootstrap a shell or acquire an input lease.
	Observe     func(context.Context) (terminalcontrol.TerminalContext, error)
	Review      assistantcontrol.Reviewer
	Location    string
	mu          sync.Mutex
	access      *terminalcontrol.Access
	run         terminalcontrol.ControlledTerminal
	denied      bool
	readAllowed bool
	readDenied  bool
}

// Yield gives the terminal back before an ACP question is shown. Concurrent
// tool requests fail immediately instead of queueing a command behind approval.
func (b *TerminalTools) Yield() error {
	if !b.mu.TryLock() {
		return errors.New("终端操作尚未结束")
	}
	defer b.mu.Unlock()
	return b.release()
}
func (b *TerminalTools) release() error {
	if b.run == nil {
		return nil
	}
	err := b.run.Release()
	b.run = nil
	return err
}
func (b *TerminalTools) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.access != nil {
		b.access.Cancel()
	}
	_ = b.release()
}
func (b *TerminalTools) prepare(ctx context.Context) error {
	if b.Turn == nil || b.Turn.Context().Err() != nil {
		return assistantcontrol.ErrStale
	}
	if b.denied || b.readDenied {
		return terminalcontrol.ErrDenied
	}
	if b.access != nil {
		return nil
	}
	err := b.authorize(ctx, "terminal_take", "保护当前终端以执行用户任务；需要普通空闲命令行。", assistantcontrol.Prompt{Kind: "approval", Title: "允许助手接手当前终端？", Description: "请确认终端正停在普通命令行，未运行 vim、less、zsql 等交互程序。允许后，助手可读取当前屏幕和执行常规只读排查；其他命令仍逐次确认。屏幕和命令结果将发送给当前 AI 模型。", Choices: []assistantcontrol.Choice{{ID: "reject", Label: "暂不允许"}, {ID: "allow", Label: "允许本轮"}}, Expires: time.Now().Add(5 * time.Minute)})
	if err != nil {
		b.denied = true
		return errors.Join(terminalcontrol.ErrDenied, err)
	}
	if b.Open == nil {
		return terminalcontrol.ErrUnavailable
	}
	b.access, err = b.Open(b.Turn.Context())
	return err
}
func (b *TerminalTools) observe(ctx context.Context) (terminalcontrol.TerminalContext, error) {
	if b.readDenied {
		return terminalcontrol.TerminalContext{}, terminalcontrol.ErrDenied
	}
	if !b.readAllowed && b.access == nil {
		err := b.authorize(ctx, "terminal_context", "只读取当前终端屏幕，不锁定、不发令。", assistantcontrol.Prompt{Kind: "approval", Title: "允许助手查看当前终端？", Description: "只读取当前屏幕，不锁定终端，也不发送命令。屏幕内容将交给当前 AI 模型。", Choices: []assistantcontrol.Choice{{ID: "reject", Label: "暂不允许"}, {ID: "allow", Label: "允许本轮查看"}}, Expires: time.Now().Add(5 * time.Minute)})
		if err != nil {
			b.readDenied = true
			return terminalcontrol.TerminalContext{}, errors.Join(terminalcontrol.ErrDenied, err)
		}
		b.readAllowed = true
	}
	if b.run != nil {
		view, err := b.run.Inspect(terminalcontrol.ReadOptions{Limit: 16384})
		if err == nil {
			return view, nil
		}
		_ = b.release()
		if !errors.Is(err, terminalcontrol.ErrRevoked) {
			return view, err
		}
		// An expired lease does not turn a passive snapshot into a take request.
	}
	if b.Observe == nil {
		return terminalcontrol.TerminalContext{}, terminalcontrol.ErrUnavailable
	}
	view, err := b.Observe(ctx)
	if errors.Is(err, terminalcontrol.ErrExecutionAudit) {
		b.Turn.Finish("failed", "终端读取审计未能保存，已停止本轮")
	}
	return view, err
}

func (b *TerminalTools) authorize(ctx context.Context, tool, action string, prompt assistantcontrol.Prompt) error {
	location := b.Location
	if location == "" {
		location = fmt.Sprintf("SSH 终端 %s / 连接 %s", b.Target.SessionID, b.Target.ConnectionID)
	}
	// Inspect is passive and audited; it never bootstraps a shell or takes a lease.
	humanReason := ""
	if tool == "terminal_take" && b.Review != nil && b.Observe != nil {
		view, err := b.Observe(ctx)
		if err != nil {
			return err
		}
		action += fmt.Sprintf("\n当前现场 ready=%t；屏幕片段：%s", view.Ready, assistantcontrol.DisplayExcerpt(view.Output.Text, 2048))
		if !view.Ready {
			humanReason = "无法确认终端是否处于空闲命令行，需要人工确认"
		}
	}
	return b.Turn.Authorize(ctx, b.Review, assistantcontrol.ReviewInput{Tool: tool, Location: location, Action: action}, prompt, humanReason)
}
func (b *TerminalTools) Call(ctx context.Context, name string, command string) (any, error) {
	return b.CallWithPurpose(ctx, name, command, "")
}

// Purpose is explanatory model text, never evidence or execution authority.
func (b *TerminalTools) CallWithPurpose(ctx context.Context, name, command, purpose string) (any, error) {
	if !b.mu.TryLock() {
		return nil, errors.New("已有终端操作进行中，请顺序调用")
	}
	defer b.mu.Unlock()
	if b.Turn == nil || b.Turn.Context().Err() != nil || ctx.Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	if name != "terminal_context" && name != "terminal_execute" && name != "terminal_take" && name != "terminal_release" {
		return nil, errors.New("未知终端工具")
	}
	if name == "terminal_release" {
		return map[string]bool{"owned": false}, b.release()
	}
	if b.Target.SessionID == "" || b.Target.ConnectionID == "" {
		return unavailableSSH("ssh_required", "当前没有可用的 SSH 终端。可以继续聊天；需要操作终端时，请先打开并选中一个 SSH 连接，再发送操作要求"), nil
	}
	if b.Validate != nil {
		if err := b.Validate(ctx); err != nil {
			if errors.Is(err, ErrSSHUnavailable) {
				return unavailableSSH("ssh_unavailable", err.Error()), nil
			}
			return nil, err
		}
	}
	if name == "terminal_context" {
		return b.observe(ctx)
	}
	if name == "terminal_execute" && b.run == nil {
		return nil, errors.New("当前未占用终端，请先调用 terminal_take 并重新读取现场；读取屏幕不会自动占用")
	}
	if err := b.prepare(ctx); err != nil {
		return nil, err
	}
	if b.run == nil {
		run, err := b.access.Operations().Take(b.Turn.Context(), terminalcontrol.TakeRequest{Target: b.Target, ConfirmedOrdinaryShell: true, Timeout: terminalcontrol.MaxRunDuration, RetainOnApproval: true})
		if err != nil {
			return nil, err
		}
		b.run = run
	}
	if name == "terminal_take" {
		return b.run.Inspect(terminalcontrol.ReadOptions{Limit: 16384})
	}
	result, err := b.run.Execute(ctx, terminalcontrol.CommandRequest{Command: command})
	var required *terminalcontrol.ApprovalRequired
	if !errors.As(err, &required) {
		b.observeCommand(result, err, command, purpose)
		if err != nil {
			_ = b.release()
		}
		return result, err
	}
	// Review and execution retain the model's exact critical-section lease.
	// Approval grants only this command, not ownership of a new run.
	review, err := b.access.Review(required.ID)
	if err != nil {
		return nil, err
	}
	node := review.Node
	if node == "" {
		node = "当前终端"
	}
	input := assistantcontrol.ReviewInput{Location: fmt.Sprintf("%s · %s · UID %d · %s", b.Location, node, review.Prompt.UID, review.Prompt.Directory), Tool: "remote_shell", Action: review.Command}
	humanReason := ""
	if terminalcontrol.RequiresHumanReview(review.Command) {
		humanReason = "操作涉及删除、服务或权限变化、敏感访问等明确风险，需要人工确认"
	}
	verdict, err := b.Turn.ReviewAction(ctx, b.Review, input, humanReason)
	if err != nil {
		_ = b.release()
		return nil, err
	}
	if auditErr := b.access.RecordPolicyReview(required.ID, verdict.Allowed()); auditErr != nil {
		_ = b.release()
		return nil, auditErr
	}
	if verdict.Allowed() {
		approve := b.access.ApproveReadOnlyReview
		if verdict.LowRisk || review.Assessment.ReadOnly {
			approve = b.access.ApproveLowRiskReview
		}
		op, approveErr := approve(required.ID, verdict.Reason)
		if approveErr != nil {
			_ = b.release()
			return nil, approveErr
		}
		result, execErr := op.Execute(ctx)
		b.observeCommand(result, execErr, command, purpose)
		if execErr != nil {
			_ = b.release()
		}
		return result, execErr
	}
	details := []assistantcontrol.Detail{{Label: "规则检查", Value: review.Assessment.Category}, {Label: "执行影响", Value: review.Assessment.Impact}, {Label: "确认原因", Value: review.Assessment.Reason}, {Label: "执行位置", Value: fmt.Sprintf("%s · UID %d · %s", node, review.Prompt.UID, review.Prompt.Directory)}}
	if len(purpose) > 0 && len(purpose) <= 512 {
		details = append([]assistantcontrol.Detail{{Label: "AI 操作目的（供参考）", Value: purpose}}, details...)
	}
	err = b.Turn.ConfirmAction(ctx, input, assistantcontrol.Prompt{Kind: "approval", Title: "确认这一次终端操作", Description: "助手仍在执行同一段操作，终端保持锁定。批准仅允许下面这一条命令；如需自己操作终端，请手动接管或停止本轮。", Command: review.Command, Expires: review.Expires, Choices: []assistantcontrol.Choice{{ID: "reject", Label: "拒绝"}, {ID: "allow", Label: "允许本次"}}, Details: details}, verdict)
	if err != nil {
		if errors.Is(err, assistantcontrol.ErrActionDenied) {
			if _, auditErr := b.access.Approve(required.ID, false); errors.Is(auditErr, terminalcontrol.ErrExecutionAudit) {
				b.Turn.Finish("failed", "终端拒绝记录未能保存，已停止本轮")
			}
		}
		_ = b.release()
		return nil, err
	}
	op, err := b.access.Approve(required.ID, true)
	if err != nil {
		_ = b.release()
		return nil, err
	}
	result, err = op.Execute(ctx)
	b.observeCommand(result, err, command, purpose)
	if err != nil {
		_ = b.release()
		return map[string]any{"execution": result, "owned": false}, err
	}
	view, inspectErr := b.runContext()
	if inspectErr != nil {
		_ = b.release()
	}
	return map[string]any{"execution": result, "owned": inspectErr == nil && view.Owned}, inspectErr
}

func (b *TerminalTools) observeCommand(result terminalcontrol.CommandResult, err error, command, purpose string) {
	if result.OperationID == "" {
		return
	}
	op := assistantcontrol.Operation{Kind: "terminal", WorkspaceID: b.Target.SessionID, ConnectionID: b.Target.ConnectionID, Status: "unknown", Stage: "远程命令", Outcome: "尚未确认命令结果。", Next: "先查看终端现场，不要直接重复执行。"}
	op.Action, op.Result, op.Location = command, result.Output.Text, "远程终端"
	if result.Output.Truncated {
		op.Result += "\n[终端片段，完整现场请查看原终端]"
	}
	title := "命令结果未确认"
	if result.Completion == terminalcontrol.Completed && result.ExitCode != nil {
		op.Status, op.Outcome, op.Next = "completed", fmt.Sprintf("已收到退出码 %d；输出保留在终端。", *result.ExitCode), ""
		title = "终端命令已结束"
		if *result.ExitCode != 0 {
			title, op.Status, op.Next = "终端命令返回异常", "failed", "查看终端输出，核对失败原因。"
		}
	} else if result.Submission == terminalcontrol.NotSubmitted {
		title, op.Status, op.Outcome = "命令未提交", "not_executed", "命令未提交到终端。"
		op.Next = "核对连接及批准状态后再决定下一步。"
	}
	if err != nil {
		op.Detail = err.Error()
		if op.Status == "completed" {
			op.Status, title, op.Next = "warning", "命令已结束，结果记录异常", "先核对终端输出，不要直接重复执行。"
		}
	}
	if purpose != "" {
		title = assistantcontrol.DisplayExcerpt(purpose, 512)
	} else if op.Action != "" {
		title = "执行命令"
	}
	b.Turn.ObserveOperation(result.OperationID, title, op)
}
func (b *TerminalTools) runContext() (terminalcontrol.TerminalContext, error) {
	if b.run == nil {
		return terminalcontrol.TerminalContext{}, terminalcontrol.ErrRevoked
	}
	return b.run.Inspect(terminalcontrol.ReadOptions{Limit: 1})
}

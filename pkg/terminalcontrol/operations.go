package terminalcontrol

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Operations is the process-local application boundary for existing SSH PTYs.
// It is not an authenticated RPC/tool endpoint. AI/plugin adapters must receive
// Access.Operations(), not a raw Service, to enforce scope, approval and audit.
type Operations interface {
	Inspect(Target, ReadOptions) (TerminalContext, error)
	Take(context.Context, TakeRequest) (ControlledTerminal, error)
}

type ControlledTerminal interface {
	ID() string
	Target() Target
	Inspect(ReadOptions) (TerminalContext, error)
	Execute(context.Context, CommandRequest) (CommandResult, error)
	Release() error
}

type Target struct {
	SessionID    string `json:"sessionId"`
	ConnectionID string `json:"connectionId"`
}

type ReadOptions struct {
	After uint64
	Limit int
}

type ShellContext struct {
	UID          int    `json:"uid"`
	PID          int    `json:"pid"`
	Directory    string `json:"directory"`
	LastExitCode int    `json:"lastExitCode"`
	Sequence     uint64 `json:"sequence"`
}

type TerminalContext struct {
	Target Target `json:"target"`
	Ready  bool   `json:"ready"`
	Owned  bool   `json:"owned"`
	Closed bool   `json:"closed"`
	// LastPrompt is historical while Ready is false, never a claim that the
	// current foreground program or execution identity has been recognized.
	LastPrompt *ShellContext `json:"lastPrompt,omitempty"`
	Output     Observation   `json:"output"`
}

type TakeRequest struct {
	Target                 Target
	ConfirmedOrdinaryShell bool
	// Zero uses 30 seconds. No lease survives longer than two minutes here;
	// future agent liveness/renewal belongs to the application run coordinator.
	Timeout time.Duration
	// A trusted adapter can retain the model's explicit ownership across review.
	// Default false preserves the standalone one-shot approval workflow.
	RetainOnApproval bool
	afterRunID       string // host-only approval continuation, checked atomically
	access           *Access
	approval         *approvedCommand
}

type CommandRequest struct {
	Command     string
	Timeout     time.Duration
	OutputLimit int
}

type Completion string

const (
	NotStarted    Completion = "not_started"
	ResultUnknown Completion = "unknown"
	Completed     Completion = "completed"
)

type CommandResult struct {
	OperationID string           `json:"operationId"`
	RunID       string           `json:"runId"`
	Target      Target           `json:"target"`
	Submission  Submission       `json:"submission"`
	Completion  Completion       `json:"completion"`
	ExitCode    *int             `json:"exitCode,omitempty"`
	Output      Observation      `json:"output"`
	Context     *TerminalContext `json:"context,omitempty"`
}

var ErrUnavailable = errors.New("终端连接已关闭或已被替换")

const MaxRunDuration = 2 * time.Minute

// Run is an opaque in-process handle. Its identity/lease cannot be supplied or
// reconstructed by a model from a session ID. No Wails method accepts this type.
type Run struct {
	service          *Service
	entry            *entry
	target           Target
	id               string
	ctx              context.Context
	cancel           context.CancelFunc
	done             chan struct{}
	end              sync.Once
	command          sync.Mutex
	prepared         bool  // guarded by command
	lease            Lease // guarded by command
	access           *Access
	approval         *approvedCommand
	retainOnApproval bool
	pendingReview    *commandReview // guarded by command
}

// AfterFunc wakes waiters, while Err checks both sources synchronously at the
// actual dispatch boundary; callback scheduling cannot hide caller cancellation.
type commandContext struct {
	context.Context
	caller context.Context
}

func (c commandContext) Err() error {
	if err := c.caller.Err(); err != nil {
		return err
	}
	return c.Context.Err()
}

var _ Operations = (*Service)(nil)
var _ ControlledTerminal = (*Run)(nil)

func (r *Run) ID() string     { return r.id }
func (r *Run) Target() Target { return r.target }

func contextFrom(target Target, state Snapshot, output Observation) TerminalContext {
	result := TerminalContext{Target: target, Ready: state.Ready && !state.Closed, Owned: state.Owned, Closed: state.Closed, Output: output}
	if state.Sequence != 0 {
		result.LastPrompt = &ShellContext{UID: state.UID, PID: state.PID, Directory: state.Directory, LastExitCode: state.ExitCode, Sequence: state.Sequence}
	}
	return result
}

func (s *Service) inspectLocked(target Target, options ReadOptions) (TerminalContext, error) {
	e := s.entries[target.SessionID]
	if target.ConnectionID == "" || e == nil || e.state.ConnectionID != target.ConnectionID {
		return TerminalContext{}, ErrUnavailable
	}
	state, output, err := e.controller.Observe(options.After, options.Limit)
	if err != nil {
		return TerminalContext{}, err
	}
	view := contextFrom(target, state, output)
	view.Owned = e.state.Owned && !state.Closed
	return view, nil
}

func (s *Service) Inspect(target Target, options ReadOptions) (TerminalContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inspectLocked(target, options)
}

func (r *Run) currentLocked() bool {
	return r.service.entries[r.target.SessionID] == r.entry && r.entry.state.RunID == r.id && r.entry.state.Owned && r.ctx.Err() == nil && (r.access == nil || r.access.ctx.Err() == nil)
}

func (r *Run) Inspect(options ReadOptions) (TerminalContext, error) {
	r.service.mu.Lock()
	if !r.currentLocked() {
		r.service.mu.Unlock()
		return TerminalContext{}, ErrRevoked
	}
	view, err := r.service.inspectLocked(r.target, options)
	r.service.mu.Unlock()
	if err != nil {
		return TerminalContext{}, err
	}
	if r.access != nil {
		if err := r.access.recordRead(r.target, r.id, view); err != nil {
			return TerminalContext{}, err
		}
	}
	return view, err
}

func (s *Service) beginRun(ctx context.Context, request TakeRequest) (*Run, State, error) {
	if !request.ConfirmedOrdinaryShell || request.Target.ConnectionID == "" {
		return nil, State{}, ErrNeedConfirmation
	}
	if request.Timeout < 0 || request.Timeout > MaxRunDuration {
		return nil, State{}, errors.New("接手期限必须在 0–2 分钟之间")
	}
	if request.Timeout == 0 {
		request.Timeout = 30 * time.Second
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, State{}, err
	}
	e := s.entries[request.Target.SessionID]
	if e == nil {
		return nil, State{}, ErrUnavailable
	}
	if e.state.ConnectionID != request.Target.ConnectionID {
		return nil, State{}, ErrNeedConfirmation
	}
	if request.afterRunID != "" && (e.state.RunID != request.afterRunID || e.state.Phase != "awaiting_approval") {
		return nil, State{}, ErrStaleApproval
	}
	if request.approval != nil && !samePrompt(request.approval.review.snapshot, e.controller.Snapshot()) {
		return nil, State{}, ErrStaleApproval
	}
	if s.active != "" {
		return nil, State{}, errors.New("另一个终端正在接手，请先停止或等待完成")
	}
	if err := e.controller.Reserve(); err != nil {
		return nil, State{}, err
	}
	runCtx, cancel := context.WithTimeout(ctx, request.Timeout)
	r := &Run{service: s, entry: e, target: request.Target, id: uuid.NewString(), ctx: runCtx, cancel: cancel, done: make(chan struct{}), access: request.access, approval: request.approval, retainOnApproval: request.RetainOnApproval}
	e.run, e.cancel = r, cancel
	e.state = State{SessionID: r.target.SessionID, ConnectionID: r.target.ConnectionID, RunID: r.id, Phase: "preparing", Owned: true, Message: "正在接手当前命令行…"}
	s.active = r.id
	// Started before any logging/preparation. Cancel always revokes the gate,
	// including when a callback or the transport writer is blocked.
	go func() {
		select {
		case <-runCtx.Done():
			_ = r.finish("stopped", "已停止发令并交还终端", nil)
		case <-r.done:
		}
	}()
	return r, cloneState(e.state), nil
}

func (s *Service) Take(ctx context.Context, request TakeRequest) (ControlledTerminal, error) {
	return s.take(ctx, request, nil, nil)
}

func (s *Service) take(ctx context.Context, request TakeRequest, access *Access, approval *approvedCommand) (ControlledTerminal, error) {
	request.access, request.approval = access, approval
	r, _, err := s.beginRun(ctx, request)
	if err != nil {
		return nil, err
	}
	s.audit(Audit{SessionID: r.target.SessionID, RunID: r.id, Action: "take", Phase: "accepted"})
	r.command.Lock()
	err = r.prepareLocked()
	r.command.Unlock()
	if err != nil {
		r.fail(err)
		return nil, err
	}
	if access != nil {
		if err := access.checkTarget(r.target, r.entry.controller.Snapshot()); err != nil {
			r.fail(err)
			return nil, err
		}
	}
	return r, nil
}

func (r *Run) prepareLocked() error {
	if r.prepared {
		return nil
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	c := r.entry.controller
	if r.approval != nil && !samePrompt(r.approval.review.snapshot, c.Snapshot()) {
		return ErrStaleApproval
	}
	if !c.Snapshot().Ready {
		// An AI scope is not a fresh human assertion that the foreground is an
		// idle shell. After yielding, never inject bootstrap probes into vim,
		// zsql, a running command, or unsubmitted user input.
		if r.access != nil {
			return ErrNeedConfirmation
		}
		ctx, cancel := context.WithTimeout(r.ctx, 4*time.Second)
		_, err := c.Bootstrap(ctx, true)
		cancel()
		if err != nil {
			return err
		}
	}
	r.service.mu.Lock()
	defer r.service.mu.Unlock()
	if !r.currentLocked() {
		return ErrRevoked
	}
	lease, err := c.Acquire()
	if err != nil {
		return err
	}
	r.lease, r.prepared = lease, true
	r.entry.state.Phase, r.entry.state.Message = "ready", "已接手当前终端"
	return nil
}

func (r *Run) Execute(ctx context.Context, request CommandRequest) (CommandResult, error) {
	return r.execute(ctx, request, nil)
}

func (r *Run) execute(ctx context.Context, request CommandRequest, continuation *approvedCommand) (CommandResult, error) {
	result := CommandResult{OperationID: uuid.NewString(), RunID: r.id, Target: r.target, Submission: NotSubmitted, Completion: NotStarted}
	if err := validateCommand(request.Command); err != nil {
		return result, err
	}
	if request.Timeout < 0 || request.Timeout > MaxRunDuration {
		return result, errors.New("命令期限必须在 0–2 分钟之间")
	}
	if err := validateRead(0, request.OutputLimit, 0); err != nil {
		return result, err
	}
	if !r.command.TryLock() {
		return result, ErrBusy
	}
	defer r.command.Unlock()
	if r.ctx.Err() != nil {
		return result, ErrRevoked
	}
	if r.pendingReview != nil {
		if continuation == nil || continuation.review != r.pendingReview {
			return result, ErrBusy
		}
		r.pendingReview = nil
		r.approval = continuation
		defer func() { r.approval = nil }()
	} else if continuation != nil {
		return result, ErrStaleApproval
	}
	if err := ctx.Err(); err != nil {
		_ = r.service.Stop(r.target.SessionID, r.id)
		return result, err
	}
	if err := r.prepareLocked(); err != nil {
		r.fail(err)
		return result, err
	}
	r.service.mu.Lock()
	if !r.currentLocked() {
		r.service.mu.Unlock()
		return result, ErrRevoked
	}
	r.entry.state.Phase, r.entry.state.Message, r.entry.state.ExitCode = "running", "正在当前 Shell 中执行命令", nil
	r.service.mu.Unlock()
	baseCtx, cancel := context.WithCancel(r.ctx)
	if request.Timeout != 0 {
		var timeoutCancel context.CancelFunc
		baseCtx, timeoutCancel = context.WithTimeout(baseCtx, request.Timeout)
		defer timeoutCancel()
	}
	stopCaller := context.AfterFunc(ctx, cancel)
	opCtx := commandContext{Context: baseCtx, caller: ctx}
	stopWatcher := context.AfterFunc(opCtx, func() { _ = r.service.Stop(r.target.SessionID, r.id) })
	defer func() { stopWatcher(); stopCaller(); cancel() }()
	r.service.audit(Audit{SessionID: r.target.SessionID, RunID: r.id, Action: "command", Phase: "dispatch_requested"})
	var check func(Snapshot) error
	auditSnapshot := r.entry.controller.Snapshot()
	if r.access != nil {
		var err error
		check, err = r.access.authorize(opCtx, ctx, r, result.OperationID, request)
		if err != nil {
			var review *ApprovalRequired
			if errors.As(err, &review) {
				if r.retainOnApproval {
					r.service.mu.Lock()
					if r.currentLocked() {
						r.entry.state.Phase, r.entry.state.Message = "awaiting_approval", "等待命令确认，仍由助手占用"
					}
					r.service.mu.Unlock()
					return result, err
				}
				// Releasing for review is not an explicit user stop. Detach the
				// command watcher before finish cancels this short-lived run.
				stopWatcher()
				_ = r.finish("awaiting_approval", "等待明确批准，终端已交还", nil)
			} else {
				r.fail(err)
			}
			return result, err
		}
	}
	execution, err := r.entry.controller.executeChecked(opCtx, r.lease, request.Command, request.OutputLimit, check)
	result.Submission = execution.Submission
	if execution.Submission != NotSubmitted {
		view := contextFrom(r.target, execution.State, execution.Output)
		result.Context, result.Output = &view, execution.Output
		result.Completion = ResultUnknown
	}
	if execution.Completed && err == nil {
		result.Completion = Completed
		exit := execution.State.ExitCode
		result.ExitCode = &exit
	}
	if r.access != nil {
		if auditErr := r.access.recordResult(r, request, auditSnapshot, result, err); auditErr != nil {
			r.fail(auditErr)
			return result, errors.Join(err, auditErr)
		}
	}
	if err != nil {
		if opCtx.Err() != nil {
			_ = r.service.Stop(r.target.SessionID, r.id)
			err = opCtx.Err()
		} else {
			r.fail(err)
		}
		return result, err
	}
	r.service.mu.Lock()
	if r.currentLocked() {
		r.entry.state.Phase, r.entry.state.Message = "ready", "命令已返回，继续接手中"
		r.entry.state.ExitCode = cloneExit(result.ExitCode)
	}
	r.service.mu.Unlock()
	return result, nil
}

func cloneExit(exit *int) *int {
	if exit == nil {
		return nil
	}
	copy := *exit
	return &copy
}
func cloneState(state State) State { state.ExitCode = cloneExit(state.ExitCode); return state }

func (r *Run) fail(err error) {
	if r.ctx.Err() != nil {
		_ = r.service.Stop(r.target.SessionID, r.id)
		return
	}
	_ = r.finish("failed", fmt.Sprintf("接手未完成：%v。终端已交还；请核对当前命令行状态。", err), nil)
}

func (r *Run) finish(phase, message string, exit *int) error {
	s := r.service
	s.mu.Lock()
	current := s.entries[r.target.SessionID] == r.entry && r.entry.state.RunID == r.id
	changed := current && r.entry.state.Owned
	if changed {
		r.entry.controller.Revoke()
		r.entry.state.Owned, r.entry.state.Phase, r.entry.state.Message = false, phase, message
		r.entry.state.ExitCode = cloneExit(exit)
		if s.active == r.id {
			s.active = ""
		}
	}
	s.mu.Unlock()
	r.cancel()
	r.end.Do(func() { close(r.done) })
	var auditErr error
	if changed {
		if r.access != nil {
			auditErr = r.access.recordRelease(r.id, phase)
		}
		s.audit(Audit{SessionID: r.target.SessionID, RunID: r.id, Action: "release", Phase: phase})
	}
	if !current {
		return ErrRevoked
	}
	return auditErr
}

func (r *Run) Release() error {
	return r.finish("released", "已交还终端；请以当前提示符确认远端状态", nil)
}

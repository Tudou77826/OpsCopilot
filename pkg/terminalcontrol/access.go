package terminalcontrol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

var ErrDenied = errors.New("操作不在本轮授权范围内")
var ErrStaleApproval = errors.New("审批已失效，需要基于最新终端现场重新确认")
var ErrExecutionAudit = errors.New("执行审计未能完整保存，已停止后续发令；不要自动重试")

// AccessTarget is constructed by the trusted host, never deserialized from tool
// arguments. UID is the actual shell identity, including a prior su/sudo switch.
// ReadFiles are individually authorized contents, not a filesystem sandbox.
type AccessTarget struct {
	Target          Target
	Node            string
	SSHUser         string
	UID             int
	ReadFiles       []string
	ListDirectories []string
	// Granted by the host after diagnostic consent, never by a model argument.
	RoutineDiagnostics bool
}

type AccessScope struct {
	Actor   string
	Expires time.Time
	Targets []AccessTarget
	// Desktop task review checks semantic scope even for syntactically read-only commands.
	RequireActionReview bool
}

// Access owns one host-issued scope. Give Operations() to an adapter, retaining
// Approve/Cancel on the host side. It is an in-process boundary, not IPC auth.
type Access struct {
	ctx     context.Context
	cancel  context.CancelFunc
	service *Service
	journal ExecutionJournal
	scope   AccessScope
	mu      sync.Mutex
	reviews map[string]*commandReview
	active  map[string]ExecutionAudit
}

type accessOperations struct{ access *Access }

type ApprovalRequired struct{ ID string }

func (e *ApprovalRequired) Error() string {
	return "该命令需要用户明确批准，尚未发送"
}

// ReviewDetails contains the exact command for a trusted confirmation UI only.
// Do not put it in logs or return it as an agent's approval credential.
type ReviewDetails struct {
	ID          string
	OperationID string
	RunID       string
	Target      Target
	Actor       string
	Node        string
	Command     string
	Prompt      ShellContext
	Expires     time.Time
	Assessment  CommandAssessment
}

type commandReview struct {
	details         ReviewDetails
	request         CommandRequest
	snapshot        Snapshot
	caller          context.Context
	invalid         bool
	resolved        bool
	selfReviewed    bool
	lowRiskReviewed bool
	heldRun         *Run
}

// ApprovedOperation is opaque and single-use; its execution accepts no target,
// command, UID, timeout or output-limit overrides.
type ApprovedOperation interface {
	Execute(context.Context) (CommandResult, error)
}

type approvedCommand struct {
	access *Access
	review *commandReview
	used   atomic.Bool
}

func NewAccess(ctx context.Context, service *Service, scope AccessScope, journal ExecutionJournal) (*Access, error) {
	if service == nil || journal == nil || scope.Actor == "" || len(scope.Actor) > 128 || len(scope.Targets) == 0 || len(scope.Targets) > 128 || !scope.Expires.After(time.Now()) || scope.Expires.After(time.Now().Add(24*time.Hour)) {
		return nil, errors.New("需要有效的调用身份、明确目标、期限和执行审计")
	}
	scope.Targets = append([]AccessTarget(nil), scope.Targets...)
	seen := make(map[Target]bool)
	for i := range scope.Targets {
		t := &scope.Targets[i]
		if t.Target.SessionID == "" || t.Target.ConnectionID == "" || t.UID < 0 || seen[t.Target] {
			return nil, ErrDenied
		}
		seen[t.Target] = true
		t.ReadFiles = append([]string(nil), t.ReadFiles...)
		t.ListDirectories = append([]string(nil), t.ListDirectories...)
	}
	ctx, cancel := context.WithDeadline(ctx, scope.Expires)
	return &Access{ctx: ctx, cancel: cancel, service: service, scope: scope, journal: journal, reviews: make(map[string]*commandReview), active: make(map[string]ExecutionAudit)}, nil
}

func (a *Access) Operations() Operations { return accessOperations{a} }
func (a *Access) Cancel()                { a.cancel() }
func (a *Access) Done() <-chan struct{}  { return a.ctx.Done() }

func (a *Access) target(target Target) (AccessTarget, error) {
	if a.ctx.Err() != nil || !a.scope.Expires.After(time.Now()) {
		return AccessTarget{}, ErrDenied
	}
	for _, allowed := range a.scope.Targets {
		if allowed.Target == target {
			return allowed, nil
		}
	}
	return AccessTarget{}, ErrDenied
}

func (a *Access) checkTarget(target Target, snapshot Snapshot) error {
	allowed, err := a.target(target)
	if err != nil || snapshot.Closed || snapshot.Sequence == 0 || snapshot.UID != allowed.UID {
		return ErrDenied
	}
	return nil
}

func (o accessOperations) Inspect(target Target, options ReadOptions) (TerminalContext, error) {
	a := o.access
	if _, err := a.target(target); err != nil {
		return TerminalContext{}, err
	}
	view, err := a.service.Inspect(target, options)
	if err != nil {
		return TerminalContext{}, err
	}
	if err := a.recordRead(target, "", view); err != nil {
		return TerminalContext{}, err
	}
	return view, nil
}

func (a *Access) recordRead(target Target, runID string, view TerminalContext) error {
	allowed, err := a.target(target)
	if err != nil || view.LastPrompt == nil || view.LastPrompt.UID != allowed.UID {
		return ErrDenied
	}
	p := view.LastPrompt
	if err := a.append(ExecutionAudit{Time: time.Now().UTC(), Actor: a.scope.Actor, Target: target, Node: allowed.Node, SSHUser: allowed.SSHUser, RunID: runID, OperationID: uuid.NewString(), UID: p.UID, PID: p.PID, Sequence: p.Sequence, DirectoryHash: digest(p.Directory), Phase: "context_read"}); err != nil {
		return err
	}
	if a.ctx.Err() != nil {
		return ErrDenied
	}
	return nil
}

func (o accessOperations) Take(ctx context.Context, request TakeRequest) (ControlledTerminal, error) {
	if _, err := o.access.target(request.Target); err != nil {
		return nil, err
	}
	// Scope cancellation also revokes an idle run. A run never extends its scope.
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(o.access.ctx, cancel)
	run, err := o.access.service.take(ctx, request, o.access, nil)
	if err != nil {
		stop()
		cancel()
		return nil, err
	}
	r := run.(*Run)
	go func() { <-r.done; stop(); cancel() }()
	return run, nil
}

func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func samePrompt(left, right Snapshot) bool {
	return !right.Closed && right.Ready && left.Sequence == right.Sequence && left.Prompt == right.Prompt
}

func (a *Access) event(run *Run, id string, request CommandRequest, snapshot Snapshot, phase, reason string) ExecutionAudit {
	event := ExecutionAudit{Time: time.Now().UTC(), Actor: a.scope.Actor, Target: run.target, RunID: run.id, OperationID: id, CommandHash: digest(request.Command), CommandBytes: len(request.Command), UID: snapshot.UID, PID: snapshot.PID, Sequence: snapshot.Sequence, DirectoryHash: digest(snapshot.Directory), Phase: phase, Reason: reason}
	if run.approval != nil {
		event.ApprovalID = run.approval.review.details.ID
	}
	for _, target := range a.scope.Targets {
		if target.Target == run.target {
			event.Node, event.SSHUser = target.Node, target.SSHUser
			break
		}
	}
	return event
}

func (a *Access) append(event ExecutionAudit) error {
	// Result evidence must still persist after the caller has stopped waiting.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.journal.Append(ctx, event); err != nil {
		a.Cancel()
		return fmt.Errorf("%w: %w", ErrExecutionAudit, err)
	}
	return nil
}

func (a *Access) authorize(ctx, caller context.Context, run *Run, id string, request CommandRequest) (func(Snapshot) error, error) {
	snapshot := run.entry.controller.Snapshot()
	a.mu.Lock()
	a.active[run.id] = a.event(run, id, request, snapshot, "requested", "")
	a.mu.Unlock()
	if err := a.checkTarget(run.target, snapshot); err != nil {
		return nil, errors.Join(err, a.append(a.event(run, id, request, snapshot, "denied", "scope_or_identity")))
	}
	allowed, _ := a.target(run.target)
	reason := "literal_read_only"
	if run.approval != nil {
		if request != run.approval.review.request || !samePrompt(run.approval.review.snapshot, snapshot) || !a.reviewValid(run.approval.review) {
			return nil, ErrStaleApproval
		}
		reason = "explicit_approval"
		if run.approval.review.selfReviewed {
			reason = "host_policy_review"
			if run.approval.review.lowRiskReviewed {
				reason = "host_policy_review"
			}
		}
	} else if assessment := assessCommand(request.Command, allowed, snapshot.Directory); !assessment.ReadOnly || a.scope.RequireActionReview {
		expires := time.Now().Add(time.Minute)
		if a.scope.Expires.Before(expires) {
			expires = a.scope.Expires
		}
		review := &commandReview{details: ReviewDetails{ID: uuid.NewString(), OperationID: id, RunID: run.id, Target: run.target, Actor: a.scope.Actor, Command: request.Command, Prompt: ShellContext{UID: snapshot.UID, PID: snapshot.PID, Directory: snapshot.Directory, LastExitCode: snapshot.ExitCode, Sequence: snapshot.Sequence}, Expires: expires, Assessment: assessment}, request: request, snapshot: snapshot, caller: caller}
		review.details.Node = allowed.Node
		if run.retainOnApproval {
			review.heldRun = run
		}
		event := a.event(run, id, request, snapshot, "approval_required", "not_proven_read_only")
		event.ApprovalID = review.details.ID
		if err := a.append(event); err != nil {
			return nil, err
		}
		a.mu.Lock()
		for key, prior := range a.reviews {
			if prior.invalid || !prior.details.Expires.After(time.Now()) {
				delete(a.reviews, key)
			}
		}
		if len(a.reviews) >= 128 {
			a.mu.Unlock()
			return nil, ErrDenied
		}
		a.reviews[review.details.ID] = review
		a.mu.Unlock()
		if run.retainOnApproval {
			run.pendingReview = review
		}
		if ctx.Err() != nil {
			a.cancelReviews(run.id)
			return nil, ctx.Err()
		}
		return nil, &ApprovalRequired{ID: review.details.ID}
	}
	if err := a.append(a.event(run, id, request, snapshot, "dispatch_intent", reason)); err != nil {
		return nil, err
	}
	return func(current Snapshot) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := a.checkTarget(run.target, current); err != nil {
			return err
		}
		if !samePrompt(snapshot, current) {
			return ErrStaleApproval
		}
		if run.approval != nil && !a.reviewValid(run.approval.review) {
			return ErrStaleApproval
		}
		return nil
	}, nil
}

func (a *Access) recordResult(run *Run, request CommandRequest, snapshot Snapshot, result CommandResult, err error) error {
	defer func() { a.mu.Lock(); delete(a.active, run.id); a.mu.Unlock() }()
	if result.Context != nil && result.Context.LastPrompt != nil {
		p := result.Context.LastPrompt
		snapshot.UID, snapshot.PID, snapshot.Directory, snapshot.Sequence = p.UID, p.PID, p.Directory, p.Sequence
	}
	reason := "returned"
	if err != nil {
		reason = "transport_or_state"
	}
	if errors.Is(err, context.Canceled) {
		reason = "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		reason = "timeout"
	}
	event := a.event(run, result.OperationID, request, snapshot, "result", reason)
	event.Submission, event.Completion, event.ExitCode = result.Submission, result.Completion, cloneExit(result.ExitCode)
	return a.append(event)
}

func (a *Access) recordRelease(runID, reason string) error {
	a.mu.Lock()
	event, found := a.active[runID]
	delete(a.active, runID)
	a.mu.Unlock()
	if !found || reason == "awaiting_approval" {
		return nil
	}
	event.Time, event.Phase, event.Reason = time.Now().UTC(), "authority_revoked", reason
	return a.append(event)
}

func (a *Access) cancelReviews(runID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, review := range a.reviews {
		if review.details.RunID == runID {
			review.invalid = true
		}
	}
}

func (a *Access) reviewValid(review *commandReview) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.ctx.Err() == nil && !review.invalid && review.caller.Err() == nil && review.details.Expires.After(time.Now())
}

func (a *Access) Review(id string) (ReviewDetails, error) {
	a.mu.Lock()
	review := a.reviews[id]
	a.mu.Unlock()
	if review == nil || !a.reviewValid(review) {
		return ReviewDetails{}, ErrStaleApproval
	}
	return review.details, nil
}

func (a *Access) RecordPolicyReview(id string, allowed bool) error {
	review, err := a.Review(id)
	if err != nil {
		return err
	}
	status := "human_required"
	if allowed {
		status = "passed_floor"
	}
	return a.append(ExecutionAudit{Time: time.Now().UTC(), Actor: a.scope.Actor, Target: review.Target, RunID: review.RunID, OperationID: review.OperationID, ApprovalID: id, CommandHash: digest(review.Command), CommandBytes: len(review.Command), Phase: "host_policy_checked", Reason: status})
}

// Approve belongs only to the trusted user approval path; ACP tool-success or
// a model's approved=true is not a substitute. Calling it never executes SQL/SSH.
func (a *Access) Approve(id string, allow bool) (ApprovedOperation, error) {
	return a.resolveReview(id, allow, "", false)
}

// ApproveReadOnlyReview retains scope, identity, expiry and exact-command binding.
func (a *Access) ApproveReadOnlyReview(id, reason string) (ApprovedOperation, error) {
	if strings.TrimSpace(reason) == "" || len(reason) > 1024 {
		return nil, ErrDenied
	}
	return a.resolveReview(id, true, reason, false)
}

// ApproveLowRiskReview is host-only; target, UID, shell sequence, lease and
// single-use dispatch are identical to exact human approval. It never broadens scope.
func (a *Access) ApproveLowRiskReview(id, reason string) (ApprovedOperation, error) {
	if strings.TrimSpace(reason) == "" || len(reason) > 1024 {
		return nil, ErrDenied
	}
	return a.resolveReview(id, true, reason, true)
}

var legacyWriteWords = regexp.MustCompile(`(?i)(^|[^a-z0-9_])(mv|cp|tee|sed|touch|mkdir|set-content|add-content|out-file)([^a-z0-9_]|$)`)

func (a *Access) resolveReview(id string, allow bool, aiReason string, lowRisk bool) (ApprovedOperation, error) {
	a.mu.Lock()
	review := a.reviews[id]
	if review == nil || review.resolved || review.invalid || a.ctx.Err() != nil || review.caller.Err() != nil || !review.details.Expires.After(time.Now()) {
		a.mu.Unlock()
		return nil, ErrStaleApproval
	}
	if aiReason != "" && ((!lowRisk && (review.details.Assessment.Category != "无法自动判定" || legacyWriteWords.MatchString(review.request.Command))) || RequiresHumanReview(review.request.Command)) {
		a.mu.Unlock()
		return nil, ErrDenied
	}
	review.selfReviewed = aiReason != ""
	review.lowRiskReviewed = lowRisk
	review.resolved = true
	if !allow {
		review.invalid = true
	}
	a.mu.Unlock()
	snapshot, err := a.service.Inspect(review.details.Target, ReadOptions{Limit: 1})
	if err != nil || snapshot.LastPrompt == nil || !snapshot.Ready || *snapshot.LastPrompt != review.details.Prompt {
		return nil, ErrStaleApproval
	}
	if review.heldRun != nil {
		a.service.mu.Lock()
		valid := review.heldRun.currentLocked()
		a.service.mu.Unlock()
		if !snapshot.Owned || !valid {
			return nil, ErrStaleApproval
		}
	} else if snapshot.Owned {
		return nil, ErrStaleApproval
	}
	event := ExecutionAudit{Time: time.Now().UTC(), Actor: a.scope.Actor, Target: review.details.Target, RunID: review.details.RunID, OperationID: review.details.OperationID, ApprovalID: id, CommandHash: digest(review.request.Command), CommandBytes: len(review.request.Command), UID: review.snapshot.UID, PID: review.snapshot.PID, Sequence: review.snapshot.Sequence, DirectoryHash: digest(review.snapshot.Directory), Phase: "approval_denied"}
	if allow {
		event.Phase = "approval_granted"
		if review.selfReviewed {
			event.Phase = "ai_review_granted"
			event.Reason = "host_policy_review"
			if review.lowRiskReviewed {
				event.Reason = "host_policy_review"
			}
		}
	}
	if err := a.append(event); err != nil {
		return nil, err
	}
	if !allow {
		return nil, ErrDenied
	}
	return &approvedCommand{access: a, review: review}, nil
}

func (p *approvedCommand) Execute(ctx context.Context) (CommandResult, error) {
	notStarted := CommandResult{Submission: NotSubmitted, Completion: NotStarted}
	if !p.used.CompareAndSwap(false, true) || !p.access.reviewValid(p.review) {
		return notStarted, ErrStaleApproval
	}
	ctx, cancel := context.WithCancel(ctx)
	stopAccess := context.AfterFunc(p.access.ctx, cancel)
	stopCaller := context.AfterFunc(p.review.caller, cancel)
	defer func() { stopAccess(); stopCaller(); cancel() }()
	if p.review.caller.Err() != nil || ctx.Err() != nil {
		return notStarted, ErrStaleApproval
	}
	if p.review.heldRun != nil {
		// Reuse the exact existing lease; neither approval nor execution releases
		// the model's critical section. Stop/expiry still revoke it at dispatch.
		return p.review.heldRun.execute(ctx, p.review.request, p)
	}
	run, err := p.access.service.take(ctx, TakeRequest{Target: p.review.details.Target, ConfirmedOrdinaryShell: true, afterRunID: p.review.details.RunID}, p.access, p)
	if err != nil {
		return notStarted, err
	}
	defer run.Release()
	return run.Execute(ctx, p.review.request)
}

package terminalcontrol

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ObservationScope grants a screen snapshot only, without a shell identity or
// permission to bootstrap/acquire/write. The trusted host binds the connection.
type ObservationScope struct {
	Actor, Node, SSHUser string
	Target               Target
	Expires              time.Time
}
type Observer struct {
	ctx     context.Context
	service *Service
	scope   ObservationScope
	journal ExecutionJournal
}

func NewObserver(ctx context.Context, service *Service, scope ObservationScope, journal ExecutionJournal) (*Observer, error) {
	if service == nil || journal == nil || scope.Actor == "" || len(scope.Actor) > 128 || scope.Target.SessionID == "" || scope.Target.ConnectionID == "" || !scope.Expires.After(time.Now()) || scope.Expires.After(time.Now().Add(24*time.Hour)) {
		return nil, errors.New("屏幕读取需要有效的身份、连接、期限和审计")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Observer{ctx: ctx, service: service, scope: scope, journal: journal}, nil
}
func (o *Observer) Inspect(options ReadOptions) (TerminalContext, error) {
	if o.ctx.Err() != nil || !o.scope.Expires.After(time.Now()) {
		return TerminalContext{}, ErrDenied
	}
	view, err := o.service.Inspect(o.scope.Target, options)
	if err != nil {
		return TerminalContext{}, err
	}
	event := ExecutionAudit{Time: time.Now().UTC(), Actor: o.scope.Actor, Target: o.scope.Target, Node: o.scope.Node, SSHUser: o.scope.SSHUser, OperationID: uuid.NewString(), UID: -1, Phase: "screen_read", Reason: "identity_unknown"}
	if prompt := view.LastPrompt; prompt != nil {
		event.UID = prompt.UID
		event.PID = prompt.PID
		event.Sequence = prompt.Sequence
		event.DirectoryHash = digest(prompt.Directory)
		event.Reason = "historical_prompt"
	}
	if err = o.journal.Append(o.ctx, event); err != nil {
		return TerminalContext{}, errors.Join(ErrExecutionAudit, err)
	}
	if o.ctx.Err() != nil || !o.scope.Expires.After(time.Now()) {
		return TerminalContext{}, ErrDenied
	}
	return view, nil
}

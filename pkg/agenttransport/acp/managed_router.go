package acp

import (
	"context"
	"encoding/json"
	"sync"

	"opscopilot/pkg/agenttransport/mcp"
	"opscopilot/pkg/assistantcontrol"
)

// A binding is immutable. Requests already received keep their original turn,
// even if their goroutine runs after a later prompt has been admitted.
type managedBinding struct {
	bridge *Interactions
	tools  ManagedTools
	ctx    context.Context
	cancel context.CancelFunc
}
type managedRouter struct {
	mu      sync.RWMutex
	current *managedBinding
}

func (r *managedRouter) bind(turn *assistantcontrol.Turn, tools ManagedTools, sessionID string, audit ...func(string, string, json.RawMessage) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil {
		return assistantcontrol.ErrBusy
	}
	if tools == nil || turn.Context().Err() != nil {
		return assistantcontrol.ErrStale
	}
	ctx, cancel := context.WithCancel(turn.Context())
	b := &managedBinding{ctx: ctx, cancel: cancel, tools: tools}
	b.bridge = &Interactions{Turn: turn, SessionID: sessionID, OnUpdate: func(_ string, raw json.RawMessage) {
		var params struct {
			Update struct {
				Type    string `json:"sessionUpdate"`
				Status  string `json:"status"`
				Content struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"update"`
		}
		if ctx.Err() == nil && json.Unmarshal(raw, &params) == nil && params.Update.Type == "agent_message_chunk" && params.Update.Content.Type == "text" {
			turn.AppendText(params.Update.Content.Text)
		}
		if ctx.Err() == nil && (params.Update.Type == "tool_call" || params.Update.Type == "tool_call_update") {
			if params.Update.Status == "completed" || params.Update.Status == "failed" {
				turn.SetActivity("")
			} else {
				turn.SetActivity("local")
			}
			if b.bridge.Audit != nil {
				if err := b.bridge.Audit("tool", params.Update.Status, raw); err != nil {
					turn.Finish("failed", "本机操作审计保存失败，助手已停止")
				}
			}
		}
	}}
	if len(audit) > 0 {
		b.bridge.Audit = audit[0]
	}
	r.current = b
	return nil
}
func (r *managedRouter) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil {
		r.current.cancel()
		r.current = nil
	}
}
func (r *managedRouter) capture() *managedBinding {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}
func (r *managedRouter) Capture() Handler { return r.capture() }
func (r *managedRouter) CaptureCaller() mcp.Caller {
	b := r.capture()
	if b != nil {
		if scoped, ok := b.tools.(interface {
			CaptureCall() func(context.Context, string, string, string) (any, error)
		}); ok {
			return &capturedToolCaller{binding: b, call: scoped.CaptureCall()}
		}
	}
	return b
}

type capturedToolCaller struct {
	binding *managedBinding
	call    func(context.Context, string, string, string) (any, error)
}

func (c *capturedToolCaller) Call(ctx context.Context, name, argument string) (any, error) {
	return c.CallWithPurpose(ctx, name, argument, "")
}
func (c *capturedToolCaller) CallWithPurpose(ctx context.Context, name, argument, purpose string) (any, error) {
	if c.binding.ctx.Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.binding.ctx, cancel)
	defer func() { stop(); cancel() }()
	return c.call(ctx, name, argument, purpose)
}
func (r *managedRouter) Notify(ctx context.Context, method string, data json.RawMessage) {
	r.capture().Notify(ctx, method, data)
}
func (r *managedRouter) Request(ctx context.Context, method string, data json.RawMessage) (any, error) {
	return r.capture().Request(ctx, method, data)
}
func (b *managedBinding) Notify(ctx context.Context, method string, data json.RawMessage) {
	if b != nil && b.ctx.Err() == nil {
		b.bridge.Notify(ctx, method, data)
	}
}
func (b *managedBinding) Request(ctx context.Context, method string, data json.RawMessage) (any, error) {
	if b == nil || b.ctx.Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(b.ctx, cancel)
	defer func() { stop(); cancel() }()
	return b.bridge.Request(ctx, method, data)
}
func (r *managedRouter) Call(ctx context.Context, name, command string) (any, error) {
	return r.CallWithPurpose(ctx, name, command, "")
}
func (r *managedRouter) CallWithPurpose(ctx context.Context, name, command, purpose string) (any, error) {
	b := r.capture()
	return b.CallWithPurpose(ctx, name, command, purpose)
}
func (b *managedBinding) Call(ctx context.Context, name, command string) (any, error) {
	return b.CallWithPurpose(ctx, name, command, "")
}
func (b *managedBinding) CallWithPurpose(ctx context.Context, name, command, purpose string) (any, error) {
	if b == nil || b.ctx.Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(b.ctx, cancel)
	defer func() { stop(); cancel() }()
	if b.ctx.Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	if tools, ok := b.tools.(interface {
		CallWithPurpose(context.Context, string, string, string) (any, error)
	}); ok {
		return tools.CallWithPurpose(ctx, name, command, purpose)
	}
	return b.tools.Call(ctx, name, command)
}

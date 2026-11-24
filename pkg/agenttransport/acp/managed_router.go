package acp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"opscopilot/pkg/agenttransport/mcp"
	"opscopilot/pkg/assistantcontrol"
)

// A binding is immutable. Requests already received keep their original turn,
// even if their goroutine runs after a later prompt has been admitted.
type managedBinding struct {
	bridge             *Interactions
	tools              ManagedTools
	ctx                context.Context
	cancel             context.CancelFunc
	turn               *assistantcontrol.Turn
	permissionMu       sync.Mutex
	pendingPermissions map[string]int
	deniedCalls        map[string]bool
}
type managedRouter struct {
	mu              sync.RWMutex
	current         *managedBinding
	denyRequests    bool
	observeUsage    func(json.RawMessage)
	observeFeatures func(json.RawMessage)
	sessionID       string
	idleUpdates     []json.RawMessage
	remembered      map[string]string
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
	b := &managedBinding{ctx: ctx, cancel: cancel, tools: tools, turn: turn, pendingPermissions: make(map[string]int), deniedCalls: make(map[string]bool)}
	feed := &executionFeed{}
	b.bridge = &Interactions{Turn: turn, SessionID: sessionID, OnUpdate: func(_ string, raw json.RawMessage) {
		if ctx.Err() == nil && r.observeUsage != nil {
			r.observeUsage(raw)
		}
		if ctx.Err() == nil && r.observeFeatures != nil {
			r.observeFeatures(raw)
		}
		var params struct {
			Update struct {
				Type      string `json:"sessionUpdate"`
				Status    string `json:"status"`
				MessageID string `json:"messageId"`
				CallID    string `json:"toolCallId"`
				Content   struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"update"`
		}
		if ctx.Err() == nil && json.Unmarshal(raw, &params) == nil && params.Update.Type == "agent_message_chunk" && params.Update.Content.Type == "text" {
			turn.AppendSegment(params.Update.Content.Text, params.Update.MessageID)
		}
		if ctx.Err() == nil && (params.Update.Type == "tool_call" || params.Update.Type == "tool_call_update") {
			turn.TrackTool(params.Update.CallID, params.Update.Status)
			if params.Update.Status == "completed" || params.Update.Status == "failed" {
				turn.SetActivity("")
			} else {
				turn.SetActivity("local")
			}
			if b.bridge.Audit != nil {
				if err := b.bridge.Audit("tool", params.Update.Status, raw); err != nil {
					turn.Finish("failed", "本机操作审计保存失败，助手已停止")
					return
				}
			}
			feed.observe(turn, raw)
		}
	}}
	b.bridge.DenyRequests = r.denyRequests
	b.bridge.DeferHostPermission = b.deferHostPermission
	b.bridge.Remembered = r.rememberedDecision
	b.bridge.Remember = r.rememberDecision
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

func (r *managedRouter) setPermissionReview(reviewer assistantcontrol.Reviewer, question, agentName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current != nil {
		r.current.bridge.PermissionReview = reviewer
		r.current.bridge.UserRequest = question
		r.current.bridge.AgentName = agentName
	}
}
func (r *managedRouter) capture() *managedBinding {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}

// rememberedDecision returns a session-scoped allow/reject decision recorded
// from an allow_always permission option, or "" when nothing was remembered.
func (r *managedRouter) rememberedDecision(key string) string {
	if key == "" {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.remembered[key]
}

func (r *managedRouter) rememberDecision(key, decision string) {
	if key == "" || decision == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.remembered == nil {
		r.remembered = make(map[string]string)
	}
	if len(r.remembered) < 128 {
		r.remembered[key] = decision
	}
}
func (r *managedRouter) Capture() Handler {
	if binding := r.capture(); binding != nil {
		return binding
	}
	return idleFeaturesHandler{router: r}
}

// Idle notifications may describe agent commands and model choices, but never
// acquire a turn or expose tool access. Keep startup updates until session/new
// returns, then verify their session identity before applying them.
type idleFeaturesHandler struct{ router *managedRouter }

func (h idleFeaturesHandler) Request(context.Context, string, json.RawMessage) (any, error) {
	return nil, &RPCError{Code: -32601, Message: "当前没有活动的助手操作"}
}
func (h idleFeaturesHandler) Notify(ctx context.Context, method string, raw json.RawMessage) {
	if ctx.Err() != nil || method != "session/update" {
		return
	}
	var message struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			Type string `json:"sessionUpdate"`
		} `json:"update"`
	}
	if json.Unmarshal(raw, &message) != nil || message.SessionID == "" || (message.Update.Type != "available_commands_update" && message.Update.Type != "config_option_update") {
		return
	}
	r := h.router
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessionID == "" {
		if len(r.idleUpdates) < 16 {
			r.idleUpdates = append(r.idleUpdates, append(json.RawMessage(nil), raw...))
		}
		return
	}
	if message.SessionID == r.sessionID && r.observeFeatures != nil {
		r.observeFeatures(raw)
	}
}
func (r *managedRouter) setSession(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessionID = id
	for _, raw := range r.idleUpdates {
		var message struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(raw, &message) == nil && message.SessionID == id && r.observeFeatures != nil {
			r.observeFeatures(raw)
		}
	}
	r.idleUpdates = nil
}
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

func (c *capturedToolCaller) CallWithArguments(ctx context.Context, name, argument, purpose string, args json.RawMessage) (any, error) {
	if c.binding.ctx.Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.binding.ctx, cancel)
	defer func() { stop(); cancel() }()
	ctx, err := c.binding.authorizeCall(ctx, name, args)
	if err != nil {
		return nil, err
	}
	return c.call(ctx, name, argument, purpose)
}

func hostToolName(name string) string {
	names := append(append(mcp.DatabaseToolNames(), mcp.FileToolNames()...), mcp.LocalDirectoryToolNames()...)
	names = append(names, mcp.ReferenceToolNames()...)
	names = append(names, "workspace_list", "workspace_open", "workspace_select", "terminal_context", "terminal_take", "terminal_release", "terminal_execute")
	for _, tool := range names {
		if name == tool || name == "opscopilot_"+tool || name == "opscopilot__"+tool {
			return tool
		}
	}
	return ""
}

// permissionTitle prefers the agent-provided title and falls back to the real
// tool name restored from private metadata, so approval cards are never blank.
func permissionTitle(title, hostName string) string {
	if strings.TrimSpace(title) != "" {
		return title
	}
	return hostName
}

func permissionKey(name string, raw json.RawMessage) string {
	var args map[string]any
	if json.Unmarshal(raw, &args) != nil || args == nil {
		return ""
	}
	canonical, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return name + "\x00" + string(canonical)
}

func (b *managedBinding) deferHostPermission(name string, args json.RawMessage) bool {
	tool := hostToolName(name)
	key := permissionKey(tool, args)
	if tool == "" || key == "" || b.ctx.Err() != nil {
		return false
	}
	b.permissionMu.Lock()
	defer b.permissionMu.Unlock()
	if len(b.pendingPermissions) >= 128 {
		return false
	}
	b.pendingPermissions[key]++
	return true
}

func executionTool(name string) bool {
	switch name {
	case "terminal_take", "terminal_execute", "workspace_open", "workspace_select", "database_open", "database_create", "database_take", "database_execute", "database_close", "file_open", "file_take", "file_transfer", "file_cancel", "local_add_directories":
		return true
	}
	return false
}

func (b *managedBinding) authorizeCall(ctx context.Context, name string, args json.RawMessage) (context.Context, error) {
	key := permissionKey(name, args)
	b.permissionMu.Lock()
	if b.deniedCalls[key] {
		b.permissionMu.Unlock()
		return ctx, assistantcontrol.ErrActionDenied
	}
	pending := b.pendingPermissions[key] > 0
	if pending {
		b.pendingPermissions[key]--
		if b.pendingPermissions[key] == 0 {
			delete(b.pendingPermissions, key)
		}
	}
	hasOtherPending := len(b.pendingPermissions) > 0
	b.permissionMu.Unlock()
	if !pending && hasOtherPending {
		return ctx, errors.New("待确认的操作与工具调用不一致，未执行；请重新发起")
	}
	if b.turn.ApprovalMode() == "bypass" || (!pending && !(b.turn.ApprovalMode() == "manual" && executionTool(name))) {
		return ctx, nil
	}
	label := map[string]string{"terminal_execute": "执行远程命令", "terminal_take": "接管终端", "workspace_open": "打开终端连接", "workspace_select": "切换终端", "database_create": "创建数据库连接", "database_open": "打开数据库连接", "database_take": "接管数据库页签", "database_execute": "执行 SQL", "database_close": "关闭数据库连接", "file_open": "打开文件页签", "file_take": "接管文件页签", "file_transfer": "传输文件", "file_cancel": "取消文件传输", "local_add_directories": "添加本地目录"}[name]
	if label == "" {
		label = "使用助手工具"
	}
	location := ""
	if source, ok := b.tools.(interface{ ApprovalLocation(string) string }); ok {
		location = source.ApprovalLocation(name)
	}
	input := assistantcontrol.ReviewInput{Tool: name, Location: location, Action: string(args)}
	details := []assistantcontrol.Detail{{Label: "操作", Value: label}}
	if location != "" {
		details = append(details, assistantcontrol.Detail{Label: "执行位置", Value: assistantcontrol.DisplayExcerpt(location, 512)})
	}
	content := string(args)
	title := "确认这一次操作"
	var fields map[string]json.RawMessage
	if json.Unmarshal(args, &fields) == nil {
		var value string
		if raw := fields["command"]; len(raw) > 0 && json.Unmarshal(raw, &value) == nil {
			content = value
		}
		if raw := fields["sql"]; len(raw) > 0 && json.Unmarshal(raw, &value) == nil {
			content = value
		}
		if name == "file_transfer" {
			title, content = "确认这一次文件传输", ""
			for _, field := range []struct{ key, label string }{{"direction", "传输方向"}, {"local_path", "本机文件"}, {"remote_path", "远端文件"}, {"purpose", "目的"}} {
				if raw := fields[field.key]; len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value != "" {
					details = append(details, assistantcontrol.Detail{Label: field.label, Value: assistantcontrol.DisplayExcerpt(value, 2048)})
				}
			}
		}
		if name == "terminal_execute" {
			title = "确认这一次终端操作"
		}
		if name == "database_execute" {
			title = "确认这一次数据库操作"
		}
	}
	prompt := assistantcontrol.Prompt{Kind: "approval", Title: title, Description: "请核对操作内容。批准只适用于这一次调用。", Command: assistantcontrol.DisplayExcerpt(content, 8192), Details: details, Choices: []assistantcontrol.Choice{{ID: "reject", Label: "拒绝"}, {ID: "allow", Label: "允许本次"}}, Expires: time.Now().Add(5 * time.Minute)}
	if err := b.turn.ConfirmAction(ctx, input, prompt, assistantcontrol.ReviewVerdict{}); err != nil {
		if errors.Is(err, assistantcontrol.ErrActionDenied) {
			b.permissionMu.Lock()
			if b.deniedCalls == nil {
				b.deniedCalls = make(map[string]bool)
			}
			b.deniedCalls[key] = true
			b.permissionMu.Unlock()
		}
		return ctx, err
	}
	return assistantcontrol.WithCallApproval(ctx), nil
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

func (b *managedBinding) CallWithArguments(ctx context.Context, name, command, purpose string, args json.RawMessage) (any, error) {
	if b == nil || b.ctx.Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	ctx, err := b.authorizeCall(ctx, name, args)
	if err != nil {
		return nil, err
	}
	return b.CallWithPurpose(ctx, name, command, purpose)
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

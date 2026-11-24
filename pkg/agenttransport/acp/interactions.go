package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

// Interactions adapts only explicit user questions/one-shot permissions. It
// cannot approve terminal Access or implement ACP filesystem/terminal tools.
// The host binds SessionID after session/new, never from a model argument.
type Interactions struct {
	Turn          *assistantcontrol.Turn
	SessionID     string
	OnUpdate      func(string, json.RawMessage)
	BeforeRequest func() error
	// Optional restricted callers can refuse all executable requests. The desktop
	// managed profile leaves this false and presents native one-shot approvals.
	DenyPermissions bool
	DenyRequests    bool
	Audit           func(event, status string, data json.RawMessage) error
	// PermissionReview is supplied only by adapters that need host-side review.
	// It has no effect on iCode's own approval path.
	PermissionReview assistantcontrol.Reviewer
	UserRequest      string
	AgentName        string
	// Defer host-tool approvals to the actual MCP invocation. Returning true
	// does not authorize the action: the host must still ask before dispatch.
	DeferHostPermission func(string, json.RawMessage) bool
	// Remembered/Remember carry per-conversation decisions for allow_always
	// permission options. They live on the router, outliving a single turn.
	Remembered func(key string) string
	Remember   func(key, decision string)
	sessionMu  sync.RWMutex
}

func (b *Interactions) SetSessionID(id string) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	b.SessionID = id
}
func (b *Interactions) sessionID() string {
	b.sessionMu.RLock()
	defer b.sessionMu.RUnlock()
	return b.SessionID
}

func (b *Interactions) Bind(client *Client) {
	if b.Turn == nil {
		client.Close()
		return
	}
	go func() {
		select {
		case <-client.Done():
			b.Turn.Finish("stopped", "助手连接已结束，旧确认已撤销")
		case <-b.Turn.Context().Done():
			client.Close()
		}
	}()
}
func (b *Interactions) Notify(ctx context.Context, method string, data json.RawMessage) {
	if b.Turn == nil || ctx.Err() != nil || b.Turn.Context().Err() != nil || method != "session/update" || b.OnUpdate == nil {
		return
	}
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(data, &params) == nil && params.SessionID == b.sessionID() {
		b.OnUpdate(b.Turn.ID(), data)
	}
}

func (b *Interactions) Request(ctx context.Context, method string, data json.RawMessage) (any, error) {
	if b.DenyRequests {
		return nil, &RPCError{Code: -32601, Message: "自审没有交互或执行权限"}
	}
	if b.Turn == nil || b.Turn.Context().Err() != nil {
		return nil, assistantcontrol.ErrStale
	}
	// options has different shapes between methods, so decode the common identity
	// first, then the matching payload. Unknown requests never acquire authority.
	var identity struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(data, &identity) != nil || identity.SessionID == "" || identity.SessionID != b.sessionID() {
		return nil, &RPCError{Code: -32602, Message: "ACP 会话不匹配"}
	}
	request := func(p assistantcontrol.Prompt) (assistantcontrol.Answer, error) {
		if b.BeforeRequest != nil {
			if err := b.BeforeRequest(); err != nil {
				return assistantcontrol.Answer{}, err
			}
		}
		answer, err := b.Turn.Request(ctx, p)
		if err != nil {
			b.Turn.Finish("stopped", "本轮已停止，旧确认已撤销")
		}
		return answer, err
	}
	switch method {
	case "_chrys/request_input", "chrys/request_input":
		// Separate legacy options from modern permission options to avoid shape
		// ambiguity; current Chrys uses questions[] with typed answer arrays.
		var input struct {
			Question  string          `json:"question"`
			Options   []string        `json:"options"`
			Questions json.RawMessage `json:"questions"`
		}
		if json.Unmarshal(data, &input) != nil {
			return nil, &RPCError{Code: -32602, Message: "问题格式无效"}
		}
		if len(input.Questions) > 0 && string(input.Questions) != "null" {
			var modern struct {
				Questions []struct {
					Question string `json:"question"`
					Header   string `json:"header"`
					Options  []struct {
						Label       string `json:"label"`
						Description string `json:"description"`
					} `json:"options"`
					MultiSelect bool `json:"multiSelect"`
				} `json:"questions"`
			}
			if json.Unmarshal(data, &modern) != nil || len(modern.Questions) == 0 || len(modern.Questions) > 8 {
				return nil, &RPCError{Code: -32602, Message: "问题数量或格式无效"}
			}
			answers := make([]map[string]any, 0, len(modern.Questions))
			for index, q := range modern.Questions {
				choices := make([]assistantcontrol.Choice, 0, len(q.Options))
				for i, option := range q.Options {
					label := option.Label
					if option.Description != "" {
						label += " · " + option.Description
					}
					choices = append(choices, assistantcontrol.Choice{ID: strconv.Itoa(i), Label: label})
				}
				p := assistantcontrol.Prompt{Kind: "input", Title: q.Question, Description: fmt.Sprintf("问题 %d / %d", index+1, len(modern.Questions)), Choices: choices, Multiple: q.MultiSelect}
				a, err := request(p)
				if err != nil {
					return nil, err
				}
				values := make([]string, 0, len(a.Selected))
				for _, id := range a.Selected {
					i, _ := strconv.Atoi(id)
					values = append(values, q.Options[i].Label)
				}
				// iCode treats typed answers as values. A note without a selected
				// option invalidates the entire response, including other answers.
				note := a.Text
				if len(values) == 0 && strings.TrimSpace(note) != "" {
					values = append(values, note)
					note = ""
				}
				answers = append(answers, map[string]any{"values": values, "note": note})
			}
			return map[string]any{"answers": answers, "cancelled": false}, nil
		}
		choices := make([]assistantcontrol.Choice, 0, len(input.Options))
		for i, label := range input.Options {
			choices = append(choices, assistantcontrol.Choice{ID: strconv.Itoa(i), Label: label})
		}
		a, err := request(assistantcontrol.Prompt{Kind: "input", Title: input.Question, Choices: choices})
		if err != nil {
			return nil, err
		}
		parts := make([]string, 0, len(a.Selected)+1)
		for _, id := range a.Selected {
			i, _ := strconv.Atoi(id)
			parts = append(parts, input.Options[i])
		}
		if a.Text != "" {
			parts = append(parts, a.Text)
		}
		return map[string]string{"text": strings.Join(parts, "\n")}, nil
	case "elicitation/create":
		return b.elicitation(ctx, data, request)
	case "session/request_permission":
		if b.DenyPermissions {
			return map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil
		}
		var permission struct {
			ToolCall struct {
				Name   string          `json:"name"`
				Title  string          `json:"title"`
				Kind   string          `json:"kind"`
				Input  json.RawMessage `json:"rawInput"`
				Output string          `json:"rawOutput"`
				Meta   struct {
					Chrys struct {
						ToolName string `json:"toolName"`
					} `json:"chrys"`
					CodeAgentCLI struct {
						ToolName string `json:"toolName"`
					} `json:"codeAgentCLI"`
				} `json:"_meta"`
			} `json:"toolCall"`
			Options []struct {
				ID   string `json:"optionId"`
				Kind string `json:"kind"`
			} `json:"options"`
		}
		if json.Unmarshal(data, &permission) != nil {
			return nil, &RPCError{Code: -32602, Message: "审批格式无效"}
		}
		allow, allowAlways, reject, rejectAlways := "", "", "", ""
		for _, option := range permission.Options {
			switch option.Kind {
			case "allow_once":
				allow = option.ID
			case "allow_always":
				allowAlways = option.ID
			case "reject_once":
				reject = option.ID
			case "reject_always":
				rejectAlways = option.ID
			}
		}
		// Unknown kinds are ignored, never treated as allow. Agents offering only
		// one direction get cancelled rather than guessed.
		allowID, rejectID := allow, reject
		if allowID == "" {
			allowID = allowAlways
		}
		if rejectID == "" {
			rejectID = rejectAlways
		}
		if allowID == "" || rejectID == "" || allowID == rejectID {
			return map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil
		}
		if b.Audit != nil {
			if err := b.Audit("permission", "requested", data); err != nil {
				return nil, err
			}
		}
		if b.AgentName != "" && b.Turn.ApprovalMode() == "bypass" {
			if b.Audit != nil {
				if err := b.Audit("permission", "allowed", data); err != nil {
					return nil, err
				}
			}
			return map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": allowID}}, nil
		}
		hostName := permission.ToolCall.Meta.Chrys.ToolName
		if hostName == "" {
			hostName = permission.ToolCall.Meta.CodeAgentCLI.ToolName
		}
		if hostName == "" && strings.HasPrefix(permission.ToolCall.Name, "opscopilot_") {
			hostName = permission.ToolCall.Name
		}
		rememberName := hostName
		if rememberName == "" {
			rememberName = permission.ToolCall.Name
		}
		rememberKey := permissionKey(rememberName, permission.ToolCall.Input)
		// A remembered allow_always/reject decision applies without a new dialog
		// and stays fully audited. Unknown keys simply never match.
		if b.Remembered != nil {
			if decision := b.Remembered(rememberKey); decision != "" {
				id, status := rejectID, "remembered_rejected"
				if decision == "allow" {
					id, status = allowID, "remembered_allowed"
				}
				if b.Audit != nil {
					if err := b.Audit("permission", status, data); err != nil {
						return nil, err
					}
				}
				return map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": id}}, nil
			}
		}
		if b.DeferHostPermission != nil && b.DeferHostPermission(hostName, permission.ToolCall.Input) {
			if b.Audit != nil {
				if err := b.Audit("permission", "deferred_to_host", data); err != nil {
					return nil, err
				}
			}
			return map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": allowID}}, nil
		}
		details := []assistantcontrol.Detail{}
		if permission.ToolCall.Output != "" {
			label := "iCode 提示"
			if b.AgentName != "" {
				label = b.AgentName + " 提示"
			}
			details = append(details, assistantcontrol.Detail{Label: label, Value: assistantcontrol.DisplayExcerpt(permission.ToolCall.Output, 512)})
		}
		input := assistantcontrol.ReviewInput{UserRequest: b.UserRequest, Tool: permission.ToolCall.Kind, Action: string(permission.ToolCall.Input), Location: "本机"}
		verdict := assistantcontrol.ReviewVerdict{}
		if b.PermissionReview != nil && b.Turn.ApprovalMode() == "auto" {
			var reviewErr error
			verdict, reviewErr = b.Turn.ReviewAction(ctx, b.PermissionReview, input, "")
			if reviewErr != nil {
				return nil, reviewErr
			}
		}
		agentName := b.AgentName
		if agentName == "" {
			agentName = "iCode"
		}
		choices := []assistantcontrol.Choice{{ID: "reject", Label: "拒绝"}, {ID: "allow", Label: "允许本次"}}
		if allowAlways != "" {
			choices = append(choices, assistantcontrol.Choice{ID: "allow_always", Label: "本会话内始终允许"})
		}
		choice, confirmErr := b.Turn.ConfirmActionChoice(ctx, input, assistantcontrol.Prompt{Kind: "approval", Title: permissionTitle(permission.ToolCall.Title, hostName), Command: string(permission.ToolCall.Input), Description: agentName + " 需要你确认这次操作；请核对操作内容和目标。", Details: details, Choices: choices, Expires: time.Now().Add(5 * time.Minute)}, verdict, "allow_always")
		id := allowID
		if confirmErr != nil {
			if !errors.Is(confirmErr, assistantcontrol.ErrActionDenied) {
				if b.Audit != nil {
					_ = b.Audit("permission", "cancelled", data)
				}
				b.Turn.Finish("stopped", "确认未完成，未作选择或授权")
				return nil, confirmErr
			}
			id = rejectID
		} else if choice == "allow_always" && allowAlways != "" {
			id = allowAlways
			if rememberKey != "" && b.Remember != nil {
				b.Remember(rememberKey, "allow")
			}
		}
		if b.Audit != nil {
			status := "rejected"
			if id == allowID || id == allowAlways {
				status = "allowed"
			}
			if auditErr := b.Audit("permission", status, data); auditErr != nil {
				return nil, auditErr
			}
		}
		return map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": id}}, nil
	default:
		return nil, &RPCError{Code: -32601, Message: "宿主未开放该能力"}
	}
}

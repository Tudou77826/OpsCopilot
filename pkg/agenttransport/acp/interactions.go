package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/terminalcontrol"
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
	Review          assistantcontrol.Reviewer
	sessionMu       sync.RWMutex
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
	case "session/request_permission":
		if b.DenyPermissions {
			return map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil
		}
		var permission struct {
			ToolCall struct {
				Title string          `json:"title"`
				Kind  string          `json:"kind"`
				Input json.RawMessage `json:"rawInput"`
			} `json:"toolCall"`
			Options []struct {
				ID   string `json:"optionId"`
				Kind string `json:"kind"`
			} `json:"options"`
		}
		if json.Unmarshal(data, &permission) != nil {
			return nil, &RPCError{Code: -32602, Message: "审批格式无效"}
		}
		allow, reject := "", ""
		for _, option := range permission.Options {
			switch option.Kind {
			case "allow_once":
				allow = option.ID
			case "reject_once":
				reject = option.ID
			}
		}
		if allow == "" || reject == "" || allow == reject {
			return map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil
		}
		if b.Audit != nil {
			if err := b.Audit("permission", "requested", data); err != nil {
				return nil, err
			}
		}
		details := []assistantcontrol.Detail{{Label: "执行位置", Value: "本机（不是 SSH 服务器）"}}
		if b.Review != nil {
			b.Turn.SetActivity("reviewing")
			verdict := b.Review(ctx, assistantcontrol.ReviewInput{Location: "本机", Tool: permission.ToolCall.Kind, Action: string(permission.ToolCall.Input)})
			if ctx.Err() != nil || b.Turn.Context().Err() != nil {
				return nil, assistantcontrol.ErrStale
			}
			// File writes never qualify for read-only automatic permission.
			kind := permission.ToolCall.Kind
			approved := verdict.Allowed() && (kind == "read" || kind == "search" || kind == "execute") && !terminalcontrol.RequiresHumanReview(string(permission.ToolCall.Input))
			if verdict.Allowed() && !approved {
				verdict.Reason = "操作类型或敏感目标触及人工确认边界"
			}
			status := "human_required"
			if approved {
				status = "auto_allowed"
			}
			if b.Audit != nil {
				encoded, _ := json.Marshal(map[string]any{"action": json.RawMessage(data), "verdict": verdict})
				if err := b.Audit("self_review", status, encoded); err != nil {
					return nil, err
				}
			}
			b.Turn.Record("review", map[bool]string{true: "只读自审通过：", false: "需要人工确认："}[approved]+verdict.Reason)
			if approved {
				return map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": allow}}, nil
			}
			details = append(details, assistantcontrol.Detail{Label: "AI 自审", Value: verdict.Reason})
			if verdict.Notice != "" {
				details = append(details, assistantcontrol.Detail{Label: "自审方式", Value: verdict.Notice})
			}
		}
		a, err := request(assistantcontrol.Prompt{Kind: "approval", Title: permission.ToolCall.Title, Command: string(permission.ToolCall.Input), Description: "此操作需要你确认。授权仅对本次操作有效。", Details: details, Choices: []assistantcontrol.Choice{{ID: "reject", Label: "拒绝"}, {ID: "allow", Label: "允许本次"}}, Expires: time.Now().Add(5 * time.Minute)})
		if err != nil {
			return nil, err
		}
		id := reject
		if len(a.Selected) == 1 && a.Selected[0] == "allow" {
			id = allow
		}
		if b.Audit != nil {
			status := "rejected"
			if id == allow {
				status = "allowed"
			}
			if err = b.Audit("permission", status, data); err != nil {
				return nil, err
			}
		}
		return map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": id}}, nil
	default:
		return nil, &RPCError{Code: -32601, Message: "宿主未开放该能力"}
	}
}

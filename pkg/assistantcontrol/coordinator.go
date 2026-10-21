// Package assistantcontrol owns a single application-side assistant turn and
// its user interactions. It has no Wails, ACP, model or terminal dependencies.
package assistantcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrBusy = errors.New("助手仍有一轮操作，请先停止或等待完成")
var ErrStale = errors.New("这条确认已经结束，请查看最新状态")

type Target struct {
	TabID        string `json:"tabId"`
	ConnectionID string `json:"connectionId"`
}
type Choice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Detail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type Prompt struct {
	Data        json.RawMessage `json:"data,omitempty"`
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Command     string          `json:"command,omitempty"`
	Details     []Detail        `json:"details,omitempty"`
	Choices     []Choice        `json:"choices,omitempty"`
	Multiple    bool            `json:"multiple"`
	Expires     time.Time       `json:"expires"`
}
type Answer struct {
	Text     string   `json:"text"`
	Selected []string `json:"selected"`
}
type State struct {
	TurnID   string  `json:"turnId"`
	Target   Target  `json:"target"`
	Phase    string  `json:"phase"`
	Message  string  `json:"message"`
	Prompt   *Prompt `json:"prompt,omitempty"`
	Output   string  `json:"output,omitempty"`
	Source   string  `json:"source,omitempty"`
	Activity string  `json:"activity,omitempty"`
}
type pending struct {
	prompt Prompt
	result chan Answer
}
type Coordinator struct {
	mu      sync.Mutex
	current *Turn
}
type Turn struct {
	owner  *Coordinator
	ctx    context.Context
	cancel context.CancelFunc
	state  State
	wait   *pending
	yield  func() error
}

func active(phase string) bool { return phase == "running" || phase == "waiting" }
func (c *Coordinator) Begin(ctx context.Context, target Target, yield func() error) (*Turn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if c.current != nil && active(c.current.state.Phase) {
		return nil, ErrBusy
	}
	ctx, cancel := context.WithCancel(ctx)
	t := &Turn{owner: c, ctx: ctx, cancel: cancel, yield: yield, state: State{TurnID: uuid.NewString(), Target: target, Phase: "running", Message: "正在处理"}}
	c.current = t
	context.AfterFunc(ctx, func() { t.Finish("stopped", "已停止，旧确认已撤销") })
	return t, nil
}
func (t *Turn) ID() string               { return t.state.TurnID }
func (t *Turn) Context() context.Context { return t.ctx }

// BindTarget is called only by the host when a tool first needs a resource.
// Ordinary conversation has no target and does not depend on its lifecycle.
func (t *Turn) BindTarget(target Target) error {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if t.owner.current != t || !active(t.state.Phase) || t.ctx.Err() != nil {
		return ErrStale
	}
	if target.TabID == "" || target.ConnectionID == "" || (t.state.Target != (Target{}) && t.state.Target != target) {
		return errors.New("本轮工具目标不能更换，请基于新的连接重新发起操作")
	}
	t.state.Target = target
	return nil
}

// ChangeTarget is host-only. The caller must revoke the old resource before
// calling it; model arguments cannot preserve approvals across this transition.
func (t *Turn) ChangeTarget(target Target) error {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if t.owner.current != t || !active(t.state.Phase) || t.ctx.Err() != nil || t.wait != nil {
		return ErrStale
	}
	if (target.TabID == "") != (target.ConnectionID == "") {
		return errors.New("目标连接编号不完整")
	}
	t.state.Target = target
	return nil
}
func (t *Turn) SetSource(source string) {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if t.owner.current == t && active(t.state.Phase) {
		t.state.Source = source
	}
}
func (t *Turn) SetActivity(activity string) {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if t.owner.current == t && active(t.state.Phase) && t.ctx.Err() == nil {
		t.state.Activity = activity
	}
}

// AppendText fences late agent notifications and bounds UI memory per turn.
func (t *Turn) AppendText(text string) {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if t.owner.current != t || !active(t.state.Phase) || t.ctx.Err() != nil {
		return
	}
	if len(t.state.Output)+len(text) > 128*1024 {
		return
	}
	t.state.Output += text
}
func (t *Turn) Snapshot() State {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	state := t.state
	if state.Prompt != nil {
		prompt := clonePrompt(*state.Prompt)
		state.Prompt = &prompt
	}
	return state
}
func clonePrompt(prompt Prompt) Prompt {
	prompt.Data = append(json.RawMessage(nil), prompt.Data...)
	prompt.Details = append([]Detail(nil), prompt.Details...)
	prompt.Choices = append([]Choice(nil), prompt.Choices...)
	return prompt
}
func (c *Coordinator) Snapshot() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return State{Phase: "idle"}
	}
	state := c.current.state
	if state.Prompt != nil {
		prompt := clonePrompt(*state.Prompt)
		state.Prompt = &prompt
	}
	return state
}
func (c *Coordinator) ClearFinished(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != nil && c.current.state.TurnID == id && !active(c.current.state.Phase) {
		c.current = nil
	}
}
func (t *Turn) Finish(phase, message string) {
	if phase != "completed" && phase != "failed" && phase != "stopped" {
		phase = "failed"
	}
	t.owner.mu.Lock()
	if t.owner.current == t && active(t.state.Phase) {
		t.state.Phase, t.state.Message, t.state.Prompt = phase, message, nil
		t.wait = nil
	}
	t.owner.mu.Unlock()
	t.cancel()
}
func (c *Coordinator) Stop(id string) error {
	c.mu.Lock()
	t := c.current
	if t == nil || t.state.TurnID != id {
		c.mu.Unlock()
		return ErrStale
	}
	c.mu.Unlock()
	t.Finish("stopped", "已停止，旧确认已撤销")
	return nil
}

func validatePrompt(p Prompt) error {
	if (p.Kind != "approval" && p.Kind != "input" && p.Kind != "database_connection") || p.Title == "" || len(p.Title) > 512 || len(p.Description) > 8192 || len(p.Command) > 8192 || len(p.Data) > 8192 || len(p.Choices) > 16 || len(p.Details) > 16 || !p.Expires.After(time.Now()) || p.Expires.After(time.Now().Add(10*time.Minute)) {
		return errors.New("交互请求无效或已过期")
	}
	seen := map[string]bool{}
	for _, option := range p.Choices {
		if option.ID == "" || option.Label == "" || len(option.ID) > 128 || len(option.Label) > 512 || seen[option.ID] {
			return errors.New("交互选项无效")
		}
		seen[option.ID] = true
	}
	for _, d := range p.Details {
		if len(d.Label) > 128 || len(d.Value) > 2048 {
			return errors.New("交互详情过长")
		}
	}
	if p.Kind == "approval" && (p.Multiple || len(p.Choices) != 2 || !seen["allow"] || !seen["reject"]) {
		return errors.New("审批只能明确批准本次或拒绝")
	}
	if p.Kind == "database_connection" && (!json.Valid(p.Data) || p.Multiple || len(p.Choices) != 2 || !seen["allow"] || !seen["reject"]) {
		return errors.New("连接表单请求无效")
	}
	return nil
}
func (t *Turn) Request(ctx context.Context, prompt Prompt) (Answer, error) {
	if err := validatePrompt(prompt); err != nil {
		return Answer{}, err
	}
	prompt = clonePrompt(prompt)
	prompt.ID = uuid.NewString()
	wait := &pending{prompt: prompt, result: make(chan Answer, 1)}
	t.owner.mu.Lock()
	if t.owner.current != t || !active(t.state.Phase) || t.ctx.Err() != nil || ctx.Err() != nil {
		t.owner.mu.Unlock()
		return Answer{}, ErrStale
	}
	if t.wait != nil {
		t.owner.mu.Unlock()
		return Answer{}, ErrBusy
	}
	// Reserve the interaction before yielding. A concurrent request cannot
	// release resources a second time while the first yield is still running.
	t.wait = wait
	t.owner.mu.Unlock()
	defer func() {
		t.owner.mu.Lock()
		if t.wait == wait {
			t.wait = nil
			t.state.Prompt = nil
			if active(t.state.Phase) {
				t.state.Phase, t.state.Message = "running", "正在继续"
			}
		}
		t.owner.mu.Unlock()
	}()
	if t.yield != nil {
		if err := t.yield(); err != nil {
			return Answer{}, err
		}
	}
	t.owner.mu.Lock()
	if t.owner.current != t || t.wait != wait || t.ctx.Err() != nil || ctx.Err() != nil || !prompt.Expires.After(time.Now()) {
		t.owner.mu.Unlock()
		return Answer{}, ErrStale
	}
	t.state.Phase, t.state.Message, t.state.Prompt = "waiting", "等待你的确认", &wait.prompt
	t.owner.mu.Unlock()
	timer := time.NewTimer(time.Until(prompt.Expires))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return Answer{}, ctx.Err()
	case <-t.ctx.Done():
		return Answer{}, t.ctx.Err()
	case <-timer.C:
		return Answer{}, context.DeadlineExceeded
	case answer := <-wait.result:
		if t.ctx.Err() != nil || ctx.Err() != nil {
			return Answer{}, ErrStale
		}
		return answer, nil
	}
}
func (c *Coordinator) Respond(turnID, requestID string, answer Answer) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.current
	if t == nil || t.state.TurnID != turnID || t.ctx.Err() != nil || t.wait == nil || t.state.Phase != "waiting" || t.wait.prompt.ID != requestID || !t.wait.prompt.Expires.After(time.Now()) {
		return ErrStale
	}
	p := t.wait.prompt
	if len(answer.Text) > 8192 || len(answer.Selected) > 16 || (!p.Multiple && len(answer.Selected) > 1) {
		return errors.New("请选择有效选项或缩短回答")
	}
	seen := map[string]bool{}
	for _, id := range answer.Selected {
		found := false
		for _, option := range p.Choices {
			if option.ID == id {
				found = true
				break
			}
		}
		if !found || seen[id] {
			return errors.New("选择不在本条问题中")
		}
		seen[id] = true
	}
	if p.Kind == "approval" && (len(answer.Selected) != 1 || answer.Text != "") {
		return errors.New("请明确选择批准本次或拒绝")
	}
	if p.Kind == "input" && len(answer.Selected) == 0 && answer.Text == "" {
		return errors.New("请填写回答")
	}
	if p.Kind == "database_connection" {
		if len(answer.Selected) != 1 {
			return errors.New("请保存新连接或取消")
		}
		if answer.Selected[0] == "allow" {
			if _, err := uuid.Parse(answer.Text); err != nil {
				return errors.New("保存的连接编号无效")
			}
		} else if answer.Text != "" {
			return errors.New("取消连接时不能提交其他内容")
		}
	}
	answer.Selected = append([]string(nil), answer.Selected...)
	t.wait.result <- answer
	t.wait = nil
	t.state.Phase, t.state.Message, t.state.Prompt = "running", "正在继续", nil
	return nil
}

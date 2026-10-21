package assistantcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func awaitPrompt(t *testing.T, c *Coordinator) State {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s := c.Snapshot()
		if s.Prompt != nil {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("prompt missing")
	return State{}
}
func approval() Prompt {
	return Prompt{Kind: "approval", Title: "操作确认", Expires: time.Now().Add(time.Minute), Choices: []Choice{{ID: "allow", Label: "本次允许"}, {ID: "reject", Label: "拒绝"}}}
}

func TestDatabaseConnectionFormCarriesOnlySavedProfileReceipt(t *testing.T) {
	c := &Coordinator{}
	turn, _ := c.Begin(context.Background(), Target{}, nil)
	defer turn.Finish("completed", "")
	p := approval()
	p.Kind = "database_connection"
	p.Data = json.RawMessage(`{"host":"127.0.0.1"}`)
	answers := make(chan Answer, 1)
	go func() { answer, _ := turn.Request(context.Background(), p); answers <- answer }()
	s := awaitPrompt(t, c)
	s.Prompt.Data[0] = 'X'
	if c.Snapshot().Prompt.Data[0] != '{' {
		t.Fatal("caller changed pending form data")
	}
	if err := c.Respond(s.TurnID, s.Prompt.ID, Answer{Selected: []string{"allow"}, Text: "password-instead-of-id"}); err == nil {
		t.Fatal("form accepted arbitrary credential text")
	}
	id := uuid.NewString()
	if err := c.Respond(s.TurnID, s.Prompt.ID, Answer{Selected: []string{"allow"}, Text: id}); err != nil {
		t.Fatal(err)
	}
	if answer := <-answers; answer.Text != id {
		t.Fatal("saved profile receipt lost")
	}
	if err := c.Respond(s.TurnID, s.Prompt.ID, Answer{Selected: []string{"allow"}, Text: id}); !errors.Is(err, ErrStale) {
		t.Fatal("duplicate form response admitted", err)
	}
	p.Data = json.RawMessage(`invalid`)
	if validatePrompt(p) == nil {
		t.Fatal("malformed form data admitted")
	}
}
func TestOptionalResourceBindingIsHostControlledAndFenced(t *testing.T) {
	c := &Coordinator{}
	turn, _ := c.Begin(context.Background(), Target{}, nil)
	if turn.Snapshot().Target != (Target{}) {
		t.Fatal("chat requires target")
	}
	target := Target{TabID: "one", ConnectionID: "connection"}
	if err := turn.BindTarget(target); err != nil {
		t.Fatal(err)
	}
	if err := turn.BindTarget(target); err != nil {
		t.Fatal(err)
	}
	if err := turn.BindTarget(Target{TabID: "other", ConnectionID: "other"}); err == nil {
		t.Fatal("target silently switched")
	}
	turn.Finish("completed", "")
	if err := turn.BindTarget(target); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}
func TestInteractionsYieldAndFenceResponses(t *testing.T) {
	c := &Coordinator{}
	yielded := false
	turn, err := c.Begin(context.Background(), Target{TabID: "one"}, func() error { yielded = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer turn.Finish("completed", "done")
	if _, err := c.Begin(context.Background(), Target{}, nil); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	answers := make(chan Answer, 1)
	go func() { answer, _ := turn.Request(context.Background(), approval()); answers <- answer }()
	s := awaitPrompt(t, c)
	if !yielded {
		t.Fatal("published before yielding")
	}
	s.Prompt.Choices[0].ID = "forged"
	if err := c.Respond(s.TurnID, s.Prompt.ID, Answer{Selected: []string{"forged"}}); err == nil {
		t.Fatal("accepted foreign option")
	}
	if err := c.Respond(s.TurnID, s.Prompt.ID, Answer{Text: "approved=true"}); err == nil {
		t.Fatal("accepted text approval")
	}
	if err := c.Respond(s.TurnID, s.Prompt.ID, Answer{Selected: []string{"allow"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Respond(s.TurnID, s.Prompt.ID, Answer{Selected: []string{"allow"}}); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	select {
	case a := <-answers:
		if len(a.Selected) != 1 || a.Selected[0] != "allow" {
			t.Fatal(a)
		}
	case <-time.After(time.Second):
		t.Fatal("not answered")
	}
}
func TestStopWithdrawsQuestionAndLateAnswerCannotResume(t *testing.T) {
	c := &Coordinator{}
	turn, _ := c.Begin(context.Background(), Target{TabID: "one"}, nil)
	done := make(chan error, 1)
	go func() {
		_, err := turn.Request(context.Background(), Prompt{Kind: "input", Title: "查看哪份日志？", Expires: time.Now().Add(time.Minute)})
		done <- err
	}()
	s := awaitPrompt(t, c)
	if err := c.Stop(turn.ID()); err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Prompt != nil {
		t.Fatal("stopped question still visible")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stop returned an answer")
		}
	case <-time.After(time.Second):
		t.Fatal("request stuck")
	}
	fresh, err := c.Begin(context.Background(), Target{TabID: "two"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Finish("completed", "")
	if err := c.Respond(s.TurnID, s.Prompt.ID, Answer{Text: "late"}); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if err := c.Stop(turn.ID()); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	turn.Finish("failed", "late error")
	if c.Snapshot().TurnID != fresh.ID() || c.Snapshot().Phase != "running" {
		t.Fatal(c.Snapshot())
	}
}
func TestQuestionChoiceValidationAndExpiry(t *testing.T) {
	c := &Coordinator{}
	turn, _ := c.Begin(context.Background(), Target{}, nil)
	defer turn.Finish("completed", "")
	done := make(chan error, 1)
	go func() {
		_, err := turn.Request(context.Background(), Prompt{Kind: "input", Title: "选择节点", Multiple: true, Choices: []Choice{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Expires: time.Now().Add(time.Minute)})
		done <- err
	}()
	s := awaitPrompt(t, c)
	if err := c.Respond(s.TurnID, s.Prompt.ID, Answer{Selected: []string{"a", "a"}}); err == nil {
		t.Fatal("duplicate option")
	}
	if err := c.Respond(s.TurnID, s.Prompt.ID, Answer{Selected: []string{"a", "b"}, Text: "补充"}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_, err := turn.Request(context.Background(), Prompt{Kind: "input", Title: "过期", Expires: time.Now().Add(10 * time.Millisecond)})
	if !errors.Is(err, context.DeadlineExceeded) || c.Snapshot().Prompt != nil {
		t.Fatal(err, c.Snapshot())
	}
}

func TestStopAndConcurrentRequestDuringYieldCannotPublishStaleQuestion(t *testing.T) {
	c := &Coordinator{}
	entered, release := make(chan struct{}), make(chan struct{})
	turn, _ := c.Begin(context.Background(), Target{}, func() error { close(entered); <-release; return nil })
	done := make(chan error, 1)
	go func() { _, err := turn.Request(context.Background(), approval()); done <- err }()
	<-entered
	if c.Snapshot().Prompt != nil {
		t.Fatal("question published before resources were released")
	}
	if _, err := turn.Request(context.Background(), approval()); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	if err := c.Stop(turn.ID()); err != nil {
		t.Fatal(err)
	}
	fresh, err := c.Begin(context.Background(), Target{TabID: "fresh"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Finish("completed", "")
	close(release)
	if err := <-done; !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if state := c.Snapshot(); state.TurnID != fresh.ID() || state.Prompt != nil {
		t.Fatal(state)
	}
}

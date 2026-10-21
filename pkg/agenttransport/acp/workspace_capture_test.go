package acp

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"opscopilot/pkg/assistantcontrol"
)

type generationTools struct {
	generation atomic.Int32
	called     atomic.Int32
}

func (g *generationTools) Call(ctx context.Context, name, argument string) (any, error) {
	return g.CaptureCall()(ctx, name, argument, "")
}
func (g *generationTools) CaptureCall() func(context.Context, string, string, string) (any, error) {
	generation := g.generation.Load()
	return func(context.Context, string, string, string) (any, error) {
		if generation != g.generation.Load() {
			return nil, assistantcontrol.ErrStale
		}
		g.called.Add(1)
		return "ok", nil
	}
}
func TestManagedMCPArrivalCapturesTargetGeneration(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	g := &generationTools{}
	r := &managedRouter{}
	if err := r.bind(turn, g, "session"); err != nil {
		t.Fatal(err)
	}
	defer r.clear()
	old := r.CaptureCaller()
	g.generation.Add(1)
	if _, err := old.Call(turn.Context(), "terminal_execute", "pwd"); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal("old MCP request inherited new target", err)
	}
	if _, err := r.CaptureCaller().Call(turn.Context(), "terminal_context", ""); err != nil {
		t.Fatal(err)
	}
	if g.called.Load() != 1 {
		t.Fatal("stale call executed")
	}
}

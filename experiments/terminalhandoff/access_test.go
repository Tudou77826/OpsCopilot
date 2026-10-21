package terminalhandoff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/terminalcontrol"
)

func TestRealPTYAccess(t *testing.T) {
	if os.Getenv("OPSCOPILOT_HANDOFF_FIXTURE") == "" {
		t.Skip("isolated PTY fixture disabled")
	}
	for _, shell := range strings.Split(os.Getenv("OPSCOPILOT_HANDOFF_SHELLS"), ";") {
		t.Run(shell, func(t *testing.T) {
			service := terminalcontrol.NewService(nil)
			target := terminalcontrol.Target{SessionID: "access-lab", ConnectionID: "isolated-access"}
			c, _, server := startLabUsing(t, shell, func(stdin io.WriteCloser) (*Controller, error) {
				controller, _, err := service.AttachConnection(target.SessionID, target.ConnectionID, stdin)
				return controller, err
			})
			defer service.Detach(target.SessionID, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			setup, err := service.Take(ctx, terminalcontrol.TakeRequest{Target: target, ConfirmedOrdinaryShell: true})
			if err != nil {
				t.Fatal(err)
			}
			initial, err := setup.Inspect(terminalcontrol.ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if err := setup.Release(); err != nil {
				t.Fatal(err)
			}
			auditPath := filepath.Join(t.TempDir(), "execution.jsonl")
			journal, err := terminalcontrol.NewFileJournal(auditPath, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			access, err := terminalcontrol.NewAccess(ctx, service, terminalcontrol.AccessScope{Actor: "isolated-agent", Expires: time.Now().Add(time.Minute), Targets: []terminalcontrol.AccessTarget{{Target: target, UID: initial.LastPrompt.UID, Node: "127.0.0.1-fixture", SSHUser: "lab"}}}, journal)
			if err != nil {
				t.Fatal(err)
			}
			defer access.Cancel()
			take := func() terminalcontrol.ControlledTerminal {
				t.Helper()
				run, err := access.Operations().Take(ctx, terminalcontrol.TakeRequest{Target: target, ConfirmedOrdinaryShell: true})
				if err != nil {
					t.Fatal(err)
				}
				return run
			}
			run := take()
			result, err := run.Execute(ctx, terminalcontrol.CommandRequest{Command: "command pwd"})
			if err != nil || result.Completion != terminalcontrol.Completed || result.Context.LastPrompt.PID != initial.LastPrompt.PID {
				t.Fatal(result, err)
			}
			_, err = run.Execute(ctx, terminalcontrol.CommandRequest{Command: `command printf 'ACCESS_APPROVED=%s\n' "$$"`})
			var review *terminalcontrol.ApprovalRequired
			if !errors.As(err, &review) {
				t.Fatal(err)
			}
			for _, state := range service.States() {
				if state.Owned {
					t.Fatal("approval retained input gate")
				}
			}
			approved, err := access.Approve(review.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			result, err = approved.Execute(ctx)
			if err != nil || !strings.Contains(result.Output.Text, fmt.Sprintf("ACCESS_APPROVED=%d", initial.LastPrompt.PID)) {
				t.Fatal(result, err)
			}
			if _, err := approved.Execute(ctx); !errors.Is(err, terminalcontrol.ErrStaleApproval) {
				t.Fatal(err)
			}
			run = take()
			_, err = run.Execute(ctx, terminalcontrol.CommandRequest{Command: `command printf 'ACCESS_MUST_NOT_RUN\n'`})
			if !errors.As(err, &review) {
				t.Fatal(err)
			}
			approved, err = access.Approve(review.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			before := c.Snapshot().Sequence
			if err := c.HumanWrite([]byte("command true\n")); err != nil {
				t.Fatal(err)
			}
			if _, err := c.WaitPrompt(ctx, before); err != nil {
				t.Fatal(err)
			}
			if _, err := approved.Execute(ctx); !errors.Is(err, terminalcontrol.ErrStaleApproval) {
				t.Fatal(err)
			}
			data, err := os.ReadFile(auditPath)
			if err != nil || !strings.Contains(string(data), "explicit_approval") || !strings.Contains(string(data), "completed") || strings.Contains(string(data), "ACCESS_MUST_NOT_RUN") || strings.Contains(string(data), "ACCESS_APPROVED") {
				t.Fatal(string(data), err)
			}
			if server.channels.Load() != 1 || server.execs.Load() != 0 {
				t.Fatal("authorization changed execution context")
			}
			t.Logf("ACCESS_APPROVAL pid=%d automatic-read approved-once stale-context-rejected audit-persisted channels=1 execs=0", initial.LastPrompt.PID)
		})
	}
}

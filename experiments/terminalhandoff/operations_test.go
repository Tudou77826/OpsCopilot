package terminalhandoff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/terminalcontrol"
)

// Exercises only the application interfaces after human setup. There is one
// original interactive PTY, no independent exec command or replacement shell.
func TestRealPTYOperations(t *testing.T) {
	if os.Getenv("OPSCOPILOT_HANDOFF_FIXTURE") == "" {
		t.Skip("isolated PTY fixture disabled")
	}
	for _, shell := range strings.Split(os.Getenv("OPSCOPILOT_HANDOFF_SHELLS"), ";") {
		t.Run(shell, func(t *testing.T) {
			service := terminalcontrol.NewService(nil)
			target := terminalcontrol.Target{SessionID: "application-lab", ConnectionID: "isolated-connection"}
			c, v, server := startLabUsing(t, shell, func(stdin io.WriteCloser) (*Controller, error) {
				controller, _, err := service.AttachConnection(target.SessionID, target.ConnectionID, stdin)
				return controller, err
			})
			defer service.Detach(target.SessionID, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			before := v.size()
			if err := c.HumanWrite([]byte(`cd /tmp; export OC_API_STAGE=retained; printf 'USER_API_PID=%s\n' "$$"` + "\n")); err != nil {
				t.Fatal(err)
			}
			if err := v.wait(ctx, before, "USER_API_PID="); err != nil {
				t.Fatal(err)
			}
			// Wait for the completed output line, rather than its command echo.
			pidPattern := regexp.MustCompile(`USER_API_PID=([0-9]+)\r\n`)
			var pid int
			for pid == 0 {
				if match := pidPattern.FindStringSubmatch(v.text()[before:]); len(match) > 1 {
					pid, _ = strconv.Atoi(match[1])
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(time.Millisecond):
				}
			}
			var operations terminalcontrol.Operations = service
			view, err := operations.Inspect(target, terminalcontrol.ReadOptions{})
			if err != nil || view.Ready || view.LastPrompt != nil {
				t.Fatal(view, err)
			}
			run, err := operations.Take(ctx, terminalcontrol.TakeRequest{Target: target, ConfirmedOrdinaryShell: true})
			if err != nil {
				t.Fatal(err)
			}
			defer run.Release()
			view, err = run.Inspect(terminalcontrol.ReadOptions{})
			if err != nil || !view.Ready || view.LastPrompt.PID != pid || view.LastPrompt.Directory != "/tmp" {
				t.Fatal(view, err)
			}
			first, err := run.Execute(ctx, terminalcontrol.CommandRequest{Command: `command printf 'API_STATE=%s|%s|%s\n' "$$" "$PWD" "$OC_API_STAGE"`})
			if err != nil || first.Completion != terminalcontrol.Completed || first.ExitCode == nil || *first.ExitCode != 0 || !strings.Contains(first.Output.Text, fmt.Sprintf("API_STATE=%d|/tmp|retained", pid)) {
				t.Fatal(first, err)
			}
			failed, err := run.Execute(ctx, terminalcontrol.CommandRequest{Command: "false"})
			if err != nil || failed.Completion != terminalcontrol.Completed || failed.ExitCode == nil || *failed.ExitCode != 1 {
				t.Fatal(failed, err)
			}
			third, err := run.Execute(ctx, terminalcontrol.CommandRequest{Command: `cd /; export OC_API_STAGE=changed; command printf 'API_LOG=%s|%s\n' "$PWD" "$OC_API_STAGE"`})
			if err != nil || third.Completion != terminalcontrol.Completed || !strings.Contains(third.Output.Text, "API_LOG=/|changed") || third.Context.LastPrompt.PID != pid {
				t.Fatal(third, err)
			}
			if err := run.Release(); err != nil {
				t.Fatal(err)
			}
			before = v.size()
			if err := c.HumanWrite([]byte(`printf 'USER_AFTER_API=%s|%s|%s\n' "$$" "$PWD" "$OC_API_STAGE"` + "\n")); err != nil {
				t.Fatal(err)
			}
			if err := v.wait(ctx, before, fmt.Sprintf("USER_AFTER_API=%d|/|changed\r\n", pid)); err != nil {
				t.Fatal(err)
			}
			t.Logf("APPLICATION_MULTI_STEP pid=%d commands=3 nonzero-exit-observed manual-handoff-retained", pid)

			run, err = operations.Take(ctx, terminalcontrol.TakeRequest{Target: target, ConfirmedOrdinaryShell: true})
			if err != nil {
				t.Fatal(err)
			}
			opCtx, stop := context.WithCancel(ctx)
			defer stop()
			result := make(chan terminalcontrol.CommandResult, 1)
			returned := make(chan error, 1)
			before = v.size()
			go func() {
				value, err := run.Execute(opCtx, terminalcontrol.CommandRequest{Command: `sh -c 'trap "exit 130" INT; printf "API_SLOW_STARTED\n"; sleep 20; printf "API_MUST_NOT_FINISH\n"'`})
				result <- value
				returned <- err
			}()
			if err := v.wait(ctx, before, "API_SLOW_STARTED\r\n"); err != nil {
				t.Fatal(err)
			}
			stop()
			select {
			case err := <-returned:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			observed := <-result
			if observed.Completion != terminalcontrol.ResultUnknown || observed.Submission != terminalcontrol.Submitted || observed.ExitCode != nil {
				t.Fatal(observed)
			}
			for _, state := range service.States() {
				if state.Owned {
					t.Fatal("canceled application run retained ownership")
				}
			}
			sequence := c.Snapshot().Sequence
			if err := c.HumanWrite([]byte{3}); err != nil {
				t.Fatal(err)
			}
			if _, err := c.WaitPrompt(ctx, sequence); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(v.text()[before:], "API_MUST_NOT_FINISH\r\n") {
				t.Fatal("foreground command continued after manual interrupt")
			}
			if server.channels.Load() != 1 || server.execs.Load() != 0 {
				t.Fatalf("used replacement session: channels=%d execs=%d", server.channels.Load(), server.execs.Load())
			}
			t.Log("APPLICATION_CANCEL returned unknown result; manual Ctrl-C confirmed original PTY return; channels=1 execs=0")
		})
	}
}

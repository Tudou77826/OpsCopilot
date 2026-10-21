package terminalhandoff

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRealPTYHandoff(t *testing.T) {
	if os.Getenv("OPSCOPILOT_HANDOFF_FIXTURE") == "" {
		t.Skip("isolated real-PTY fixture not enabled")
	}
	shells := strings.Split(os.Getenv("OPSCOPILOT_HANDOFF_SHELLS"), ";")
	for _, shell := range shells {
		if shell == "" {
			t.Fatal("OPSCOPILOT_HANDOFF_SHELLS is required")
		}
		t.Run(shell, func(t *testing.T) {
			c, v, server := startLab(t, shell)
			for i := 0; i < 40; i++ {
				if err := server.client.Resize(120+i, 30); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				if t.Failed() {
					text := v.text()
					if len(text) > 16000 {
						text = text[len(text)-16000:]
					}
					t.Logf("PTY_TRANSCRIPT=%q", text)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			waitText := func(after int, text string) {
				t.Helper()
				if err := v.wait(ctx, after, text); err != nil {
					t.Fatalf("%v\noutput=%q", err, v.text())
				}
			}
			before := v.size()
			if err := c.HumanWrite([]byte("cd /tmp; export OC_HANDOFF_PROBE=retained\n")); err != nil {
				t.Fatal(err)
			}
			waitText(before, "LAB> ")
			initial, err := c.Bootstrap(ctx, true)
			if err != nil {
				t.Fatalf("bootstrap: %v; output=%q", err, v.text())
			}
			if initial.Directory != "/tmp" || initial.UID != 0 {
				t.Fatalf("initial shell state: %+v", initial)
			}
			lease, err := c.Acquire()
			if err != nil {
				t.Fatal(err)
			}
			if err := c.HumanWrite([]byte("must-not-be-typed\n")); !errors.Is(err, ErrOwned) {
				t.Fatal(err)
			}
			if _, err := c.Execute(ctx, lease, `printf 'STATE=%s|%s|%s\n' "$$" "$PWD" "$OC_HANDOFF_PROBE"`); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(v.text(), fmt.Sprintf("STATE=%d|/tmp|retained", initial.PID)) {
				t.Fatal("lost original PID/cwd/environment")
			}
			s, err := c.Execute(ctx, lease, "false")
			if err != nil || s.ExitCode != 1 {
				t.Fatalf("exit status: %+v %v", s, err)
			}
			if strings.HasSuffix(shell, "/bash") {
				s, err = c.Execute(ctx, lease, "PROMPT_COMMAND='true'; false")
				if err != nil || s.ExitCode != 1 {
					t.Fatalf("custom PROMPT_COMMAND exit status: %+v %v", s, err)
				}
				if _, err = c.Execute(ctx, lease, "unset PROMPT_COMMAND"); err != nil {
					t.Fatal(err)
				}
				t.Log("BASH_PROMPT_COMMAND existing pre-prompt command preserved failed exit status")
			}
			s, err = c.Execute(ctx, lease, "cd /; export OC_HANDOFF_PROBE=changed")
			if err != nil || s.Directory != "/" || s.PID != initial.PID {
				t.Fatalf("AI state continuity: %+v %v", s, err)
			}
			c.Revoke()
			seq := c.Snapshot().Sequence
			if err := c.HumanWrite([]byte("printf 'HUMAN=%s|%s\\n' \"$PWD\" \"$OC_HANDOFF_PROBE\"\n")); err != nil {
				t.Fatal(err)
			}
			if _, err := c.WaitPrompt(ctx, seq); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(v.text(), "HUMAN=/|changed") {
				t.Fatal("human did not inherit AI state")
			}
			t.Logf("STATE_CONTINUITY pid=%d uid=%d cwd+export preserved in both directions; false exit=1", initial.PID, initial.UID)

			// With a foreground reader, no prompt frame arrives. Acquiring or
			// unconfirmed bootstrapping must not inject anything into that reader.
			before = v.size()
			if err := c.HumanWrite([]byte("sh -c 'printf \"FOREGROUND_READY\\n\"; exec cat'\n")); err != nil {
				t.Fatal(err)
			}
			waitText(before, "FOREGROUND_READY\r\n")
			if _, err := c.Acquire(); !errors.Is(err, ErrNeedConfirmation) {
				t.Fatal(err)
			}
			if _, err := c.Bootstrap(ctx, false); !errors.Is(err, ErrNeedConfirmation) {
				t.Fatal(err)
			}
			seq = c.Snapshot().Sequence
			if err := c.HumanWrite([]byte{3}); err != nil {
				t.Fatal(err)
			}
			if _, err := c.WaitPrompt(ctx, seq); err != nil {
				t.Fatal(err)
			}
			t.Log("UNKNOWN_FOREGROUND refused AI acquisition; Ctrl-C returned to observed prompt")

			lease, err = c.Acquire()
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			before = v.size()
			go func() {
				// The marker must come from the foreground program after its
				// interrupt handler is installed. A printf in the parent shell
				// races Bash's subsequent fork/job-control setup for sleep.
				_, err := c.Execute(ctx, lease, `sh -c 'trap "exit 130" INT; printf "SLOW_STARTED\n"; sleep 20; printf "SHOULD_NOT_FINISH\n"'`)
				done <- err
			}()
			waitText(before, "SLOW_STARTED\r\n")
			c.Revoke()
			select {
			case err := <-done:
				if !errors.Is(err, ErrRevoked) {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if _, err := c.Execute(ctx, lease, "printf 'STALE_COMMAND' "); !errors.Is(err, ErrRevoked) {
				t.Fatal(err)
			}
			seq = c.Snapshot().Sequence
			if err := c.HumanWrite([]byte{3}); err != nil {
				t.Fatal(err)
			}
			s, err = c.WaitPrompt(ctx, seq)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(v.text()[before:], "SHOULD_NOT_FINISH\r\n") {
				t.Fatal("cancelled command continued past interruption")
			}
			t.Logf("STOP revoked old lease before remote completion; foreground interruption acknowledged exit=%d", s.ExitCode)

			// Non-login su can inherit the self-contained prompt protocol without
			// inheriting shell functions. Observe its actual new UID and PID.
			seq = c.Snapshot().Sequence
			suCommand := "su -s /bin/sh nobody"
			if strings.HasSuffix(shell, "/bash") {
				childCommand := "LD_LIBRARY_PATH=" + shellQuote(os.Getenv("OPSCOPILOT_HANDOFF_LIB")) + " exec " + shellQuote(shell) + " -i"
				suCommand += " -c " + shellQuote(childCommand)
			}
			if err := c.HumanWrite([]byte("cd /tmp; " + suCommand + "\n")); err != nil {
				t.Fatal(err)
			}
			child, err := c.WaitPrompt(ctx, seq)
			if err != nil {
				t.Fatalf("su bootstrap: %v; output=%q", err, v.text())
			}
			if child.UID != 65534 || child.PID == initial.PID || child.Directory != "/tmp" {
				t.Fatalf("did not inherit su shell: %+v", child)
			}
			lease, err = c.Acquire()
			if err != nil {
				t.Fatal(err)
			}
			s, err = c.Execute(ctx, lease, "cd /; export OC_HANDOFF_PROBE=child")
			if err != nil || s.UID != 65534 || s.Directory != "/" {
				t.Fatalf("su execution: %+v %v", s, err)
			}
			c.Revoke()
			seq = c.Snapshot().Sequence
			if err := c.HumanWrite([]byte("exit\n")); err != nil {
				t.Fatal(err)
			}
			parent, err := c.WaitPrompt(ctx, seq)
			if err != nil || parent.UID != 0 || parent.PID != initial.PID || parent.Directory != "/tmp" {
				t.Fatalf("parent shell recovery: %+v %v", parent, err)
			}
			t.Logf("SU_HANDOFF child_uid=%d child_pid=%d; parent_pid=%d restored", child.UID, child.PID, parent.PID)

			// Login su clears the environment. Refuse acquisition until the
			// fixture/human explicitly confirms the fresh ordinary-shell prompt.
			before = v.size()
			if err := c.HumanWrite([]byte("su -l -s /bin/sh nobody\n")); err != nil {
				t.Fatal(err)
			}
			waitText(before, "$ ")
			if _, err := c.Acquire(); !errors.Is(err, ErrNeedConfirmation) {
				t.Fatal(err)
			}
			login, err := c.Bootstrap(ctx, true)
			if err != nil || login.UID != 65534 {
				t.Fatalf("login su reattachment: %+v %v", login, err)
			}
			seq = c.Snapshot().Sequence
			if err := c.HumanWrite([]byte("exit\n")); err != nil {
				t.Fatal(err)
			}
			if _, err := c.WaitPrompt(ctx, seq); err != nil {
				t.Fatal(err)
			}
			t.Log("LOGIN_SU environment reset detected as unknown; explicit confirmation reattached in current login shell")
			before = v.size()
			if err := c.Uninstall(); err != nil {
				t.Fatal(err)
			}
			waitText(before, "LAB> ")
			if c.Snapshot().Ready {
				t.Fatal("removed instrumentation still reported readiness")
			}
			if strings.HasSuffix(shell, "/bash") {
				before = v.size()
				if err := c.HumanWrite([]byte("shopt -u promptvars\n")); err != nil {
					t.Fatal(err)
				}
				waitText(before, "LAB> ")
				probeCtx, probeCancel := context.WithTimeout(ctx, 200*time.Millisecond)
				_, probeErr := c.Bootstrap(probeCtx, true)
				probeCancel()
				if !errors.Is(probeErr, context.DeadlineExceeded) || c.Snapshot().Owned || c.Snapshot().Ready {
					t.Fatalf("unsupported promptvars config was treated as ready: %v %+v", probeErr, c.Snapshot())
				}
				seq = c.Snapshot().Sequence
				if err := c.HumanWrite([]byte("shopt -s promptvars\n")); err != nil {
					t.Fatal(err)
				}
				if _, err := c.WaitPrompt(ctx, seq); err != nil {
					t.Fatal(err)
				}
				before = v.size()
				if err := c.Uninstall(); err != nil {
					t.Fatal(err)
				}
				waitText(before, "LAB> ")
				t.Log("BASH_PROMPTVARS disabled expansion timed out without granting ownership; restored explicitly by lab")
			}
			if server.channels.Load() != 1 || server.execs.Load() != 0 {
				t.Fatalf("used a new SSH execution context: channels=%d execs=%d", server.channels.Load(), server.execs.Load())
			}
			t.Log("RESTORE original prompt restored; exactly one SSH session and zero exec channels")
		})
	}
}

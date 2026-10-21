package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func TestNativeApprovalIsOneShotBoundAuditedAndRevokedOnStop(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(map[bool]string{false: "allow", true: "stop"}[stop], func(t *testing.T) {
			c := &assistantcontrol.Coordinator{}
			turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
			defer turn.Finish("completed", "")
			auditDir := t.TempDir()
			router := &managedRouter{}
			if err := router.bind(turn, &managedFixtureTools{}, "session", nativeAudit(auditDir, turn.ID())); err != nil {
				t.Fatal(err)
			}
			defer router.clear()
			finished := make(chan error, 1)
			go func() {
				result, err := router.Request(context.Background(), "session/request_permission", json.RawMessage(`{"sessionId":"session","toolCall":{"title":"Local Python","rawInput":{"command":"PRIVATE_TEST_MARKER"}},"options":[{"optionId":"once","kind":"allow_once"},{"optionId":"deny","kind":"reject_once"}]}`))
				if err == nil {
					data, _ := json.Marshal(result)
					if !strings.Contains(string(data), `"optionId":"once"`) {
						err = ErrClosed
					}
				}
				finished <- err
			}()
			deadline := time.Now().Add(time.Second)
			for c.Snapshot().Prompt == nil && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			p := c.Snapshot().Prompt
			if p == nil {
				t.Fatal("permission was silently denied or skipped")
			}
			if stop {
				_ = c.Stop(turn.ID())
				if err := c.Respond(turn.ID(), p.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); err == nil {
					t.Fatal("late approval accepted")
				}
			} else {
				if err := c.Respond(turn.ID(), p.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-finished:
				if !stop && err != nil {
					t.Fatal(err)
				}
				if stop && err == nil {
					t.Fatal("stopped permission granted")
				}
			case <-time.After(time.Second):
				t.Fatal("permission remained pending")
			}
			log, err := os.ReadFile(filepath.Join(auditDir, "native.jsonl"))
			if err != nil || strings.Contains(string(log), "PRIVATE_TEST_MARKER") {
				t.Fatal("audit missing or contained raw secrets", err)
			}
		})
	}
}
func TestNativeAuditFailureCannotApprove(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	file := filepath.Join(t.TempDir(), "not-directory")
	os.WriteFile(file, []byte("blocked"), 0600)
	b := &Interactions{Turn: turn, SessionID: "session", Audit: nativeAudit(file, turn.ID())}
	if _, err := b.Request(context.Background(), "session/request_permission", json.RawMessage(`{"sessionId":"session","options":[{"optionId":"a","kind":"allow_once"},{"optionId":"r","kind":"reject_once"}]}`)); err == nil {
		t.Fatal("approval continued without audit")
	}
	if turn.Snapshot().Prompt != nil {
		t.Fatal("approval offered despite audit failure")
	}
}

package shellsidecar

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/script"
)

func TestPrepareReplayValidatesAndSnapshots(t *testing.T) {
	sc := &script.Script{Variables: []script.ScriptVariable{{Name: "TARGET", Required: true}}, Steps: []script.ScriptStep{{Command: "echo ${TARGET}", Enabled: true}}}
	if _, _, err := prepareReplay(sc, nil); err == nil {
		t.Fatal("required variable accepted")
	}
	if _, _, err := prepareReplay(sc, map[string]string{"TARGET": "ok", "unknown": "bad"}); err == nil {
		t.Fatal("unknown variable accepted")
	}
	commands, _, err := prepareReplay(sc, map[string]string{"TARGET": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	sc.Steps[0].Command = "changed"
	if commands[0] != "echo ok\n" {
		t.Fatalf("wrong command or double newline: %q", commands[0])
	}
	sc.Steps[0].Delay = -1
	if _, _, err := prepareReplay(sc, map[string]string{"TARGET": "ok"}); err == nil {
		t.Fatal("negative delay accepted")
	}
	sc.Steps[0].Delay = 0
	sc.Steps[0].Command = strings.Repeat("x", 65536)
	if _, _, err := prepareReplay(sc, map[string]string{"TARGET": "ok"}); err == nil {
		t.Fatal("oversized command accepted")
	}
}

func TestManagedReplayStopDuringDelayNeverSendsLaterSteps(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	job := &replayJob{status: ReplayState{State: "running", Total: 2}, cancel: cancel, done: make(chan struct{})}
	first := make(chan struct{})
	calls := 0
	go runReplay(ctx, job, []string{"first\n", "never\n"}, []time.Duration{0, time.Minute}, func(_ string, _ []byte) error { calls++; close(first); return nil })
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("first command not sent")
	}
	cancel()
	select {
	case <-job.done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt delay")
	}
	state := job.snapshot()
	if state.State != "stopped" || state.Sent != 1 || calls != 1 {
		t.Fatalf("bad stop result: %+v", state)
	}
}

func TestManagedReplayStopWaitsForInflightWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	job := &replayJob{status: ReplayState{ID: "run", State: "running", Total: 2}, cancel: cancel, done: make(chan struct{})}
	s := &StructuredScriptService{jobs: map[string]*replayJob{"run": job}}
	entered, release := make(chan struct{}), make(chan struct{})
	go runReplay(ctx, job, []string{"first\n", "never\n"}, []time.Duration{0, 0}, func(_ string, _ []byte) error { close(entered); <-release; return nil })
	<-entered
	deadline, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if _, err := s.StopReplay(deadline, "run"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stop falsely acknowledged: %v", err)
	}
	close(release)
	state, err := s.StopReplay(context.Background(), "run")
	if err != nil || state.State != "stopped" || state.Sent != 1 {
		t.Fatalf("bad stop state %+v %v", state, err)
	}
}

func TestManagedReplayReportsDispatchNotRemoteSuccess(t *testing.T) {
	for _, fail := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		job := &replayJob{status: ReplayState{State: "running", Total: 1}, cancel: cancel, done: make(chan struct{})}
		go runReplay(ctx, job, []string{"command\n"}, []time.Duration{0}, func(_ string, _ []byte) error {
			if fail {
				return errors.New("sensitive detail")
			}
			return nil
		})
		<-job.done
		state := job.snapshot()
		if fail && (state.State != "failed" || state.Sent != 0) {
			t.Fatal(state)
		}
		if !fail && (state.State != "dispatched" || state.Sent != 1) {
			t.Fatal(state)
		}
	}
}

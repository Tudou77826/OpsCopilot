package assistanttools

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/filecontrol"
	"opscopilot/pkg/filetransfer"
)

type fileBackendFixture struct{ transfers atomic.Int32 }

func (b *fileBackendFixture) Check(ctx context.Context, _ filecontrol.Endpoint) error {
	return ctx.Err()
}
func (b *fileBackendFixture) List(ctx context.Context, _ filecontrol.Endpoint, _ string, _ bool) ([]filetransfer.Entry, error) {
	return []filetransfer.Entry{}, ctx.Err()
}
func (b *fileBackendFixture) Transfer(ctx context.Context, _ filecontrol.Endpoint, _ filecontrol.Task, _ func(filetransfer.Progress)) (filetransfer.TransferResult, error) {
	b.transfers.Add(1)
	return filetransfer.TransferResult{Bytes: 7}, ctx.Err()
}

type fileJournalFixture struct{}

func (fileJournalFixture) Append(ctx context.Context, _ filecontrol.Event) error { return ctx.Err() }
func TestFileTransferApprovalCannotExecuteBeforeConfirmationOrAfterStop(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	b := &fileBackendFixture{}
	service := filecontrol.New(b, fileJournalFixture{})
	w, err := service.Open(context.Background(), filecontrol.Endpoint{SessionID: "session", ConnectionID: "actual", Identity: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Take(turn.Context(), w.ID, turn.ID()); err != nil {
		t.Fatal(err)
	}
	f := &FileTools{Turn: turn, Service: service, current: w.ID, Review: lowRiskFixture}
	defer f.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for ctx.Err() == nil {
			v, e := service.Snapshot(w.ID)
			if e == nil && v.Pending != "" {
				_ = service.Acknowledge(w.ID, v.Pending, v.Endpoint.ConnectionID)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	local := filepath.Join(t.TempDir(), "package.zip")
	args := `{"direction":"upload","local_path":` + quoteJSON(local) + `,"remote_path":"/package.zip","purpose":"测试传输"}`
	done := make(chan error, 1)
	go func() { _, err := f.Call(turn.Context(), "file_transfer", args); done <- err }()
	var state assistantcontrol.State
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		state = c.Snapshot()
		if state.Prompt != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if state.Prompt == nil || state.Prompt.Title != "确认这一次文件传输" {
		t.Fatal("missing exact approval")
	}
	if b.transfers.Load() != 0 {
		t.Fatal("premature transfer")
	}
	if v, _ := service.Snapshot(w.ID); v.OwnerTurn != turn.ID() {
		t.Fatal("approval released ownership")
	}
	if err = c.Stop(turn.ID()); err != nil {
		t.Fatal(err)
	}
	if err = c.Respond(turn.ID(), state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal("late approval accepted", err)
	}
	if err = <-done; err == nil {
		t.Fatal("stopped call succeeded")
	}
	f.Close()
	if b.transfers.Load() != 0 {
		t.Fatal("transfer dispatched after stop")
	}
}
func quoteJSON(s string) string { data, _ := json.Marshal(s); return string(data) }

func TestFileTransferRefusalCreatesCompactDeniedReceipt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	backend := &fileBackendFixture{}
	service := filecontrol.New(backend, fileJournalFixture{})
	w, err := service.Open(ctx, filecontrol.Endpoint{SessionID: "session", ConnectionID: "actual", Identity: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Take(turn.Context(), w.ID, turn.ID()); err != nil {
		t.Fatal(err)
	}
	f := &FileTools{Turn: turn, Service: service, current: w.ID, Review: lowRiskFixture}
	defer f.Close()
	go func() {
		for ctx.Err() == nil {
			if snapshot, e := service.Snapshot(w.ID); e == nil && snapshot.Pending != "" {
				_ = service.Acknowledge(w.ID, snapshot.Pending, snapshot.Endpoint.ConnectionID)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	args := `{"direction":"upload","local_path":` + quoteJSON(filepath.Join(t.TempDir(), "package.zip")) + `,"remote_path":"/package.zip","purpose":"测试传输"}`
	done := make(chan error, 1)
	go func() { _, err := f.Call(ctx, "file_transfer", args); done <- err }()
	prompt := waitPrompt(t, c)
	if err = c.Respond(turn.ID(), prompt.Prompt.ID, assistantcontrol.Answer{Selected: []string{"reject"}}); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, assistantcontrol.ErrActionDenied) || backend.transfers.Load() != 0 {
		t.Fatal("refused upload ran", err)
	}
	var receipt assistantcontrol.Message
	for _, message := range c.Snapshot().Messages {
		if message.Kind == "operation" {
			receipt = message
		}
	}
	if receipt.Content != "文件传输" || receipt.Operation.Status != "denied" || receipt.Operation.Detail != "" || receipt.Operation.Next != "" {
		t.Fatal("refusal was presented as a failure", receipt)
	}
}

func TestSensitiveFilePathsTriggerHostFloor(t *testing.T) {
	for _, path := range []string{"/root/.ssh/id_rsa", `C:\Users\me\.ssh\id_rsa`} {
		if !sensitiveFilePath(path) {
			t.Fatalf("sensitive path missed: %s", path)
		}
	}
	if sensitiveFilePath("/tmp/service.log") {
		t.Fatal("ordinary log was flagged")
	}
}

package assistantcontrol

// Operation is a host-observed display receipt. Identifiers only locate existing
// UI resources; they never grant access, reconnect or replay an action.
// Scalar fields keep history snapshots independent without nested aliases.
type Operation struct {
	Measured     bool   `json:"measured,omitempty"`
	DurationMs   int64  `json:"durationMs,omitempty"`
	Kind         string `json:"kind"`
	WorkspaceID  string `json:"workspaceId"`
	ItemID       string `json:"itemId,omitempty"`
	ConnectionID string `json:"connectionId,omitempty"`
	Status       string `json:"status"`
	Stage        string `json:"stage,omitempty"`
	Outcome      string `json:"outcome,omitempty"`
	Next         string `json:"next,omitempty"`
	Detail       string `json:"detail,omitempty"`
	Action       string `json:"action,omitempty"`
	Result       string `json:"result,omitempty"`
	Location     string `json:"location,omitempty"`
}

// ObserveOperation updates one receipt as an asynchronous task settles. A stopped
// turn cannot publish new activity; its last receipt remains explicitly pending.
func (t *Turn) ObserveOperation(key, title string, operation Operation) {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if t.owner.current != t || !active(t.state.Phase) || t.ctx.Err() != nil {
		return
	}
	if len(key) > 200 || len(title) > 2048 {
		return
	}
	title = DisplayExcerpt(title, 2048)
	operation.Location = DisplayExcerpt(operation.Location, 1024)
	operation.Action = DisplayExcerpt(operation.Action, 4096)
	operation.Result = DisplayExcerpt(operation.Result, 6144)
	operation.Detail = DisplayExcerpt(operation.Detail, 8192)
	id := t.ID() + ":operation:" + key
	// Keep receipts inside a separate per-turn display budget as task status
	// changes. A flood of diagnostics must not make the archive unsaveable.
	used := 0
	existing := false
	for _, m := range t.state.Messages {
		if m.Kind == "operation" && m.ID != id {
			used += messageBytes(m)
		}
		if m.ID == id {
			existing = true
		}
	}
	if used+messageBytes(Message{Content: title, Operation: operation}) > 128*1024 {
		operation.Detail = "[详情超出本轮记录上限，请查看原页签]"
		operation.Result = DisplayExcerpt(operation.Result, 512)
		// Never lose an observed final status merely because its output grew.
		// Existing receipts may exceed the soft budget by their bounded metadata.
		if !existing && used+messageBytes(Message{Content: title, Operation: operation}) > 128*1024 {
			return
		}
	}
	for i := range t.state.Messages {
		if t.state.Messages[i].ID == id {
			t.state.Messages[i].Content = title
			t.state.Messages[i].Operation = operation
			t.operationRevision++
			return
		}
	}
	if len(t.state.Messages) >= 512 {
		return
	}
	t.record("ai", "operation", title)
	if len(t.state.Messages) > 0 {
		last := &t.state.Messages[len(t.state.Messages)-1]
		// record may refuse a message when the per-turn limit is reached.
		if last.Kind == "operation" && last.Operation.Kind == "" {
			last.ID, last.Operation = id, operation
			t.operationRevision++
		}
	}
}

// OperationRevision allows the serialized tool boundary to provide a fallback
// when a detailed tool fails before it can create its normal resource receipt.
func (t *Turn) OperationRevision() uint64 {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	return t.operationRevision
}

func messageBytes(m Message) int {
	op := m.Operation
	referenceBytes := 0
	for _, r := range m.References {
		referenceBytes += len(r.Kind) + len(r.TabID) + len(r.ConnectionID) + len(r.Title)
	}
	return referenceBytes + len(m.Content) + len(op.Kind) + len(op.WorkspaceID) + len(op.ItemID) + len(op.ConnectionID) + len(op.Status) + len(op.Stage) + len(op.Outcome) + len(op.Next) + len(op.Detail) + len(op.Action) + len(op.Result) + len(op.Location)
}

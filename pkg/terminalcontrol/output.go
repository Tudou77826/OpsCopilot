package terminalcontrol

import (
	"errors"
	"strings"
)

const OutputCapacity = 64 << 10
const DefaultOutputLimit = 16 << 10

// Observation is a bounded window of visible PTY bytes, not command stdout.
// Echo, prompts and unrelated background output may be present. Cursors count
// original bytes; invalid/clipped UTF-8 is replaced in Text for JSON consumers.
type Observation struct {
	From      uint64 `json:"from"`
	To        uint64 `json:"to"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

type outputWindow struct {
	data  []byte
	total uint64
}

// Freeze at completion/revocation, not when a delayed transport call returns.
// Otherwise an old operation could collect the user's post-takeover output.
type commandObservation struct {
	after  uint64
	limit  int
	frozen bool
	state  Snapshot
	output Observation
}

func (c *Controller) freezeObservation() {
	if capture := c.capture; capture != nil && !capture.frozen {
		capture.state, capture.output, capture.frozen = c.state, c.output.read(capture.after, capture.limit), true
	}
}

func (w *outputWindow) append(data []byte) {
	if len(data) == 0 {
		return
	}
	if w.data == nil {
		w.data = make([]byte, OutputCapacity)
	}
	end := w.total + uint64(len(data))
	if len(data) > OutputCapacity {
		data = data[len(data)-OutputCapacity:]
	}
	start := int((end - uint64(len(data))) % OutputCapacity)
	n := copy(w.data[start:], data)
	copy(w.data, data[n:])
	w.total = end
}

func validateRead(after uint64, limit int, total uint64) error {
	if limit < 0 || limit > OutputCapacity {
		return errors.New("终端输出读取上限必须在 0–65536 字节之间")
	}
	if after > total {
		return errors.New("终端输出游标超出当前连接范围")
	}
	return nil
}

func (w *outputWindow) read(after uint64, limit int) Observation {
	if limit == 0 {
		limit = DefaultOutputLimit
	}
	start := after
	if w.total > uint64(limit) && start < w.total-uint64(limit) {
		start = w.total - uint64(limit)
	}
	result := Observation{From: start, To: w.total, Truncated: start > after}
	if start == w.total {
		return result
	}
	out := make([]byte, int(w.total-start))
	index := int(start % OutputCapacity)
	n := copy(out, w.data[index:])
	copy(out[n:], w.data)
	result.Text = strings.ToValidUTF8(string(out), "�")
	return result
}

// Observe is read-only and returns context and output from one controller lock.
// A cursor is scoped to this connection; callers must also bind ConnectionID.
func (c *Controller) Observe(after uint64, limit int) (Snapshot, Observation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := validateRead(after, limit, c.output.total); err != nil {
		return Snapshot{}, Observation{}, err
	}
	return c.state, c.output.read(after, limit), nil
}

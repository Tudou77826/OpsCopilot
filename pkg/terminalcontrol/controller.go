package terminalcontrol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNeedConfirmation = errors.New("终端状态未知，需要确认普通命令行")
	ErrOwned            = errors.New("页签已被 AI 占用")
	ErrRevoked          = errors.New("本轮操作权已失效")
	ErrBusy             = errors.New("上一条命令尚未完成")
)

type Lease struct {
	controller *Controller
	generation uint64
}

type Snapshot struct {
	Prompt
	Sequence uint64
	Ready    bool
	Owned    bool
	Closed   bool
}

// Controller gates one existing Shell. Service coordinates runs; authorization
// must be added at the application boundary before external exposure. This type
// never opens a new SSH execution context.
type Controller struct {
	mu               sync.Mutex
	dispatch         sync.Mutex
	writer           io.Writer
	protocol         *protocol
	state            Snapshot
	generation       uint64
	pending          bool
	request          uint64
	started          bool
	reserved         bool
	inputDirty       bool
	bootstrapPending bool
	changed          chan struct{}
	output           outputWindow
	capture          *commandObservation
}

func New(writer io.Writer) (*Controller, error) {
	if writer == nil {
		return nil, errors.New("终端输入流不可用")
	}
	p, err := newProtocol()
	if err != nil {
		return nil, err
	}
	return &Controller{writer: writer, protocol: p, changed: make(chan struct{})}, nil
}

func (c *Controller) signal() { close(c.changed); c.changed = make(chan struct{}) }

func (c *Controller) Snapshot() Snapshot { c.mu.Lock(); defer c.mu.Unlock(); return c.state }

func (c *Controller) Feed(data []byte) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	visible, prompts := c.protocol.feed(data)
	c.output.append(visible)
	if !c.state.Closed {
		for _, p := range prompts {
			if p.Request != 0 {
				if c.pending && p.Request == c.request {
					c.started = true
				}
				continue
			}
			// A prompt buffered before this command's begin marker is not its
			// completion. The marker runs in the same shell, before the command.
			if c.pending && !c.started {
				continue
			}
			c.state.Prompt = p
			c.state.Sequence++
			c.state.Ready = !c.inputDirty
			c.signal()
		}
	}
	return visible
}

func (c *Controller) Closed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state.Closed = true
	c.state.Ready = false
	c.state.Owned = false
	c.reserved = false
	c.generation++
	c.freezeObservation()
	c.signal()
}

func (c *Controller) WaitPrompt(ctx context.Context, after uint64) (Snapshot, error) {
	for {
		c.mu.Lock()
		s, changed := c.state, c.changed
		c.mu.Unlock()
		if s.Closed {
			return s, io.EOF
		}
		if s.Ready && s.Sequence > after {
			return s, nil
		}
		select {
		case <-ctx.Done():
			return s, ctx.Err()
		case <-changed:
		}
	}
}

func (c *Controller) Bootstrap(ctx context.Context, confirmedOrdinaryShell bool) (Snapshot, error) {
	if !confirmedOrdinaryShell {
		return c.Snapshot(), ErrNeedConfirmation
	}
	c.dispatch.Lock()
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		c.dispatch.Unlock()
		return c.Snapshot(), err
	}
	if c.state.Closed || c.state.Owned || c.inputDirty || c.bootstrapPending {
		c.mu.Unlock()
		c.dispatch.Unlock()
		return c.Snapshot(), ErrNeedConfirmation
	}
	before := c.state.Sequence
	c.bootstrapPending = true
	c.state.Ready = false
	c.signal()
	c.mu.Unlock()
	err := c.write([]byte(c.protocol.bootstrap() + "\n"))
	c.dispatch.Unlock()
	defer func() { c.mu.Lock(); c.bootstrapPending = false; c.signal(); c.mu.Unlock() }()
	if err != nil {
		return c.Snapshot(), err
	}
	return c.WaitPrompt(ctx, before)
}

func (c *Controller) Uninstall() error { return c.HumanWrite([]byte(c.protocol.uninstall() + "\n")) }

func (c *Controller) Acquire() (Lease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Closed {
		return Lease{}, io.EOF
	}
	if c.state.Owned {
		return Lease{}, ErrOwned
	}
	if !c.state.Ready || c.pending || c.bootstrapPending {
		return Lease{}, ErrNeedConfirmation
	}
	c.generation++
	c.reserved = false
	c.state.Owned = true
	c.signal()
	return Lease{controller: c, generation: c.generation}, nil
}

// Revoke is immediate, including when a previous transport write is blocked.
// Writes already committed to the transport before this boundary cannot be
// recalled. Pending/new commands must recheck their lease before commitment.
func (c *Controller) Revoke() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.state.Owned = false
	c.reserved = false
	c.freezeObservation()
	c.signal()
}

// HumanWrite includes Ctrl-C after revocation. Remote process termination is a
// separate fact, acknowledged only when a new prompt is actually observed.
func (c *Controller) HumanWrite(data []byte) error {
	c.dispatch.Lock()
	defer c.dispatch.Unlock()
	c.mu.Lock()
	if c.state.Closed {
		c.mu.Unlock()
		return io.EOF
	}
	reply := isStatusReply(data)
	if (c.state.Owned || c.reserved) && !reply {
		c.mu.Unlock()
		return ErrOwned
	}
	if c.bootstrapPending && !reply {
		c.mu.Unlock()
		return ErrBusy
	}
	if reply {
		c.mu.Unlock()
		return c.write(data)
	}
	// A late prompt must not make an unfinished human input line executable.
	// Full terminal editing semantics are outside this experiment: uncertain
	// edits conservatively remain dirty until Enter or Ctrl-C clears them.
	for _, b := range data {
		switch b {
		case '\r', '\n', 3:
			c.inputDirty = false
		default:
			c.inputDirty = true
		}
	}
	c.state.Ready = false
	c.signal()
	c.mu.Unlock()
	return c.write(data)
}

// Reserve closes the human-input gate before asynchronous preparation starts.
// It never writes to an unknown terminal; Bootstrap still requires confirmation.
func (c *Controller) Reserve() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Closed {
		return io.EOF
	}
	if c.reserved || c.state.Owned || c.pending || c.bootstrapPending {
		return ErrBusy
	}
	if c.inputDirty {
		return errors.New("请先提交或取消终端中未完成的输入，再开始接手测试")
	}
	c.reserved = true
	return nil
}

func (c *Controller) write(data []byte) error {
	n, err := c.writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		c.Revoke()
		return fmt.Errorf("终端输入未完整发送，需核对现场: %w", err)
	}
	return nil
}

type Submission string

const (
	NotSubmitted      Submission = "not_submitted"
	SubmissionUnknown Submission = "unknown"
	Submitted         Submission = "submitted"
)

type Execution struct {
	State      Snapshot
	Submission Submission
	Completed  bool
	Output     Observation
}

func validateCommand(command string) error {
	if strings.TrimSpace(command) == "" || len(command) > 2048 || !utf8.ValidString(command) {
		return errors.New("只接受不超过 2048 字节的 UTF-8 单行命令")
	}
	for _, r := range command {
		if unicode.IsControl(r) && r != '\t' {
			return errors.New("命令不能包含换行或终端控制字符")
		}
	}
	return nil
}

func (c *Controller) Execute(ctx context.Context, lease Lease, command string) (Snapshot, error) {
	result, err := c.ExecuteObserved(ctx, lease, command, DefaultOutputLimit)
	return result.State, err
}

// ExecuteObserved preserves the existing shell execution contract and adds
// delivery evidence. Attempting a write is never reported as remote completion.
func (c *Controller) ExecuteObserved(ctx context.Context, lease Lease, command string, outputLimit int) (result Execution, err error) {
	return c.executeChecked(ctx, lease, command, outputLimit, nil)
}

// check is an in-memory authority check under the same lock as commitment.
// It must not perform I/O or acquire a service lock.
func (c *Controller) executeChecked(ctx context.Context, lease Lease, command string, outputLimit int, check func(Snapshot) error) (result Execution, err error) {
	result.Submission = NotSubmitted
	var capture *commandObservation
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if capture == nil {
			result.State = c.state
			return
		}
		if c.capture == capture {
			c.freezeObservation()
			c.capture = nil
		}
		result.State, result.Output = capture.state, capture.output
	}()
	if err := validateCommand(command); err != nil {
		return result, err
	}
	if err := validateRead(0, outputLimit, 0); err != nil {
		return result, err
	}
	c.dispatch.Lock()
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		c.dispatch.Unlock()
		return result, err
	}
	if c.state.Closed {
		c.mu.Unlock()
		c.dispatch.Unlock()
		return result, io.EOF
	}
	if lease.controller != c || !c.state.Owned || c.generation != lease.generation {
		c.mu.Unlock()
		c.dispatch.Unlock()
		return result, ErrRevoked
	}
	if c.pending || !c.state.Ready {
		c.mu.Unlock()
		c.dispatch.Unlock()
		return result, ErrBusy
	}
	if check != nil {
		if err := check(c.state); err != nil {
			c.mu.Unlock()
			c.dispatch.Unlock()
			return result, err
		}
	}
	before := c.state.Sequence
	capture = &commandObservation{after: c.output.total, limit: outputLimit}
	c.capture = capture
	c.pending = true
	c.request++
	c.started = false
	request := c.request
	c.state.Ready = false
	c.signal()
	c.mu.Unlock()
	result.Submission = SubmissionUnknown
	err = c.write([]byte(c.protocol.begin(request) + command + "\n"))
	c.dispatch.Unlock()
	defer func() { c.mu.Lock(); c.pending = false; c.signal(); c.mu.Unlock() }()
	if err != nil {
		return result, err
	}
	result.Submission = Submitted
	for {
		c.mu.Lock()
		s, changed := c.state, c.changed
		valid := lease.controller == c && c.state.Owned && c.generation == lease.generation
		if valid && !s.Closed && ctx.Err() == nil && s.Ready && s.Sequence > before {
			result.Completed = true
			c.freezeObservation()
			c.mu.Unlock()
			return result, nil
		}
		c.mu.Unlock()
		if s.Closed {
			return result, io.EOF
		}
		if !valid {
			return result, ErrRevoked
		}
		if err := ctx.Err(); err != nil {
			c.Revoke()
			return result, err
		}
		select {
		case <-ctx.Done():
			c.Revoke()
			return result, ctx.Err()
		case <-changed:
		}
	}
}

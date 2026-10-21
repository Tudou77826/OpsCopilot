package terminalcontrol

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/google/uuid"
)

// State is a snapshot, never a capability the caller can use to execute SQL or
// arbitrary shell code. Run IDs also fence late stop requests from the UI.
type State struct {
	SessionID    string `json:"sessionId"`
	ConnectionID string `json:"connectionId"`
	RunID        string `json:"runId"`
	Phase        string `json:"phase"`
	Owned        bool   `json:"owned"`
	Ready        bool   `json:"ready"`
	Message      string `json:"message"`
	ExitCode     *int   `json:"exitCode,omitempty"`
}

type Audit struct {
	SessionID string
	RunID     string
	Action    string
	Phase     string
}

type endpoint struct {
	*Controller
	raw io.WriteCloser
}

func (e *endpoint) Write(p []byte) (int, error) {
	if err := e.HumanWrite(p); err != nil {
		return 0, err
	}
	return len(p), nil
}
func (e *endpoint) Close() error { e.Closed(); return e.raw.Close() }

type entry struct {
	controller *Controller
	state      State
	cancel     context.CancelFunc
	run        *Run
}

// Service coordinates one controlled run per desktop process. Operations is its
// trusted in-process application interface. The desktop test adapter below still
// accepts only two fixed scenarios; no arbitrary-command transport is exposed.
type Service struct {
	mu      sync.Mutex
	entries map[string]*entry
	active  string
	audit   func(Audit)
}

// NewService accepts a concurrent lifecycle sink. Callbacks receive immutable
// events outside service locks and may read state or request cancellation.
func NewService(audit func(Audit)) *Service {
	if audit == nil {
		audit = func(Audit) {}
	}
	return &Service{entries: make(map[string]*entry), audit: audit}
}

func (s *Service) Attach(id string, raw io.WriteCloser) (*Controller, io.WriteCloser, error) {
	return s.AttachConnection(id, uuid.NewString(), raw)
}

func (s *Service) AttachConnection(id, connectionID string, raw io.WriteCloser) (*Controller, io.WriteCloser, error) {
	if connectionID == "" {
		return nil, nil, errors.New("连接编号不能为空")
	}
	c, err := New(raw)
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous := s.entries[id]; previous != nil {
		previous.controller.Closed()
		if previous.cancel != nil {
			previous.cancel()
		}
		if s.active == previous.state.RunID {
			s.active = ""
		}
	}
	s.entries[id] = &entry{controller: c, state: State{SessionID: id, ConnectionID: connectionID, Phase: "idle", Message: "沿用当前终端，验证接手与交还"}}
	return c, &endpoint{Controller: c, raw: raw}, nil
}

func (s *Service) Detach(id string, expected *Controller) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[id]
	if e == nil || (expected != nil && e.controller != expected) {
		return
	}
	e.controller.Closed()
	if e.cancel != nil {
		e.cancel()
	}
	if s.active == e.state.RunID {
		s.active = ""
	}
	delete(s.entries, id)
}

func (s *Service) States() []State {
	s.mu.Lock()
	defer s.mu.Unlock()
	states := make([]State, 0, len(s.entries))
	for _, e := range s.entries {
		state := cloneState(e.state)
		state.Ready = e.controller.Snapshot().Ready
		if state.Phase == "stopped" {
			if state.Ready {
				state.Message = "已交还终端，可以继续操作"
			} else {
				state.Message = "已交还终端；远端命令可能仍在运行，可按 Ctrl+C 中断"
			}
		}
		states = append(states, state)
	}
	return states
}

func (s *Service) Start(ctx context.Context, id, scenario string, confirmed bool) (State, error) {
	return s.start(ctx, id, "", scenario, confirmed)
}

// StartForConnection binds the user's idle-shell confirmation to the actual
// endpoint observed by the UI. Reconnect must require fresh confirmation.
func (s *Service) StartForConnection(ctx context.Context, id, connectionID, scenario string, confirmed bool) (State, error) {
	if connectionID == "" {
		return State{}, ErrNeedConfirmation
	}
	return s.start(ctx, id, connectionID, scenario, confirmed)
}

func (s *Service) start(ctx context.Context, id, connectionID, scenario string, confirmed bool) (State, error) {
	command := ""
	switch scenario {
	case "inspect":
		command = `command printf '\n[接手测试] 当前 Shell PID=%s\n' "$$"; command id; command pwd`
	case "slow":
		command = `command printf '\n[接手测试] 慢命令已开始，等待 12 秒，可停止并接回终端\n'; command sleep 12; command printf '[接手测试] 慢命令已返回\n'`
	default:
		return State{}, errors.New("不支持的接手测试，只能选择查看现场或慢命令")
	}
	// Internal fixture convenience only. The Wails entry always supplies the
	// connection actually confirmed by its UI.
	if connectionID == "" {
		s.mu.Lock()
		if e := s.entries[id]; e != nil {
			connectionID = e.state.ConnectionID
		}
		s.mu.Unlock()
	}
	run, state, err := s.beginRun(ctx, TakeRequest{Target: Target{SessionID: id, ConnectionID: connectionID}, ConfirmedOrdinaryShell: confirmed})
	if err != nil {
		return State{}, err
	}
	go s.runScenario(run, scenario, command)
	return state, nil
}

func (s *Service) runScenario(run *Run, scenario, command string) {
	s.audit(Audit{SessionID: run.target.SessionID, RunID: run.id, Action: scenario, Phase: "accepted"})
	result, err := run.Execute(run.ctx, CommandRequest{Command: command})
	if err != nil {
		run.fail(err)
		return
	}
	_ = run.finish("completed", "测试完成，已交还终端", result.ExitCode)
}

func (s *Service) Stop(id, runID string) error {
	s.mu.Lock()
	e := s.entries[id]
	if e == nil || e.state.RunID != runID {
		s.mu.Unlock()
		return errors.New("该轮接手已结束，请刷新终端状态")
	}
	if !e.state.Owned {
		run := e.run
		s.mu.Unlock()
		if run != nil && run.access != nil {
			run.access.cancelReviews(run.id)
		}
		return nil
	}
	run := e.run
	s.mu.Unlock()
	if run.access != nil {
		run.access.cancelReviews(run.id)
	}
	return run.finish("stopped", "已停止发令并交还终端", nil)
}

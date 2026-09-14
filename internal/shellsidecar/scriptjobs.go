package shellsidecar

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"opscopilot/pkg/script"
)

// ReplayState describes dispatch to a PTY, never remote command success.
// Commands and variable values are deliberately absent from status/events.
type ReplayState struct {
	ID         string `json:"id"`
	ScriptID   string `json:"scriptId"`
	TerminalID string `json:"terminalId"`
	State      string `json:"state"`
	Sent       int    `json:"sent"`
	Total      int    `json:"total"`
}

type replayJob struct {
	mu     sync.Mutex
	status ReplayState
	cancel context.CancelFunc
	done   chan struct{}
}

var replayID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func replayActive(state string) bool { return state == "running" || state == "stopping" }

func (j *replayJob) snapshot() ReplayState {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

// StartReplay snapshots the script before starting a bounded, cancellable worker.
// The trusted host must acquire the terminal writer lease before calling this API.
func (s *StructuredScriptService) StartReplay(runID, scriptID, terminalID string, values map[string]string) (ReplayState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ReplayState{}, fmt.Errorf("script service closed")
	}
	if !replayID.MatchString(runID) || !replayID.MatchString(scriptID) {
		return ReplayState{}, fmt.Errorf("invalid replay identity")
	}
	if _, exists := s.jobs[runID]; exists {
		return ReplayState{}, fmt.Errorf("replay identity already used")
	}
	if s.RecordingStatus().IsRecording {
		return ReplayState{}, fmt.Errorf("recording is active")
	}
	for _, job := range s.jobs {
		state := job.snapshot()
		if state.TerminalID == terminalID && replayActive(state.State) {
			return ReplayState{}, fmt.Errorf("terminal is replaying")
		}
	}
	if _, _, err := s.svc.SessionInfo(terminalID); err != nil {
		return ReplayState{}, err
	}
	sc, err := s.mgr.LoadScript(scriptID)
	if err != nil {
		return ReplayState{}, err
	}
	commands, delays, err := prepareReplay(sc, values)
	if err != nil {
		return ReplayState{}, err
	}
	if len(s.jobs) >= 128 {
		for id, job := range s.jobs {
			if !replayActive(job.snapshot().State) {
				delete(s.jobs, id)
				break
			}
		}
		if len(s.jobs) >= 128 {
			return ReplayState{}, fmt.Errorf("too many replays")
		}
	}
	if s.jobs == nil {
		s.jobs = make(map[string]*replayJob)
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &replayJob{status: ReplayState{ID: runID, ScriptID: scriptID, TerminalID: terminalID, State: "running", Total: len(commands)}, cancel: cancel, done: make(chan struct{})}
	s.jobs[runID] = job
	initial := job.snapshot()
	go runReplay(ctx, job, commands, delays, s.svc.WriteInput)
	return initial, nil
}

func prepareReplay(sc *script.Script, values map[string]string) ([]string, []time.Duration, error) {
	if len(sc.Variables) > 128 {
		return nil, nil, fmt.Errorf("too many variables")
	}
	merged := map[string]string{}
	for _, v := range sc.Variables {
		if !variableName.MatchString(v.Name) {
			return nil, nil, fmt.Errorf("invalid variable name")
		}
		if _, exists := merged[v.Name]; exists {
			return nil, nil, fmt.Errorf("duplicate variable")
		}
		value := v.DefaultValue
		if supplied, ok := values[v.Name]; ok {
			value = supplied
		}
		if len(value) > 8192 || strings.ContainsRune(value, 0) || (v.Required && strings.TrimSpace(value) == "") {
			return nil, nil, fmt.Errorf("invalid variable value")
		}
		merged[v.Name] = value
	}
	for name := range values {
		if _, ok := merged[name]; !ok {
			return nil, nil, fmt.Errorf("unknown variable")
		}
	}
	sc.MigrateCommandsToSteps()
	if len(sc.Steps) > 1000 {
		return nil, nil, fmt.Errorf("too many steps")
	}
	var commands []string
	var delays []time.Duration
	bytes := 0
	for _, step := range sc.Steps {
		if !step.Enabled {
			continue
		}
		if step.Delay < 0 || step.Delay > 300000 {
			return nil, nil, fmt.Errorf("invalid delay")
		}
		command := script.SubstituteVariables(step.Command, merged)
		bytes += len(command)
		if len(command) > 65535 || strings.ContainsRune(command, 0) || bytes > 1<<20 {
			return nil, nil, fmt.Errorf("script exceeds dispatch limit")
		}
		if strings.TrimSpace(command) == "" {
			continue
		}
		commands = append(commands, command+"\n")
		delays = append(delays, time.Duration(step.Delay)*time.Millisecond)
	}
	if len(commands) == 0 {
		return nil, nil, fmt.Errorf("script has no enabled commands")
	}
	return commands, delays, nil
}

func runReplay(ctx context.Context, job *replayJob, commands []string, delays []time.Duration, write func(string, []byte) error) {
	defer close(job.done)
	defer job.cancel()
	finish := func(state string) { job.mu.Lock(); job.status.State = state; job.mu.Unlock() }
	for i, command := range commands {
		delay := delays[i]
		if i > 0 {
			delay += 500 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			finish("stopped")
			return
		case <-timer.C:
		}
		// Status stays readable while SSH stdin is backpressured. Stop waits for
		// the worker's done signal before acknowledging that dispatch has stopped.
		if ctx.Err() != nil {
			finish("stopped")
			return
		}
		err := write(job.status.TerminalID, []byte(command))
		job.mu.Lock()
		if err != nil {
			job.status.State = "failed"
			job.mu.Unlock()
			return
		}
		job.status.Sent++
		job.mu.Unlock()
	}
	finish("dispatched")
}

func (s *StructuredScriptService) ReplayStatus(runID string) (ReplayState, error) {
	s.mu.Lock()
	job := s.jobs[runID]
	s.mu.Unlock()
	if job == nil {
		return ReplayState{}, fmt.Errorf("replay not found")
	}
	return job.snapshot(), nil
}

func (s *StructuredScriptService) StopReplay(ctx context.Context, runID string) (ReplayState, error) {
	s.mu.Lock()
	job := s.jobs[runID]
	s.mu.Unlock()
	if job == nil {
		return ReplayState{}, fmt.Errorf("replay not found")
	}
	job.cancel()
	select {
	case <-job.done:
		return job.snapshot(), nil
	case <-ctx.Done():
		return ReplayState{}, ctx.Err()
	}
}

// Close cancels pending dispatch before the Sidecar tears down its terminals.
func (s *StructuredScriptService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, job := range s.jobs {
		job.cancel()
	}
}

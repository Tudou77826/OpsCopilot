// Package filecontrol coordinates visible file workspaces and revocable transfer
// jobs. Transport implementations remain shared with the manual file panel.
package filecontrol

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"opscopilot/pkg/filetransfer"
)

var ErrStale = errors.New("文件页签授权已失效")

type Endpoint struct {
	SessionID    string `json:"sessionId"`
	ConnectionID string `json:"connectionId"`
	Title        string `json:"title"`
	Identity     string `json:"identity"`
}
type Task struct {
	ID          string `json:"id"`
	Direction   string `json:"direction"`
	LocalPath   string `json:"localPath"`
	RemotePath  string `json:"remotePath"`
	Status      string `json:"status"`
	Bytes       int64  `json:"bytes"`
	Total       int64  `json:"total"`
	Message     string `json:"message"`
	Transferred bool   `json:"transferred"` // transport acknowledged completion, independent of audit
}
type Workspace struct {
	ID            string               `json:"id"`
	Endpoint      Endpoint             `json:"endpoint"`
	LocalPath     string               `json:"localPath"`
	RemotePath    string               `json:"remotePath"`
	LocalEntries  []filetransfer.Entry `json:"localEntries"`
	RemoteEntries []filetransfer.Entry `json:"remoteEntries"`
	Tasks         []Task               `json:"tasks"`
	OwnerTurn     string               `json:"ownerTurn"`
	Pending       string               `json:"pending"`
	Revision      uint64               `json:"revision"`
	Dismissed     bool                 `json:"dismissed"`
	Error         string               `json:"error,omitempty"`
	RevealTask    string               `json:"revealTask,omitempty"`
	RevealNonce   string               `json:"revealNonce,omitempty"`
}
type Backend interface {
	Check(context.Context, Endpoint) error
	List(context.Context, Endpoint, string, bool) ([]filetransfer.Entry, error)
	Transfer(context.Context, Endpoint, Task, func(filetransfer.Progress)) (filetransfer.TransferResult, error)
}
type Event struct {
	At        time.Time `json:"at"`
	Turn      string    `json:"turn"`
	Workspace string    `json:"workspace"`
	Task      string    `json:"task,omitempty"`
	Action    string    `json:"action"`
	PathsHash string    `json:"pathsHash,omitempty"`
	Status    string    `json:"status,omitempty"`
}
type Journal interface {
	Append(context.Context, Event) error
}
type job struct {
	task   Task
	cancel context.CancelFunc
	done   chan struct{}
	turn   string
}
type record struct {
	Workspace
	receipt chan struct{}
	lease   context.Context
	cancel  context.CancelFunc
	jobs    map[string]*job
}
type Service struct {
	mu       sync.Mutex
	items    map[string]*record
	revision uint64
	Backend  Backend
	Journal  Journal
}

func New(backend Backend, journal Journal) *Service {
	return &Service{Backend: backend, Journal: journal, items: map[string]*record{}}
}
func (s *Service) changed(r *record) { s.revision++; r.Revision = s.revision }
func copyWorkspace(r *record) Workspace {
	w := r.Workspace
	w.LocalEntries = append([]filetransfer.Entry{}, w.LocalEntries...)
	w.RemoteEntries = append([]filetransfer.Entry{}, w.RemoteEntries...)
	w.Tasks = []Task{}
	for _, j := range r.jobs {
		w.Tasks = append(w.Tasks, j.task)
	}
	sort.Slice(w.Tasks, func(i, j int) bool { return w.Tasks[i].ID < w.Tasks[j].ID })
	return w
}
func (s *Service) Snapshot(id string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[id]
	if r == nil || r.Dismissed {
		return Workspace{}, ErrStale
	}
	return copyWorkspace(r), nil
}

// Reveal never starts a transfer or reads a file. Existing task records survive
// a disconnected SSH channel and remain inspectable in the file workspace.
func (s *Service) Reveal(ctx context.Context, id, taskID string) error {
	s.mu.Lock()
	r := s.items[id]
	if r == nil || r.Dismissed {
		s.mu.Unlock()
		return errors.New("原文件页签已关闭或未恢复；不会重新连接或传输")
	}
	if r.Pending != "" {
		s.mu.Unlock()
		return errors.New("文件页签正在呈现，请稍后再定位")
	}
	if taskID != "" && r.jobs[taskID] == nil {
		s.mu.Unlock()
		return errors.New("原传输任务已不存在")
	}
	r.RevealTask, r.RevealNonce = taskID, uuid.NewString()
	s.changed(r)
	s.mu.Unlock()
	return s.Present(ctx, id)
}
func (s *Service) Poll(since uint64) (uint64, []Workspace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Workspace{}
	if since != s.revision {
		for _, r := range s.items {
			out = append(out, copyWorkspace(r))
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	}
	return s.revision, out
}
func (s *Service) Open(ctx context.Context, e Endpoint) (Workspace, error) {
	if err := s.Backend.Check(ctx, e); err != nil {
		return Workspace{}, err
	}
	s.mu.Lock()
	for id, r := range s.items {
		if r.Dismissed {
			delete(s.items, id)
		}
	}
	for _, r := range s.items {
		if !r.Dismissed && r.Endpoint == e {
			w := copyWorkspace(r)
			s.mu.Unlock()
			return w, nil
		}
	}
	if len(s.items) >= 64 {
		s.mu.Unlock()
		return Workspace{}, errors.New("文件页签过多，请先关闭不用的页签")
	}
	r := &record{Workspace: Workspace{ID: uuid.NewString(), Endpoint: e, RemotePath: "/", LocalEntries: []filetransfer.Entry{}, RemoteEntries: []filetransfer.Entry{}}, jobs: map[string]*job{}}
	s.items[r.ID] = r
	s.changed(r)
	w := copyWorkspace(r)
	s.mu.Unlock()
	return w, nil
}
func (s *Service) Present(ctx context.Context, id string) error {
	s.mu.Lock()
	r := s.items[id]
	if r == nil || r.Dismissed || ctx.Err() != nil {
		s.mu.Unlock()
		return ErrStale
	}
	if r.Pending != "" {
		s.mu.Unlock()
		return errors.New("文件页签正在呈现")
	}
	nonce := uuid.NewString()
	r.Pending = nonce
	r.receipt = make(chan struct{})
	receipt := r.receipt
	s.changed(r)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if r.Pending == nonce {
			r.Pending = ""
			s.changed(r)
		}
		s.mu.Unlock()
	}()
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("文件页签呈现超时，未继续操作")
	case <-receipt:
		return ctx.Err()
	}
}
func (s *Service) Acknowledge(id, nonce, connection string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[id]
	if r == nil || nonce == "" || r.Pending != nonce || r.Endpoint.ConnectionID != connection {
		return ErrStale
	}
	r.Pending = ""
	close(r.receipt)
	s.changed(r)
	return nil
}
func (s *Service) Take(ctx context.Context, id, turn string) error {
	w, err := s.Snapshot(id)
	if err != nil {
		return err
	}
	if err = s.Backend.Check(ctx, w.Endpoint); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[id]
	if r.Dismissed || ctx.Err() != nil {
		return ErrStale
	}
	if r.OwnerTurn != "" {
		return errors.New("文件页签已被占用")
	}
	r.lease, r.cancel = context.WithCancel(ctx)
	r.OwnerTurn = turn
	s.changed(r)
	return nil
}
func (s *Service) access(id, turn string) (context.Context, Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[id]
	if r == nil || r.Dismissed || r.OwnerTurn != turn || r.lease == nil || r.lease.Err() != nil {
		return nil, Workspace{}, ErrStale
	}
	if r.Error != "" {
		return nil, Workspace{}, errors.New(r.Error)
	}
	return r.lease, copyWorkspace(r), nil
}
func (s *Service) Record(ctx context.Context, id, turn, action, task, paths, status string) error {
	if s.Journal == nil {
		return errors.New("文件操作审计不可用")
	}
	h := ""
	if paths != "" {
		h = fmt.Sprintf("%x", sha256.Sum256([]byte(paths)))
	}
	if err := s.Journal.Append(ctx, Event{At: time.Now().UTC(), Turn: turn, Workspace: id, Task: task, Action: action, PathsHash: h, Status: status}); err != nil {
		return fmt.Errorf("文件操作审计失败: %w", err)
	}
	return nil
}
func (s *Service) Browse(ctx context.Context, id, turn, path string, local bool) (Workspace, error) {
	lease, w, err := s.access(id, turn)
	if err != nil {
		return w, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(lease, cancel)
	defer stop()
	if err = s.Backend.Check(ctx, w.Endpoint); err != nil {
		return w, err
	}
	if err = s.Record(ctx, id, turn, "browse", "", path, ""); err != nil {
		return w, err
	}
	entries, err := s.Backend.List(ctx, w.Endpoint, path, local)
	if err != nil {
		return w, err
	}
	if len(entries) > 4096 {
		return w, errors.New("目录超过 4096 项，请通过终端缩小范围")
	}
	s.mu.Lock()
	r := s.items[id]
	if ctx.Err() != nil || r.OwnerTurn != turn {
		s.mu.Unlock()
		return w, ErrStale
	}
	if local {
		r.LocalPath = path
		r.LocalEntries = entries
	} else {
		r.RemotePath = path
		r.RemoteEntries = entries
	}
	s.changed(r)
	s.mu.Unlock()
	if err = s.Present(ctx, id); err != nil {
		return w, err
	}
	return s.Snapshot(id)
}
func (s *Service) Start(ctx context.Context, id, turn string, task Task) (Task, error) {
	lease, w, err := s.access(id, turn)
	if err != nil {
		return Task{}, err
	}
	if err = s.Backend.Check(ctx, w.Endpoint); err != nil {
		return Task{}, err
	}
	if task.Direction != "upload" && task.Direction != "download" {
		return Task{}, errors.New("未知传输方向")
	}
	task.ID = uuid.NewString()
	task.Status = "running"
	task.Total = -1
	if err = s.Record(ctx, id, turn, "transfer_intent", task.ID, task.LocalPath+"\x00"+task.RemotePath, ""); err != nil {
		return Task{}, err
	}
	s.mu.Lock()
	r := s.items[id]
	if ctx.Err() != nil || lease.Err() != nil || r.OwnerTurn != turn {
		s.mu.Unlock()
		return Task{}, ErrStale
	}
	if len(r.jobs) >= 128 {
		s.mu.Unlock()
		return Task{}, errors.New("此文件页签已达任务上限，请新建页签")
	}
	for _, old := range r.jobs {
		if old.task.Status == "running" || old.task.Status == "cancelling" {
			s.mu.Unlock()
			return Task{}, errors.New("请等待当前文件传输结束")
		}
	}
	run, cancel := context.WithCancel(lease)
	j := &job{task: task, cancel: cancel, done: make(chan struct{}), turn: turn}
	r.jobs[task.ID] = j
	s.changed(r)
	s.mu.Unlock()
	go func() {
		defer close(j.done)
		defer cancel()
		res, runErr := s.Backend.Transfer(run, w.Endpoint, task, func(p filetransfer.Progress) {
			s.mu.Lock()
			if j.task.Status == "running" {
				if p.Step == "" {
					j.task.Bytes = p.BytesDone
					j.task.Total = p.BytesTotal
				}
				j.task.Message = p.Step
				s.changed(r)
			}
			s.mu.Unlock()
		})
		status, msg := "done", "传输完成"
		if runErr != nil {
			status = "failed"
			msg = runErr.Error()
		}
		if runErr != nil && run.Err() != nil {
			status = "cancelled"
			msg = "已取消；目标可能保留部分文件，请检查后再继续"
		}
		auditCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		auditErr := s.Record(auditCtx, id, turn, "transfer_result", task.ID, "", status)
		done()
		if auditErr != nil {
			status = "failed"
			msg += "；" + auditErr.Error()
		}
		s.mu.Lock()
		if auditErr != nil {
			r.Error = "传输结果审计未完整保存，已禁止此页签继续自动操作；请检查实际文件，不要自动重试"
		}
		j.task.Status = status
		j.task.Transferred = runErr == nil
		j.task.Message = msg
		if runErr == nil {
			j.task.Bytes = res.Bytes
		}
		s.changed(r)
		s.mu.Unlock()
	}()
	return task, nil
}
func (s *Service) Cancel(id, turn, task string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[id]
	if r == nil {
		return ErrStale
	}
	j := r.jobs[task]
	if j == nil || j.turn != turn {
		return ErrStale
	}
	if j.task.Status == "running" {
		j.task.Status = "cancelling"
		j.cancel()
		s.changed(r)
	}
	return nil
}

// Release cancels and drains this owner's unfinished jobs before returning control.
func (s *Service) Release(id, turn string) {
	s.mu.Lock()
	r := s.items[id]
	if r == nil || r.OwnerTurn != turn {
		s.mu.Unlock()
		return
	}
	if r.cancel != nil {
		r.cancel()
	}
	jobs := []*job{}
	for _, j := range r.jobs {
		if j.turn == turn {
			jobs = append(jobs, j)
			if j.task.Status == "running" {
				j.task.Status = "cancelling"
			}
		}
	}
	s.changed(r)
	s.mu.Unlock()
	for _, j := range jobs {
		<-j.done
	}
	s.mu.Lock()
	if r.OwnerTurn == turn {
		r.OwnerTurn = ""
		r.lease = nil
		r.cancel = nil
		s.changed(r)
	}
	s.mu.Unlock()
}
func (s *Service) Dismiss(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[id]
	if r == nil {
		return nil
	}
	if r.OwnerTurn != "" || r.Pending != "" {
		return errors.New("请先停止 AI 并接回文件页签")
	}
	r.Dismissed = true
	s.changed(r)
	return nil
}

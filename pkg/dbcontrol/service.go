// Package dbcontrol owns AI-created database workspaces, append-only execution
// blocks and short-lived authority. Query execution remains in dbworkspace.
package dbcontrol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"opscopilot/pkg/dbworkspace"
)

type Block struct {
	ID      string                  `json:"id"`
	Purpose string                  `json:"purpose"`
	SQL     string                  `json:"sql"`
	At      time.Time               `json:"at"`
	Status  string                  `json:"status"`
	Result  dbworkspace.QueryResult `json:"result"`
	Error   string                  `json:"error,omitempty"`
}
type Workspace struct {
	ID          string                  `json:"id"`
	Profile     dbworkspace.Profile     `json:"profile"`
	Session     dbworkspace.SessionInfo `json:"session"`
	Schemas     []string                `json:"schemas"`
	SQL         string                  `json:"sql"`
	Blocks      []Block                 `json:"blocks"`
	OwnerTurn   string                  `json:"ownerTurn"`
	Pending     string                  `json:"pending"`
	Connected   bool                    `json:"connected"`
	Dismissed   bool                    `json:"dismissed"`
	Revision    uint64                  `json:"revision"`
	RevealBlock string                  `json:"revealBlock,omitempty"`
	RevealNonce string                  `json:"revealNonce,omitempty"`
}
type Event struct {
	At          time.Time `json:"at"`
	TurnID      string    `json:"turnId"`
	WorkspaceID string    `json:"workspaceId"`
	SessionID   string    `json:"sessionId"`
	BlockID     string    `json:"blockId,omitempty"`
	SQLHash     string    `json:"sqlHash,omitempty"`
	Phase       string    `json:"phase"`
}
type Journal interface {
	Append(context.Context, Event) error
}

var ErrAudit = errors.New("数据库审计不可用")

func (s *Service) Record(ctx context.Context, id, turn, phase string) error {
	s.mu.Lock()
	r := s.items[id]
	if r == nil || r.Dismissed {
		s.mu.Unlock()
		return errors.New("数据库页签不存在")
	}
	sessionID := r.Session.ID
	s.mu.Unlock()
	if s.Journal == nil {
		return ErrAudit
	}
	if err := s.Journal.Append(ctx, Event{At: time.Now().UTC(), TurnID: turn, WorkspaceID: id, SessionID: sessionID, Phase: phase}); err != nil {
		return fmt.Errorf("%w: 操作未继续", ErrAudit)
	}
	return nil
}

type Database interface {
	Connect(context.Context, string) (dbworkspace.SessionInfo, error)
	ConnectRevision(context.Context, string, int) (dbworkspace.SessionInfo, error)
	Disconnect(string) error
	DisconnectContext(context.Context, string) error
	Session(string) (dbworkspace.SessionInfo, error)
	Own(context.Context, string, string) (func(), error)
	SchemasContext(context.Context, string) ([]string, error)
	RunContext(context.Context, dbworkspace.QueryRequest) (dbworkspace.QueryResult, error)
	List() ([]dbworkspace.Profile, error)
	Dialect(string) (string, error)
	TablesContext(context.Context, string, string) ([]dbworkspace.Table, error)
	StructureContext(context.Context, dbworkspace.TableRequest) (dbworkspace.Structure, error)
}
type record struct {
	ownerID string
	Workspace
	receipt chan struct{}
	release context.CancelFunc
}
type Service struct {
	DB       Database
	Journal  Journal
	mu       sync.Mutex
	items    map[string]*record
	revision uint64
}

func New(db Database, journal Journal) *Service {
	return &Service{DB: db, Journal: journal, items: map[string]*record{}}
}
func (s *Service) changed(r *record) { s.revision++; r.Revision = s.revision }
func clone(w Workspace) Workspace {
	data, _ := json.Marshal(w)
	var copy Workspace
	_ = json.Unmarshal(data, &copy)
	return copy
}
func (s *Service) List(since uint64) (uint64, []Workspace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.items {
		if r.Connected {
			if _, err := s.DB.Session(r.Session.ID); err != nil {
				r.Connected = false
				s.changed(r)
			}
		}
	}
	out := []Workspace{}
	if since != s.revision {
		for _, r := range s.items {
			out = append(out, clone(r.Workspace))
		}
	}
	return s.revision, out
}
func (s *Service) Snapshot(id string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[id]
	if r == nil || r.Dismissed {
		return Workspace{}, errors.New("数据库页签不存在")
	}
	return clone(r.Workspace), nil
}

// Reveal changes presentation only, including disconnected retained results.
func (s *Service) Reveal(ctx context.Context, id, blockID string) error {
	s.mu.Lock()
	r := s.items[id]
	if r == nil || r.Dismissed {
		s.mu.Unlock()
		return errors.New("原数据库页签已关闭或未恢复；不会重新连接或执行 SQL")
	}
	if r.Pending != "" {
		s.mu.Unlock()
		return errors.New("数据库页签正在呈现，请稍后再定位")
	}
	if blockID != "" {
		found := false
		for _, block := range r.Blocks {
			if block.ID == blockID {
				found = true
				break
			}
		}
		if !found {
			s.mu.Unlock()
			return errors.New("原 SQL 记录已不存在")
		}
	}
	r.RevealBlock, r.RevealNonce = blockID, uuid.NewString()
	s.changed(r)
	s.mu.Unlock()
	return s.Present(ctx, id)
}
func (s *Service) Open(ctx context.Context, profile dbworkspace.Profile) (Workspace, error) {
	info, err := s.DB.ConnectRevision(ctx, profile.ID, profile.Revision)
	if err != nil {
		return Workspace{}, err
	}
	if ctx.Err() != nil {
		_ = s.DB.Disconnect(info.ID)
		return Workspace{}, ctx.Err()
	}
	r := &record{Workspace: Workspace{ID: uuid.NewString(), Profile: profile, Session: info, Connected: true, Blocks: []Block{}}}
	r.Schemas, err = s.DB.SchemasContext(ctx, info.ID)
	if err != nil {
		_ = s.DB.Disconnect(info.ID)
		return Workspace{}, fmt.Errorf("读取数据库列表失败: %w", err)
	}
	s.mu.Lock()
	s.items[r.ID] = r
	s.changed(r)
	s.mu.Unlock()
	if err = s.Present(ctx, r.ID); err != nil {
		_ = s.DB.Disconnect(info.ID)
		s.mu.Lock()
		r.Connected = false
		r.Dismissed = true
		delete(s.items, r.ID)
		s.revision++
		s.mu.Unlock()
		return Workspace{}, err
	}
	return s.Snapshot(r.ID)
}

// Present is required for opening, taking and before every submitted SQL block.
func (s *Service) Present(ctx context.Context, id string) error {
	s.mu.Lock()
	r := s.items[id]
	if r == nil || r.Dismissed {
		s.mu.Unlock()
		return errors.New("数据库页签不存在")
	}
	if r.Pending != "" {
		s.mu.Unlock()
		return errors.New("数据库页签正在等待呈现，请顺序操作")
	}
	r.Pending = uuid.NewString()
	r.receipt = make(chan struct{})
	receipt := r.receipt
	nonce := r.Pending
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
		return errors.New("数据库页签未呈现，操作未继续")
	case <-receipt:
		return ctx.Err()
	}
}
func (s *Service) Acknowledge(id, nonce, sessionID, sql string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[id]
	if r == nil || nonce == "" || r.Pending != nonce || r.Session.ID != sessionID || r.SQL != sql || r.receipt == nil {
		return errors.New("数据库呈现回执已失效")
	}
	close(r.receipt)
	r.receipt = nil
	r.Pending = ""
	s.changed(r)
	return nil
}
func (s *Service) Take(ctx context.Context, id, turn string) (context.Context, error) {
	if err := s.Present(ctx, id); err != nil {
		return nil, err
	}
	s.mu.Lock()
	r := s.items[id]
	if r == nil || !r.Connected || r.OwnerTurn != "" {
		s.mu.Unlock()
		return nil, errors.New("数据库页签已断开或正在使用")
	}
	sessionID := r.Session.ID
	s.mu.Unlock()
	lease, cancel := context.WithTimeout(ctx, 2*time.Minute)
	owner := uuid.NewString()
	release, err := s.DB.Own(lease, sessionID, owner)
	if err != nil {
		cancel()
		return nil, err
	}
	s.mu.Lock()
	if lease.Err() != nil || r.Dismissed || !r.Connected || r.OwnerTurn != "" {
		s.mu.Unlock()
		cancel()
		release()
		return nil, errors.New("数据库接管已取消或现场已改变")
	}
	r.OwnerTurn = turn
	r.ownerID = owner
	r.release = func() { cancel(); release() }
	s.changed(r)
	s.mu.Unlock()
	context.AfterFunc(lease, func() {
		release()
		s.mu.Lock()
		if r.release != nil && r.ownerID == owner {
			r.OwnerTurn = ""
			r.release = nil
			s.changed(r)
		}
		s.mu.Unlock()
	})
	return dbworkspace.WithOwnership(lease, owner), nil
}
func (s *Service) Release(id, turn string) {
	s.mu.Lock()
	r := s.items[id]
	var cancel context.CancelFunc
	if r != nil && r.OwnerTurn == turn {
		cancel = r.release
		r.release = nil
		r.ownerID = ""
		r.OwnerTurn = ""
		s.changed(r)
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
func (s *Service) owned(ctx context.Context, id, turn string) (*record, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	r := s.items[id]
	if r == nil || r.Dismissed || !r.Connected || r.OwnerTurn != turn {
		return nil, errors.New("请先取得该数据库页签的控制权")
	}
	return r, nil
}
func (s *Service) Append(ctx context.Context, id, turn, purpose, sql string) (Block, error) {
	if strings.TrimSpace(purpose) == "" || len(purpose) > 2048 || strings.TrimSpace(sql) == "" || len(sql) > 8192 {
		return Block{}, errors.New("请提供用途说明和有效 SQL，且长度不能超过限制")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.owned(ctx, id, turn)
	if err != nil {
		return Block{}, err
	}
	if len(r.SQL)+len(sql)+len(purpose) > 1<<20 {
		return Block{}, errors.New("查询区已达到 1 MiB，请新建页签")
	}
	b := Block{ID: uuid.NewString(), Purpose: purpose, SQL: sql, At: time.Now().UTC(), Status: "not_executed", Result: dbworkspace.QueryResult{Sets: []dbworkspace.ResultSet{}}}
	comment := fmt.Sprintf("-- AI · %s\n", b.At.Local().Format("2006-01-02 15:04:05"))
	for _, line := range splitLines(purpose) {
		comment += "-- " + line + "\n"
	}
	if r.SQL != "" {
		r.SQL += "\n\n"
	}
	r.SQL += comment + sql
	r.Blocks = append(r.Blocks, b)
	s.changed(r)
	return b, nil
}
func (s *Service) SetBlock(id, blockID, status string, result dbworkspace.QueryResult, message string) {
	if result.Sets == nil {
		result.Sets = []dbworkspace.ResultSet{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.items[id]; r != nil {
		for i := range r.Blocks {
			if r.Blocks[i].ID == blockID {
				previous := r.Blocks[i].Status
				if previous == "completed" || previous == "cancelled" || previous == "failed" {
					// A late transport/audit error cannot rewrite an observed SQL
					// outcome or discard its result after control was handed back.
					if message != "" {
						r.Blocks[i].Error = message
						s.changed(r)
					}
					return
				}
				r.Blocks[i].Status = status
				r.Blocks[i].Result = result
				r.Blocks[i].Error = message
				s.changed(r)
				break
			}
		}
		if result.Disconnected {
			r.Connected = false
			s.changed(r)
		}
	}
}
func (s *Service) Execute(ctx context.Context, id, turn string, b Block, database string) (dbworkspace.QueryResult, error) {
	s.mu.Lock()
	r, err := s.owned(ctx, id, turn)
	if err != nil {
		s.mu.Unlock()
		return dbworkspace.QueryResult{}, err
	}
	sessionID := r.Session.ID
	// Reserve the immutable stored block before presentation or any slow I/O.
	// Repeated or concurrent dispatches must never execute it twice.
	found := false
	for i := range r.Blocks {
		stored := &r.Blocks[i]
		if stored.ID == b.ID && stored.SQL == b.SQL && (stored.Status == "not_executed" || stored.Status == "awaiting_approval") && stored.Error == "" {
			b = *stored
			stored.Status = "preparing"
			s.changed(r)
			found = true
			break
		}
	}
	s.mu.Unlock()
	if !found {
		return dbworkspace.QueryResult{}, errors.New("SQL 记录已失效或已提交，请追加新的执行记录")
	}
	if err = s.Present(ctx, id); err != nil {
		return dbworkspace.QueryResult{}, err
	}
	if s.Journal == nil {
		return dbworkspace.QueryResult{}, fmt.Errorf("%w，未提交 SQL", ErrAudit)
	}
	hash := sha256.Sum256([]byte(b.SQL))
	event := Event{At: time.Now().UTC(), TurnID: turn, WorkspaceID: id, SessionID: sessionID, BlockID: b.ID, SQLHash: hex.EncodeToString(hash[:]), Phase: "dispatch_intent"}
	if err = s.Journal.Append(ctx, event); err != nil {
		return dbworkspace.QueryResult{}, fmt.Errorf("%w，未提交 SQL", ErrAudit)
	}
	s.SetBlock(id, b.ID, "running", dbworkspace.QueryResult{}, "")
	result, err := s.DB.RunContext(ctx, dbworkspace.QueryRequest{SessionID: sessionID, RequestID: b.ID, SQL: b.SQL, Database: database, Mode: "selection", Limit: 1000, TimeoutSeconds: 30})
	status, message := "completed", ""
	if result.Cancelled || ctx.Err() != nil {
		status = "cancelled"
		if err != nil {
			message = err.Error()
		}
	} else if err != nil {
		status = "failed"
		message = err.Error()
	} else if result.Error != "" {
		status = "failed"
		message = result.Error
	}
	s.SetBlock(id, b.ID, status, result, message)
	event.At = time.Now().UTC()
	event.Phase = status
	if auditErr := s.Journal.Append(context.Background(), event); auditErr != nil {
		outcome := "SQL 可能已执行"
		if status == "completed" {
			outcome = "SQL 已执行"
		}
		return result, fmt.Errorf("%w：%s，但结果审计未保存；请核对页面，勿自动重试", ErrAudit, outcome)
	}
	return result, err
}
func (s *Service) Edit(id, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[id]
	if r == nil {
		return errors.New("页签不存在")
	}
	if r.OwnerTurn != "" {
		return dbworkspace.ErrOwned
	}
	if len(text) > 1<<20 {
		return errors.New("查询区过大")
	}
	r.SQL = text
	s.changed(r)
	return nil
}
func (s *Service) Close(ctx context.Context, id, turn string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	r := s.items[id]
	if r == nil {
		s.mu.Unlock()
		return errors.New("页签不存在")
	}
	if r.OwnerTurn != "" && r.OwnerTurn != turn {
		s.mu.Unlock()
		return dbworkspace.ErrOwned
	}
	if turn != "" {
		if _, err := s.owned(ctx, id, turn); err != nil {
			s.mu.Unlock()
			return err
		}
	}
	sessionID := r.Session.ID
	s.mu.Unlock()
	if err := s.DB.DisconnectContext(ctx, sessionID); err != nil {
		return err
	}
	s.Release(id, turn)
	s.mu.Lock()
	r.Connected = false
	s.changed(r)
	s.mu.Unlock()
	return nil
}
func (s *Service) Dismiss(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.items[id]
	if r == nil {
		return nil
	}
	if r.Connected || r.OwnerTurn != "" {
		return errors.New("请先断开连接并释放页签")
	}
	r.Dismissed = true
	delete(s.items, id)
	s.revision++
	return nil
}

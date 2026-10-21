package dbworkspace

import (
	"context"
	"database/sql"
	"errors"
)

var ErrOwned = errors.New("该数据库会话正由 AI 使用，请先手动接管")

type ownershipKey struct{}

// WithOwnership is a trusted host capability, never a model/UI request field.
func WithOwnership(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, ownershipKey{}, owner)
}

func (s *Service) Own(ctx context.Context, id, owner string) (func(), error) {
	if owner == "" || ctx.Err() != nil {
		return nil, ErrOwned
	}
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v := s.sessions[id]
	if v == nil {
		return nil, errors.New("数据库会话已断开")
	}
	if v.owner != "" || len(v.gate) != 0 {
		return nil, errors.New("数据库会话正在使用中")
	}
	v.owner = owner
	release := func() {
		s.mu.Lock()
		if v.owner == owner {
			v.owner = ""
		}
		s.mu.Unlock()
	}
	stop := context.AfterFunc(ctx, release)
	return func() { stop(); release() }, nil
}
func (s *Service) Session(id string) (SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.sessions[id]; v != nil {
		return v.info, nil
	}
	return SessionInfo{}, errors.New("数据库会话已断开")
}
func (s *Service) editableProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.sessions {
		if v.info.ProfileID == id && v.owner != "" {
			return ErrOwned
		}
	}
	return nil
}
func sessionContext(parent context.Context, v *session) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(v.ctx)
	stop := context.AfterFunc(parent, cancel)
	if parent.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}

func (s *Service) retireInterruptedSession(ctx context.Context, id string, v *session) {
	broken := ctx.Err() != nil
	if health, ok := v.connector.(interface{ Disconnected(*sql.Conn) bool }); ok {
		broken = broken || health.Disconnected(v.conn)
	}
	if broken {
		s.discardSession(id, v)
	}
}

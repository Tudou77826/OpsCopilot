package session

import (
	"errors"
	"sync"
)

var ErrConnectingOrActive = errors.New("此页签正在连接或已有活动连接")

// Lifecycle fences a connection attempt and all of its later reader callbacks.
// Its zero value is ready to use. Different tabs never share a callback lock.
type Lifecycle struct {
	mu      sync.Mutex
	current map[string]*Lifetime
}

type Lifetime struct {
	mu     sync.Mutex
	owner  *Lifecycle
	id     string
	closed bool
}

// Begin reserves the tab before dialing. Repeated reconnects cannot open two
// remote shells for the same tab. Finish releases the reservation on failure.
func (l *Lifecycle) Begin(id string) (*Lifetime, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.current == nil {
		l.current = make(map[string]*Lifetime)
	}
	if l.current[id] != nil {
		return nil, ErrConnectingOrActive
	}
	v := &Lifetime{owner: l, id: id}
	l.current[id] = v
	return v, nil
}

// Apply serializes short publication/state changes with close/disconnect.
// Callbacks must not perform network I/O or reenter this lifetime. Blocking
// reads, dialing and resource disposal belong outside Apply/Finish callbacks.
func (v *Lifetime) Apply(fn func()) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return false
	}
	if fn != nil {
		fn()
	}
	return true
}

// ApplyCurrent is for late UI callbacks that also check their captured resource
// instance. Looking up the current lifetime alone does not authorize old work.
func (l *Lifecycle) ApplyCurrent(id string, fn func()) bool {
	l.mu.Lock()
	v := l.current[id]
	l.mu.Unlock()
	return v != nil && v.Apply(fn)
}

// Finish invalidates the lifetime before cleanup and admits the next attempt
// only after the callback has completed. Old callbacks remain invalid forever.
func (v *Lifetime) Finish(fn func()) bool {
	return v.finishIf(nil, fn)
}
func (v *Lifetime) finishIf(matches func() bool, fn func()) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || (matches != nil && !matches()) {
		return false
	}
	v.closed = true
	if fn != nil {
		fn()
	}
	v.owner.mu.Lock()
	if v.owner.current[v.id] == v {
		delete(v.owner.current, v.id)
	}
	v.owner.mu.Unlock()
	return true
}

// CloseMatching never closes a replacement connection after a stale cleanup.
func (l *Lifecycle) CloseMatching(id string, matches func() bool, fn func()) bool {
	l.mu.Lock()
	v := l.current[id]
	l.mu.Unlock()
	return v != nil && v.finishIf(matches, fn)
}

// Close also fences a pending dial. A temporary reservation serializes cleanup
// even when the previous reader already finished or the tab never connected.
func (l *Lifecycle) Close(id string, fn func()) {
	for {
		l.mu.Lock()
		if l.current == nil {
			l.current = make(map[string]*Lifetime)
		}
		v := l.current[id]
		if v == nil {
			v = &Lifetime{owner: l, id: id}
			l.current[id] = v
		}
		l.mu.Unlock()
		// If another Finish won, its cleanup is complete. Retry against the current
		// lifetime so explicit close still removes the retained disconnected state.
		if v.Finish(fn) {
			return
		}
	}
}

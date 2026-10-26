package sshclient

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"opscopilot/pkg/remote"
)

// boundedCapture drains the stream while retaining a bounded head and tail.
// Snapshot is safe even when cancellation races a late SSH reader.
type boundedCapture struct {
	mu         sync.Mutex
	cap        int
	count      int64
	head, tail []byte
}

func (b *boundedCapture) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	b.count += int64(n)
	headCap := b.cap / 2
	take := headCap - len(b.head)
	if take > len(p) {
		take = len(p)
	}
	if take > 0 {
		b.head = append(b.head, p[:take]...)
		p = p[take:]
	}
	tailCap := b.cap - headCap
	if len(p) >= tailCap {
		b.tail = append(b.tail[:0], p[len(p)-tailCap:]...)
	} else {
		overflow := len(b.tail) + len(p) - tailCap
		if overflow > 0 {
			copy(b.tail, b.tail[overflow:])
			b.tail = b.tail[:len(b.tail)-overflow]
		}
		b.tail = append(b.tail, p...)
	}
	return n, nil
}
func (b *boundedCapture) snapshot() (string, int64, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := string(b.head)
	truncated := b.count > int64(len(b.head)+len(b.tail))
	if truncated {
		text += "\n...[采集缓冲已截断，请缩小查询范围]...\n"
	}
	return strings.ToValidUTF8(text+string(b.tail), "�"), b.count, truncated
}

func (c *Client) RunCommand(ctx context.Context, request remote.CommandRequest) (result remote.CommandResult, err error) {
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if c.client == nil {
		return result, errors.New("SSH 连接不可用")
	}
	limit := request.CaptureBytes
	if limit < 1024 || limit > 1<<20 {
		limit = 64 << 10
	}
	type opened struct {
		s *ssh.Session
		e error
	}
	ready := make(chan opened, 1)
	go func() {
		s, e := c.client.NewSession()
		if ctx.Err() != nil && s != nil {
			s.Close()
		}
		ready <- opened{s, e}
	}()
	var session *ssh.Session
	select {
	case <-ctx.Done():
		return result, ctx.Err()
	case value := <-ready:
		session, err = value.s, value.e
	}
	if err != nil {
		return result, err
	}
	defer session.Close()
	stdout, stderr := &boundedCapture{cap: limit}, &boundedCapture{cap: limit}
	session.Stdout = stdout
	session.Stderr = stderr
	session.Stdin = strings.NewReader(request.Input)
	defer func() {
		var a, b bool
		result.Stdout, result.StdoutBytes, a = stdout.snapshot()
		result.Stderr, result.StderrBytes, b = stderr.snapshot()
		result.Truncated = a || b
	}()
	// Best effort: ask the server to kill this owned command, then close its
	// channel. Neither the request nor channel closure proves descendants stopped.
	var cancelling sync.Once
	cancelSession := func() {
		cancelling.Do(func() {
			sent := make(chan struct{})
			go func() { _ = session.Signal(ssh.SIGKILL); close(sent) }()
			select {
			case <-sent:
			case <-time.After(50 * time.Millisecond):
			}
			_ = session.Close()
		})
	}
	stop := context.AfterFunc(ctx, cancelSession)
	defer stop()
	result.StartAttempted = true
	if err = session.Start(request.Command); err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, err
	}
	result.Started = true
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	finish := func(waitErr error) {
		var exit *ssh.ExitError
		if waitErr == nil {
			code := 0
			result.ExitCode = &code
			result.CompletionKnown = true
		} else if errors.As(waitErr, &exit) {
			code := exit.ExitStatus()
			result.ExitCode = &code
			result.CompletionKnown = code >= 0
		}
	}
	select {
	case err = <-done:
		finish(err)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, err
	case <-ctx.Done():
		cancelSession()
		select {
		case waitErr := <-done:
			finish(waitErr)
		case <-time.After(time.Second):
		}
		return result, ctx.Err()
	}
}
